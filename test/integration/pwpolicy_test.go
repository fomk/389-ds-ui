//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestLocalPasswordPolicies(t *testing.T) {
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Pages: pages.New(defs, nil), Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}
	people := "ou=People," + suffix
	user := "uid=asmith," + people

	// Subtree policy.
	code, body := do("POST", "/api/v1/pwpolicies", `{"kind":"subtree","target":"`+people+`"}`)
	var created ds.LocalPolicyCreated
	if code != 201 || json.Unmarshal([]byte(body), &created) != nil {
		t.Fatalf("create subtree: %d %s", code, body)
	}
	subtreeDN := created.Policy.DN
	defer do("DELETE", "/api/v1/pwpolicies?dn="+url.QueryEscape(subtreeDN), "")
	if code, _ := do("POST", "/api/v1/pwpolicies", `{"kind":"subtree","target":"`+people+`"}`); code != 409 {
		t.Errorf("duplicate: %d", code)
	}
	if code, _ := do("POST", "/api/v1/pwpolicies", `{"kind":"subtree","target":"ou=Nowhere,`+suffix+`"}`); code != 404 {
		t.Errorf("missing target: %d", code)
	}

	// User policy.
	code, body = do("POST", "/api/v1/pwpolicies", `{"kind":"user","target":"`+user+`"}`)
	if code != 201 || json.Unmarshal([]byte(body), &created) != nil {
		t.Fatalf("create user: %d %s", code, body)
	}
	userDN := created.Policy.DN
	defer do("DELETE", "/api/v1/pwpolicies?dn="+url.QueryEscape(userDN), "")

	code, body = do("GET", "/api/v1/pwpolicies", "")
	var list []ds.LocalPolicy
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list) != 2 {
		t.Fatalf("list: %d %s", code, body)
	}
	if list[0].Kind != "subtree" || !strings.EqualFold(list[0].Target, people) || list[1].Kind != "user" {
		t.Errorf("list = %+v", list)
	}

	// Edit through the settings page bound to the policy entry.
	page := "/api/v1/pages/password-policy-local?dn=" + url.QueryEscape(subtreeDN)
	if code, body := do("PATCH", page, `{"values":{"passwordMinLength":["12"],"passwordCheckSyntax":["on"]}}`); code != 200 ||
		!strings.Contains(body, `"passwordMinLength":["12"]`) {
		t.Errorf("patch policy: %d %s", code, body)
	}
	if code, _ := do("GET", "/api/v1/pages/password-policy-local", ""); code != 422 {
		t.Errorf("page without dn: %d", code)
	}
	if code, _ := do("GET", "/api/v1/pages/password-policy-local?dn="+url.QueryEscape(people), ""); code != 404 {
		t.Errorf("page on a non-policy entry: %d", code)
	}

	// The policies are in effect: CoS for the subtree, the pointer for the user.
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		for dn, want := range map[string]string{"uid=jdoe," + people: subtreeDN, user: userDN} {
			r, err := conn.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
				"(objectClass=*)", []string{"pwdpolicysubentry"}, nil))
			if err != nil {
				return err
			}
			if got := r.Entries[0].GetAttributeValue("pwdpolicysubentry"); ldapx.NormDN(got) != ldapx.NormDN(want) {
				t.Errorf("%s: pwdpolicysubentry = %q, want %q", dn, got, want)
			}
		}
		r, err := conn.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"nsslapd-pwpolicy-local"}, nil))
		if err != nil {
			return err
		}
		if v := r.Entries[0].GetAttributeValue("nsslapd-pwpolicy-local"); v != "on" {
			t.Errorf("nsslapd-pwpolicy-local = %q", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Delete both; the containers go away with the last policy.
	for _, dn := range []string{userDN, subtreeDN} {
		if code, body := do("DELETE", "/api/v1/pwpolicies?dn="+url.QueryEscape(dn), ""); code != 204 {
			t.Errorf("delete %s: %d %s", dn, code, body)
		}
	}
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		r, err := conn.Search(ldap.NewSearchRequest(suffix, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			"(|(cn=nsPwPolicyContainer)(objectClass=cosPointerDefinition)(objectClass=costemplate)(objectClass=passwordPolicy))", []string{"1.1"}, nil))
		if err != nil {
			return err
		}
		if len(r.Entries) != 0 {
			t.Errorf("left over: %d entries, first %s", len(r.Entries), r.Entries[0].DN)
		}
		r, err = conn.Search(ldap.NewSearchRequest(user, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"pwdpolicysubentry"}, nil))
		if err != nil {
			return err
		}
		if v := r.Entries[0].GetAttributeValue("pwdpolicysubentry"); v != "" {
			t.Errorf("user still points to %s", v)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
