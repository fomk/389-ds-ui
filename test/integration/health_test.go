//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/health"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// TestHealthCheck runs the checks on the test server, then weakens a few
// settings and expects them to be reported.
func TestHealthCheck(t *testing.T) {
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Lifecycle: providers.NoLifecycle{}})
	run := func() health.Report {
		t.Helper()
		rec := serve(h, httptestRequest("GET", "/api/v1/health", ""))
		var r health.Report
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &r) != nil {
			t.Fatalf("health: %d %s", rec.Code, rec.Body)
		}
		if len(r.Failed) > 0 {
			t.Errorf("checks failed: %+v", r.Failed)
		}
		return r
	}
	ids := func(r health.Report) []string {
		out := []string{}
		for _, f := range r.Findings {
			out = append(out, f.ID)
		}
		return out
	}

	r := run()
	if r.Checks < 10 || !slices.Contains(ids(r), "anonymous-access") {
		t.Errorf("fresh server: %d checks, %v", r.Checks, ids(r))
	}

	set := func(dn string, values map[string]string) {
		t.Helper()
		err := c.Do(context.Background(), func(conn *ldap.Conn) error {
			req := ldap.NewModifyRequest(dn, nil)
			for a, v := range values {
				req.Replace(a, []string{v})
			}
			return conn.Modify(req)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	set("cn=config", map[string]string{"passwordStorageScheme": "SSHA512", "nsslapd-errorlog-level": "16385"})
	defer set("cn=config", map[string]string{"passwordStorageScheme": "PBKDF2-SHA512", "nsslapd-errorlog-level": "16384"})
	set("cn=encryption,cn=config", map[string]string{"allowWeakCipher": "on"})
	defer set("cn=encryption,cn=config", map[string]string{"allowWeakCipher": "off"})

	r = run()
	got := ids(r)
	for _, want := range []string{"password-scheme-fast", "error-log-debug"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	// Weak ciphers count only while TLS is on.
	sec := ""
	_ = c.Do(context.Background(), func(conn *ldap.Conn) error {
		res, err := conn.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"nsslapd-security"}, nil))
		if err == nil {
			sec = res.Entries[0].GetAttributeValue("nsslapd-security")
		}
		return err
	})
	if sec == "on" && !slices.Contains(got, "tls-weak-ciphers") {
		t.Errorf("missing tls-weak-ciphers in %v", got)
	}
	// Most serious first.
	for i := 1; i < len(r.Findings); i++ {
		if rank(r.Findings[i-1].Severity) > rank(r.Findings[i].Severity) {
			t.Errorf("order: %v", got)
			break
		}
	}
}

func rank(s string) int { return map[string]int{health.Error: 0, health.Warning: 1, health.Info: 2}[s] }
