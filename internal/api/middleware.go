package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// CSRFHeader must be sent with every mutating API request. Browsers do not
// allow cross-site requests to set custom headers without a CORS preflight,
// which ds-ui never approves.
const CSRFHeader = "X-Requested-With"

const csrfValue = "ds-ui"

// csrfGuard protects mutating /api requests while ds-ui has no authentication.
func csrfGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/") || isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get(CSRFHeader) != csrfValue {
				writeProblem(w, http.StatusForbidden, "missing "+CSRFHeader+" header")
				return
			}
			if o := r.Header.Get("Origin"); o != "" && !slices.Contains(allowedOrigins, strings.TrimRight(o, "/")) {
				writeProblem(w, http.StatusForbidden, "origin "+o+" is not allowed (DSUI_ALLOWED_ORIGINS)")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem(status, detail))
}

// requestLogger logs API requests; static assets are logged at debug level.
type logUserKey struct{}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			// authGuard, further down the chain, fills in the user.
			who := new(string)
			r = r.WithContext(context.WithValue(r.Context(), logUserKey{}, who))
			next.ServeHTTP(ww, r)
			level := slog.LevelInfo
			if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/v1/system/state" {
				level = slog.LevelDebug
			}
			log.Log(r.Context(), level, "http",
				"method", r.Method, "path", r.URL.Path, "status", ww.Status(),
				"bytes", ww.BytesWritten(), "duration", time.Since(start).Round(time.Microsecond), "user", *who)
		})
	}
}

// hostGuard rejects requests whose Host header is not one of the allowed
// origins. Without authentication this is the defence against DNS rebinding:
// a hostile page whose domain resolves to 127.0.0.1 would otherwise be able to
// read the directory through the browser. /healthz and /readyz stay open for
// container health checks.
func hostGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	hosts := map[string]bool{}
	for _, o := range allowedOrigins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			hosts[strings.ToLower(u.Host)] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(hosts) == 0 || hosts[strings.ToLower(r.Host)] || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			writeProblem(w, http.StatusMisdirectedRequest,
				"host "+r.Host+" is not allowed; add its origin to DSUI_ALLOWED_ORIGINS")
		})
	}
}
