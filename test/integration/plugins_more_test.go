//go:build integration

package integration

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// TestUniquenessInstancesAndPassThru adds an Attribute Uniqueness instance,
// configures it with the shared page and removes it; then sets and reads the
// Pass-Through Authentication servers.
func TestUniquenessInstancesAndPassThru(t *testing.T) {
	c := newClient(t)
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	bus := &events.Bus{}
	h := api.NewHandler(api.Deps{LDAP: c, Pages: pages.New(defs, nil), Tasks: ds.NewTasks(c, bus, nil), Events: bus,
		Restart: &ds.RestartRegistry{}, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}

	code, body := do("POST", "/api/v1/plugins/uniqueness", js(map[string]any{"name": "itest mail uniqueness", "attributes": []string{"mail"}, "subtrees": []string{suffix}}))
	var p ds.Plugin
	if code != 201 || json.Unmarshal([]byte(body), &p) != nil || p.Enabled || p.Path != ds.UniquenessPath {
		t.Fatalf("create: %d %s", code, body)
	}
	t.Cleanup(func() {
		do("PATCH", "/api/v1/plugins/itest%20mail%20uniqueness", `{"enabled":false}`)
		do("DELETE", "/api/v1/plugins/itest%20mail%20uniqueness", "")
	})
	if code, _ := do("POST", "/api/v1/plugins/uniqueness", js(map[string]any{"name": "itest mail uniqueness", "attributes": []string{"mail"}})); code != 409 {
		t.Errorf("create twice: %d", code)
	}

	// The shared page edits this instance and the shipped one, nothing else.
	page := "/api/v1/pages/plugin-uniqueness?dn="
	code, body = do("GET", page+url.QueryEscape(p.DN), "")
	if code != 200 || !strings.Contains(body, `"uniqueness-attribute-name":["mail"]`) {
		t.Errorf("page of the instance: %d %s", code, body)
	}
	if code, body := do("PATCH", page+url.QueryEscape(p.DN), `{"values":{"uniqueness-attribute-name":["mail","telephoneNumber"]}}`); code != 200 {
		t.Errorf("change the instance: %d %s", code, body)
	}
	if code, _ := do("GET", page+url.QueryEscape("cn=attribute uniqueness,cn=plugins,cn=config"), ""); code != 200 {
		t.Errorf("page of the shipped instance: %d", code)
	}
	if code, _ := do("GET", page+url.QueryEscape("cn=Retro Changelog Plugin,cn=plugins,cn=config"), ""); code != 404 {
		t.Errorf("page of another plug-in: %d", code)
	}

	// Deletion: never the shipped instance, an enabled one only after disabling.
	if code, _ := do("DELETE", "/api/v1/plugins/attribute%20uniqueness", ""); code != 409 {
		t.Errorf("delete the shipped instance: %d", code)
	}
	if code, _ := do("DELETE", "/api/v1/plugins/Retro%20Changelog%20Plugin", ""); code != 422 {
		t.Errorf("delete another plug-in: %d", code)
	}
	do("PATCH", "/api/v1/plugins/itest%20mail%20uniqueness", `{"enabled":true}`)
	if code, _ := do("DELETE", "/api/v1/plugins/itest%20mail%20uniqueness", ""); code != 422 {
		t.Errorf("delete an enabled instance: %d", code)
	}
	do("PATCH", "/api/v1/plugins/itest%20mail%20uniqueness", `{"enabled":false}`)
	if code, body := do("DELETE", "/api/v1/plugins/itest%20mail%20uniqueness", ""); code != 204 {
		t.Errorf("delete: %d %s", code, body)
	}

	// Pass-through servers are plug-in arguments.
	servers := `[{"secure":false,"hosts":["ldap1.corp.example","ldap2.corp.example:1389"],"subtree":"ou=Remote,dc=corp,dc=example",` +
		`"maxConns":3,"maxOps":5,"timeout":60,"ldapVersion":3,"connLifetime":300,"startTLS":true},` +
		`{"secure":true,"hosts":["ad.corp.example:636"],"subtree":"o=Partner Org","maxConns":2,"maxOps":5,"timeout":30,"ldapVersion":3,"connLifetime":0,"startTLS":false}]`
	if code, body := do("PUT", "/api/v1/plugins/passthru/servers", servers); code != 200 {
		t.Fatalf("set servers: %d %s", code, body)
	}
	t.Cleanup(func() { do("PUT", "/api/v1/plugins/passthru/servers", `[]`) })
	code, body = do("GET", "/api/v1/plugins/passthru/servers", "")
	var got []ds.PassThruServer
	if code != 200 || json.Unmarshal([]byte(body), &got) != nil || len(got) != 2 ||
		got[0].Hosts[1] != "ldap2.corp.example:1389" || !got[0].StartTLS || got[1].Subtree != "o=Partner Org" || !got[1].Secure {
		t.Errorf("servers: %d %s", code, body)
	}
	if code, _ := do("PUT", "/api/v1/plugins/passthru/servers", `[{"secure":true,"hosts":["x"],"subtree":"o=x","maxConns":1,"maxOps":1,"timeout":1,"ldapVersion":3,"connLifetime":0,"startTLS":true}]`); code != 422 {
		t.Errorf("StartTLS with ldaps: %d", code)
	}
	if code, body := do("PUT", "/api/v1/plugins/passthru/servers", `[]`); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Errorf("clear: %d %s", code, body)
	}
}
