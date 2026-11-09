package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

type fakeLDAP struct{ err error }

func (f fakeLDAP) Do(_ context.Context, _ func(*ldap.Conn) error) error { return f.err }

func newTestHandler(l LDAP) http.Handler {
	return NewHandler(Deps{
		LDAP:           l,
		Lifecycle:      providers.NoLifecycle{},
		Capabilities:   providers.Capabilities{Mode: "remote", LDAPAuth: "simple", Lifecycle: "none"},
		AllowedOrigins: []string{"http://127.0.0.1:8080"},
		Static: fstest.MapFS{
			"index.html":      {Data: []byte("<html>app</html>")},
			"assets/app-1.js": {Data: []byte("js")},
			"favicon.svg":     {Data: []byte("<svg/>")},
		},
	})
}

func do(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Host = "127.0.0.1:8080"
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthAndReady(t *testing.T) {
	h := newTestHandler(fakeLDAP{})
	if rec := do(h, "GET", "/healthz", nil); rec.Code != 200 {
		t.Errorf("healthz = %d", rec.Code)
	}
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 {
		t.Errorf("readyz = %d", rec.Code)
	}
	down := newTestHandler(fakeLDAP{err: &ldapx.ConnError{Op: "dial", Err: errors.New("refused")}})
	if rec := do(down, "GET", "/readyz", nil); rec.Code != 503 {
		t.Errorf("readyz with LDAP down = %d", rec.Code)
	}
	if rec := do(down, "GET", "/healthz", nil); rec.Code != 200 {
		t.Errorf("healthz must not depend on LDAP, got %d", rec.Code)
	}
}

func TestSystemState(t *testing.T) {
	down := newTestHandler(fakeLDAP{err: &ldapx.ConnError{Op: "dial", Err: errors.New("refused")}})
	rec := do(down, "GET", "/api/v1/system/state", nil)
	if rec.Code != 200 {
		t.Fatalf("state = %d %s", rec.Code, rec.Body)
	}
	var st SystemState
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.LDAP != "unreachable" || st.Server != "unknown" {
		t.Errorf("state = %+v", st)
	}

	rec = do(newTestHandler(fakeLDAP{}), "GET", "/api/v1/system/state", nil)
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.LDAP != "ok" || st.Server != "running" {
		t.Errorf("state = %+v", st)
	}
}

func TestSystemInfoLDAPDown(t *testing.T) {
	h := newTestHandler(fakeLDAP{err: &ldapx.ConnError{Op: "dial", Err: errors.New("refused")}})
	rec := do(h, "GET", "/api/v1/system/info", nil)
	if rec.Code != 503 || !strings.Contains(rec.Header().Get("Content-Type"), "problem+json") {
		t.Errorf("info = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestCapabilities(t *testing.T) {
	rec := do(newTestHandler(fakeLDAP{}), "GET", "/api/v1/system/capabilities", nil)
	var c providers.Capabilities
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil || c.Mode != "remote" {
		t.Errorf("capabilities = %s (%v)", rec.Body, err)
	}
}

func TestCSRFGuard(t *testing.T) {
	h := newTestHandler(fakeLDAP{})
	// Unknown endpoint, but the guard runs first.
	if rec := do(h, "POST", "/api/v1/anything", nil); rec.Code != 403 {
		t.Errorf("POST without header = %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/anything", map[string]string{
		CSRFHeader: "ds-ui", "Origin": "http://evil.example",
	}); rec.Code != 403 {
		t.Errorf("POST from foreign origin = %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/anything", map[string]string{
		CSRFHeader: "ds-ui", "Origin": "http://127.0.0.1:8080",
	}); rec.Code == 403 {
		t.Errorf("POST from allowed origin was rejected")
	}
}

func TestHostGuard(t *testing.T) {
	h := newTestHandler(fakeLDAP{})
	req := httptest.NewRequest("GET", "/api/v1/system/capabilities", nil)
	req.Host = "evil.example:8080" // DNS rebinding: hostile name resolving to 127.0.0.1
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMisdirectedRequest {
		t.Errorf("foreign Host = %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Host = "172.18.0.3:8080"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("healthz must stay reachable by any host, got %d", rec.Code)
	}
	req = httptest.NewRequest("GET", "/api/v1/system/capabilities", nil)
	req.Host = "127.0.0.1:8080"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("allowed Host = %d", rec.Code)
	}
}

func TestSPA(t *testing.T) {
	h := newTestHandler(fakeLDAP{})
	for _, p := range []string{"/", "/browser/dc=example", "/index.html"} {
		rec := do(h, "GET", p, nil)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "app") {
			t.Errorf("%s = %d %q", p, rec.Code, rec.Body)
		}
	}
	rec := do(h, "GET", "/assets/app-1.js", nil)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset = %d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(h, "GET", "/assets/missing.js", nil); rec.Code != 404 {
		t.Errorf("missing asset = %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/nope", nil); rec.Code != 404 || !strings.Contains(rec.Header().Get("Content-Type"), "problem") {
		t.Errorf("unknown API path = %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestOpenAPI(t *testing.T) {
	b, err := OpenAPI("test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"getSystemInfo", "getCapabilities", "getSystemState", `"3.1`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("OpenAPI document lacks %q", want)
		}
	}
}
