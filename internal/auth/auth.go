// Package auth signs users in with their directory account: the password is
// checked with a bind to the server, the role comes from group membership
// (the root DN is always an administrator), and sessions live in memory.
//
// Directory data is then read and written with Proxied Authorization as the
// user (see ldapx.ProxyControls), so the server applies the user's ACIs.
// (389-ds still records ds-ui's own bind DN in modifiersName; who did what is
// in the ds-ui log and task history.) Server administration (cn=config,
// tasks, files) is done by ds-ui itself and limited by role (Allowed).
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/actor"
	"github.com/fomk/389-ds-ui/internal/ldapx"
)

var (
	// ErrInvalidCredentials hides whether the user or the password was wrong.
	ErrInvalidCredentials = errors.New("invalid user name or password")
	// ErrNoAccess: the password is right but the account has no ds-ui role.
	ErrNoAccess = errors.New("this account may not use ds-ui")
	// ErrTooManyAttempts: login is blocked for a while.
	ErrTooManyAttempts = errors.New("too many failed attempts; try again later")
	// ErrNoSession: not signed in or the session expired.
	ErrNoSession = errors.New("not signed in")
)

// Directory is the server connection ds-ui administers with.
type Directory interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
	CheckPassword(dn, password string) error
}

// Config of authentication.
type Config struct {
	AdminGroups    []string      // members are administrators
	OperatorGroups []string      // members are operators
	IdleTimeout    time.Duration // a session ends after this long without requests
	MaxAge         time.Duration // and at the latest after this long
}

type session struct {
	actor   actor.Actor
	created time.Time
	seen    time.Time
}

// Service authenticates users and keeps their sessions.
type Service struct {
	cfg      Config
	dir      Directory
	now      func() time.Time
	mu       sync.Mutex
	sessions map[string]*session
	limiter  *limiter
}

// New creates the service.
func New(dir Directory, cfg Config) *Service {
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 30 * time.Minute
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 12 * time.Hour
	}
	return &Service{cfg: cfg, dir: dir, now: time.Now, sessions: map[string]*session{}, limiter: newLimiter()}
}

// Config returns the settings in use.
func (s *Service) Config() Config { return s.cfg }

// Login checks the credentials and opens a session. username is a uid, a DN
// or "Directory Manager".
func (s *Service) Login(ctx context.Context, username, password, client string) (*actor.Actor, string, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, "", ErrInvalidCredentials
	}
	key := strings.ToLower(username) + "|" + client
	if !s.limiter.allow(key, client, s.now()) {
		return nil, "", ErrTooManyAttempts
	}
	a, err := s.authenticate(ctx, username, password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			s.limiter.fail(key, client, s.now())
		}
		return nil, "", err
	}
	s.limiter.reset(key)
	id, err := newID()
	if err != nil {
		return nil, "", err
	}
	now := s.now()
	s.mu.Lock()
	s.sessions[id] = &session{actor: *a, created: now, seen: now}
	s.mu.Unlock()
	return a, id, nil
}

func (s *Service) authenticate(ctx context.Context, username, password string) (*actor.Actor, error) {
	var rootDN, dn, name string
	var memberOf []string
	err := s.dir.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"nsslapd-rootdn"}, nil))
		if err != nil {
			return err
		}
		rootDN = res.Entries[0].GetAttributeValue("nsslapd-rootdn")
		switch {
		case strings.EqualFold(username, "directory manager") || strings.EqualFold(username, rootDN):
			dn, name = rootDN, "Directory Manager"
		case strings.Contains(username, "="):
			if _, err := ldap.ParseDN(username); err != nil {
				return ErrInvalidCredentials
			}
			dn = username
		default:
			if dn, err = findUID(c, username); err != nil {
				return err
			}
		}
		if ldapx.NormDN(dn) == ldapx.NormDN(rootDN) {
			return nil
		}
		r, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"cn", "displayName", "memberOf"}, nil))
		if err != nil || len(r.Entries) == 0 {
			return ErrInvalidCredentials
		}
		dn = r.Entries[0].DN
		name = r.Entries[0].GetAttributeValue("displayName")
		if name == "" {
			name = r.Entries[0].GetAttributeValue("cn")
		}
		memberOf = r.Entries[0].GetAttributeValues("memberOf")
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return nil, err
		}
		return nil, fmt.Errorf("look up %s: %w", username, err)
	}
	if err := s.dir.CheckPassword(dn, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) || ldap.IsErrorWithCode(err, ldap.LDAPResultUnwillingToPerform) ||
			ldap.IsErrorWithCode(err, ldap.LDAPResultConstraintViolation) {
			// Locked or expired accounts are refused by the server the same way.
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if ldapx.NormDN(dn) == ldapx.NormDN(rootDN) {
		return &actor.Actor{DN: rootDN, Name: name, Role: actor.RoleAdmin, Proxy: false}, nil
	}
	if name == "" {
		name = dn
	}
	role, err := s.roleOf(ctx, dn, memberOf)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, ErrNoAccess
	}
	return &actor.Actor{DN: dn, Name: name, Role: role, Proxy: true}, nil
}

// findUID finds the single entry with this uid in the naming contexts.
func findUID(c *ldap.Conn, uid string) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"namingContexts"}, nil))
	if err != nil {
		return "", err
	}
	var found []string
	for _, base := range res.Entries[0].GetAttributeValues("namingContexts") {
		r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2, 10, false,
			"(uid="+ldap.EscapeFilter(uid)+")", []string{"1.1"}, nil))
		if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			return "", err
		}
		if r != nil {
			for _, e := range r.Entries {
				found = append(found, e.DN)
			}
		}
	}
	if len(found) != 1 {
		return "", ErrInvalidCredentials // none, or ambiguous
	}
	return found[0], nil
}

// roleOf checks membership: direct member/uniqueMember of a configured group,
// or the group in memberOf (nested groups, if the MemberOf plug-in is on).
func (s *Service) roleOf(ctx context.Context, dn string, memberOf []string) (string, error) {
	in := func(groups []string) (bool, error) {
		for _, g := range groups {
			if slices.ContainsFunc(memberOf, func(m string) bool { return ldapx.NormDN(m) == ldapx.NormDN(g) }) {
				return true, nil
			}
			var ok bool
			err := s.dir.Do(ctx, func(c *ldap.Conn) error {
				f := "(|(member=" + ldap.EscapeFilter(dn) + ")(uniqueMember=" + ldap.EscapeFilter(dn) + "))"
				r, err := c.Search(ldap.NewSearchRequest(g, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false, f, []string{"1.1"}, nil))
				if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
					return nil
				}
				ok = err == nil && len(r.Entries) == 1
				return err
			})
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}
	if ok, err := in(s.cfg.AdminGroups); err != nil || ok {
		return actor.RoleAdmin, err
	}
	if ok, err := in(s.cfg.OperatorGroups); err != nil || ok {
		return actor.RoleOperator, err
	}
	return "", nil
}

func newID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Session returns the user of a session and extends it.
func (s *Service) Session(id string) (*actor.Actor, error) {
	if id == "" {
		return nil, ErrNoSession
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok {
		return nil, ErrNoSession
	}
	if now.Sub(ss.seen) > s.cfg.IdleTimeout || now.Sub(ss.created) > s.cfg.MaxAge {
		delete(s.sessions, id)
		return nil, ErrNoSession
	}
	ss.seen = now
	a := ss.actor
	return &a, nil
}

// Logout ends a session.
func (s *Service) Logout(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

// --- login rate limiting ----------------------------------------------------------

const (
	limitWindow  = 5 * time.Minute
	limitPerUser = 5  // failures per user name and client in the window
	limitPerIP   = 20 // failures per client in the window
)

type limiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newLimiter() *limiter { return &limiter{failures: map[string][]time.Time{}} }

func (l *limiter) recent(key string, now time.Time) []time.Time {
	var keep []time.Time
	for _, t := range l.failures[key] {
		if now.Sub(t) < limitWindow {
			keep = append(keep, t)
		}
	}
	if keep == nil {
		delete(l.failures, key)
	} else {
		l.failures[key] = keep
	}
	return keep
}

func (l *limiter) allow(key, client string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) < limitPerUser && len(l.recent("ip|"+client, now)) < limitPerIP
}

func (l *limiter) fail(key, client string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.recent(key, now), now)
	l.failures["ip|"+client] = append(l.recent("ip|"+client, now), now)
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.failures, key)
	l.mu.Unlock()
}

// --- authorization ----------------------------------------------------------------

// Paths of directory data: run with Proxied Authorization, open to every
// role (the server's ACIs decide).
var dataPaths = []string{"/api/v1/entries", "/api/v1/search", "/api/v1/ldif/apply", "/api/v1/export", "/api/v1/aci/effective-rights"}

// Paths operators may not read: files with keys or full data.
var adminOnlyReads = []func(path string) bool{
	func(p string) bool {
		return strings.HasPrefix(p, "/api/v1/backups/") && strings.HasSuffix(p, "/download")
	},
	func(p string) bool { return strings.HasPrefix(p, "/api/v1/ldif-files/") },
}

func under(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/") || strings.HasPrefix(path, prefix+"?")
}

// Allowed reports whether a role may call method on path. Administrators
// may do everything; operators read the administration pages and work with
// directory data as their ACIs allow.
func Allowed(role, method, path string) bool {
	if role == actor.RoleAdmin {
		return true
	}
	if role != actor.RoleOperator {
		return false
	}
	for _, p := range dataPaths {
		if under(path, p) {
			return true
		}
	}
	if method != "GET" && method != "HEAD" {
		return false
	}
	for _, f := range adminOnlyReads {
		if f(path) {
			return false
		}
	}
	return true
}
