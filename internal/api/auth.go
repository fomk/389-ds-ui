package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/fomk/389-ds-ui/internal/actor"
	"github.com/fomk/389-ds-ui/internal/auth"
)

// SessionCookie holds the session id.
const SessionCookie = "dsui_session"

type requestInfoKey struct{}

// requestInfo is what the auth endpoints need from the raw request.
type requestInfo struct {
	client string // remote address (without port)
	https  bool
}

func clientInfo(r *http.Request) requestInfo {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return requestInfo{client: host, https: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")}
}

// authGuard requires a session for the API (except /api/v1/auth/*), checks
// the role against the method and path, and puts the user into the request
// context. Without an auth service (DSUI_AUTH=none) everything passes.
func authGuard(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), requestInfoKey{}, clientInfo(r))
			r = r.WithContext(ctx)
			if svc == nil || !strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, apiPrefix+"/auth/") {
				next.ServeHTTP(w, r)
				return
			}
			var id string
			if c, err := r.Cookie(SessionCookie); err == nil {
				id = c.Value
			}
			a, err := svc.Session(id)
			if err != nil {
				writeProblem(w, http.StatusUnauthorized, "sign in to use ds-ui")
				return
			}
			if !auth.Allowed(a.Role, r.Method, r.URL.Path) {
				writeProblem(w, http.StatusForbidden, "the role "+a.Role+" may not do this")
				return
			}
			if who, ok := ctx.Value(logUserKey{}).(*string); ok {
				*who = a.DN
			}
			next.ServeHTTP(w, r.WithContext(actor.With(ctx, a)))
		})
	}
}

// Me is the signed-in user.
type Me struct {
	Enabled bool         `json:"enabled" doc:"Authentication is on (DSUI_AUTH=ldap)"`
	User    *actor.Actor `json:"user,omitempty"`
}

type meInput struct {
	Session string `cookie:"dsui_session"`
}
type meOutput struct{ Body Me }
type loginInput struct {
	Body struct {
		Username string `json:"username" minLength:"1" doc:"uid, DN or Directory Manager"`
		Password string `json:"password" minLength:"1"`
	}
}
type loginOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      Me
}
type logoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

func sessionCookie(ctx context.Context, value string, maxAge time.Duration) http.Cookie {
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	c := http.Cookie{Name: SessionCookie, Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: info.https}
	if maxAge > 0 {
		c.MaxAge = int(maxAge.Seconds())
	} else {
		c.MaxAge = -1
	}
	return c
}

func registerAuth(api huma.API, d Deps) {
	tags := []string{"auth"}
	huma.Register(api, huma.Operation{OperationID: "me", Method: http.MethodGet, Path: apiPrefix + "/auth/me",
		Summary: "The signed-in user", Tags: tags},
		func(ctx context.Context, in *meInput) (*meOutput, error) {
			if d.Auth == nil {
				return &meOutput{Body: Me{Enabled: false}}, nil
			}
			out := Me{Enabled: true}
			if a, err := d.Auth.Session(in.Session); err == nil {
				out.User = a
			}
			return &meOutput{Body: out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "login", Method: http.MethodPost, Path: apiPrefix + "/auth/login",
		Summary: "Sign in with a directory account", Tags: tags},
		func(ctx context.Context, in *loginInput) (*loginOutput, error) {
			if d.Auth == nil {
				return nil, problem(http.StatusConflict, "authentication is off (DSUI_AUTH=none)")
			}
			info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
			a, id, err := d.Auth.Login(ctx, in.Body.Username, in.Body.Password, info.client)
			switch {
			case errors.Is(err, auth.ErrInvalidCredentials):
				d.Logger.Warn("login failed", "user", in.Body.Username, "client", info.client)
				return nil, problem(http.StatusUnauthorized, err.Error())
			case errors.Is(err, auth.ErrNoAccess):
				d.Logger.Warn("login refused: no role", "user", in.Body.Username, "client", info.client)
				return nil, problem(http.StatusForbidden, err.Error())
			case errors.Is(err, auth.ErrTooManyAttempts):
				return nil, problem(http.StatusTooManyRequests, err.Error())
			case err != nil:
				return nil, toHTTPError(err)
			}
			d.Logger.Info("login", "user", a.DN, "role", a.Role, "client", info.client)
			return &loginOutput{SetCookie: sessionCookie(ctx, id, d.Auth.Config().MaxAge), Body: Me{Enabled: true, User: a}}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "logout", Method: http.MethodPost, Path: apiPrefix + "/auth/logout",
		Summary: "Sign out", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *meInput) (*logoutOutput, error) {
			if d.Auth != nil && in.Session != "" {
				d.Auth.Logout(in.Session)
			}
			return &logoutOutput{SetCookie: sessionCookie(ctx, "", 0)}, nil
		})
}
