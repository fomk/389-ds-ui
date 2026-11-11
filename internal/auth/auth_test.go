package auth

import (
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/actor"
)

func TestAllowed(t *testing.T) {
	cases := []struct {
		role, method, path string
		want               bool
	}{
		{actor.RoleAdmin, "DELETE", "/api/v1/backends/userroot", true},
		{actor.RoleOperator, "GET", "/api/v1/monitor", true},
		{actor.RoleOperator, "GET", "/api/v1/pages/server-general", true},
		{actor.RoleOperator, "PATCH", "/api/v1/pages/server-general", false},
		{actor.RoleOperator, "POST", "/api/v1/system/restart", false},
		{actor.RoleOperator, "PATCH", "/api/v1/entries", true},
		{actor.RoleOperator, "POST", "/api/v1/entries/rename", true},
		{actor.RoleOperator, "POST", "/api/v1/search", true},
		{actor.RoleOperator, "POST", "/api/v1/ldif/apply", true},
		{actor.RoleOperator, "POST", "/api/v1/entriesx", false},
		{actor.RoleOperator, "GET", "/api/v1/backups/b1/download", false},
		{actor.RoleOperator, "GET", "/api/v1/backups", true},
		{actor.RoleOperator, "GET", "/api/v1/ldif-files", true},
		{actor.RoleOperator, "GET", "/api/v1/ldif-files/x.ldif", false},
		{"", "GET", "/api/v1/monitor", false},
	}
	for _, c := range cases {
		if got := Allowed(c.role, c.method, c.path); got != c.want {
			t.Errorf("%s %s %s = %v", c.role, c.method, c.path, got)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter()
	now := time.Now()
	for i := 0; i < limitPerUser; i++ {
		if !l.allow("u|ip", "ip", now) {
			t.Fatalf("blocked after %d failures", i)
		}
		l.fail("u|ip", "ip", now)
	}
	if l.allow("u|ip", "ip", now) {
		t.Error("not blocked")
	}
	if !l.allow("other|ip", "ip", now) {
		t.Error("another user from the same client is blocked too early")
	}
	if !l.allow("u|ip", "ip", now.Add(limitWindow+time.Second)) {
		t.Error("still blocked after the window")
	}
}

func TestSessionExpiry(t *testing.T) {
	s := New(nil, Config{IdleTimeout: time.Minute, MaxAge: time.Hour})
	now := time.Now()
	s.now = func() time.Time { return now }
	s.sessions["a"] = &session{actor: actor.Actor{DN: "uid=a"}, created: now, seen: now}
	now = now.Add(50 * time.Second)
	if _, err := s.Session("a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(50 * time.Second) // 50 s since the last request: still alive
	if _, err := s.Session("a"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := s.Session("a"); err == nil {
		t.Error("idle session survived")
	}
	s.sessions["b"] = &session{actor: actor.Actor{DN: "uid=b"}, created: now.Add(-2 * time.Hour), seen: now}
	if _, err := s.Session("b"); err == nil {
		t.Error("session older than MaxAge survived")
	}
}
