//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestRolesAndCoS(t *testing.T) {
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}
	ctx := context.Background()
	people := "ou=People," + suffix
	jdoe, asmith := "uid=jdoe,"+people, "uid=asmith,"+people
	q := func(dn string) string { return "?dn=" + url.QueryEscape(dn) }
	attr := func(dn, name string) []string {
		var out []string
		err := c.Do(ctx, func(conn *ldap.Conn) error {
			r, err := conn.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
				"(objectClass=*)", []string{name}, nil))
			if err != nil {
				return err
			}
			out = r.Entries[0].GetAttributeValues(name)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	// CoS and role values are computed from caches the server rebuilds in
	// the background.
	eventually := func(what string, ok func() bool) {
		t.Helper()
		for i := 0; i < 50; i++ {
			if ok() {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Errorf("timed out waiting for %s", what)
	}
	created := func(path, body string) string {
		t.Helper()
		code, out := do("POST", path, body)
		var x struct{ DN string }
		if code != 201 || json.Unmarshal([]byte(out), &x) != nil {
			t.Fatalf("POST %s: %d %s", path, code, out)
		}
		return x.DN
	}
	hasNorm := func(list []string, dn string) bool {
		return slices.ContainsFunc(list, func(v string) bool { return ldapx.NormDN(v) == ldapx.NormDN(dn) })
	}

	// Roles.
	managed := created("/api/v1/roles", `{"parent":"`+people+`","name":"itest managed","kind":"managed","description":"m"}`)
	defer do("DELETE", "/api/v1/roles"+q(managed), "")
	filtered := created("/api/v1/roles", `{"parent":"`+people+`","name":"itest filtered","kind":"filtered","filter":"(uid=a*)"}`)
	defer do("DELETE", "/api/v1/roles"+q(filtered), "")
	nested := created("/api/v1/roles", `{"parent":"`+people+`","name":"itest nested","kind":"nested","roles":["`+managed+`","`+filtered+`"]}`)
	defer do("DELETE", "/api/v1/roles"+q(nested), "")
	if code, _ := do("POST", "/api/v1/roles", `{"parent":"`+people+`","name":"bad","kind":"nested","roles":["`+people+`"]}`); code != 422 {
		t.Errorf("nested role with a non-role: %d", code)
	}

	if code, body := do("POST", "/api/v1/roles/members"+q(managed), `{"member":"`+jdoe+`","add":true}`); code != 204 {
		t.Fatalf("add member: %d %s", code, body)
	}
	defer do("POST", "/api/v1/roles/members"+q(managed), `{"member":"`+jdoe+`","add":false}`)
	if code, _ := do("POST", "/api/v1/roles/members"+q(filtered), `{"member":"`+jdoe+`","add":true}`); code != 422 {
		t.Errorf("member of a filtered role: %d", code)
	}
	eventually("nsRole of jdoe", func() bool {
		r := attr(jdoe, "nsRole")
		return hasNorm(r, managed) && hasNorm(r, nested)
	})
	eventually("nsRole of asmith", func() bool {
		r := attr(asmith, "nsRole")
		return hasNorm(r, filtered) && hasNorm(r, nested) && !hasNorm(r, managed)
	})
	code, body := do("GET", "/api/v1/roles/members"+q(nested), "")
	var members api.RoleMembers
	if code != 200 || json.Unmarshal([]byte(body), &members) != nil || !hasNorm(members.Members, jdoe) || !hasNorm(members.Members, asmith) {
		t.Errorf("nested members: %d %s", code, body)
	}
	if code, body := do("PATCH", "/api/v1/roles"+q(filtered), `{"filter":"(uid=b*)"}`); code != 200 || !strings.Contains(body, `"(uid=b*)"`) {
		t.Errorf("change filter: %d %s", code, body)
	}
	code, body = do("GET", "/api/v1/roles", "")
	var roles []ds.Role
	if code != 200 || json.Unmarshal([]byte(body), &roles) != nil || len(roles) < 3 {
		t.Errorf("list roles: %d %s", code, body)
	}

	// Pointer CoS: one template for the whole subtree.
	pointerTpl := "cn=itest pointer template," + people
	if code, body := do("PUT", "/api/v1/cos/templates"+q(pointerTpl), `{"priority":1,"values":{"postalCode":["12345"]}}`); code != 204 {
		t.Fatalf("pointer template: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/cos/templates"+q(pointerTpl), "")
	pointer := created("/api/v1/cos", `{"parent":"`+people+`","name":"itest pointer","kind":"pointer","attributes":["postalCode default"],"templateDN":"`+pointerTpl+`"}`)
	defer do("DELETE", "/api/v1/cos"+q(pointer), "")
	eventually("postalCode from pointer CoS", func() bool { return slices.Contains(attr(asmith, "postalCode"), "12345") })

	// Classic CoS: the template is chosen by employeeType.
	if code, body := do("PUT", "/api/v1/cos/templates"+q("cn=contractor,"+people), `{"values":{"businessCategory":["external"]}}`); code != 204 {
		t.Fatalf("classic template: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/cos/templates"+q("cn=contractor,"+people), "")
	classic := created("/api/v1/cos", `{"parent":"`+people+`","name":"itest classic","kind":"classic","attributes":["businessCategory default"],"templateDN":"`+people+`","specifier":"employeeType"}`)
	defer do("DELETE", "/api/v1/cos"+q(classic), "")
	err := c.Do(ctx, func(conn *ldap.Conn) error {
		req := ldap.NewModifyRequest(jdoe, nil)
		req.Replace("employeeType", []string{"contractor"})
		return conn.Modify(req)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Do(ctx, func(conn *ldap.Conn) error {
		req := ldap.NewModifyRequest(jdoe, nil)
		req.Delete("employeeType", nil)
		return conn.Modify(req)
	})
	eventually("businessCategory from classic CoS", func() bool { return slices.Contains(attr(jdoe, "businessCategory"), "external") })

	code, body = do("GET", "/api/v1/cos", "")
	var defs []ds.CoSDefinition
	if code != 200 || json.Unmarshal([]byte(body), &defs) != nil {
		t.Fatalf("list CoS: %d %s", code, body)
	}
	for _, d := range defs {
		if d.Name == "itest pointer" && (d.Kind != ds.CoSPointer || len(d.Templates) != 1 || d.Templates[0].Values["postalCode"][0] != "12345") {
			t.Errorf("pointer definition: %+v", d)
		}
		if d.Name == "itest classic" && (d.Kind != ds.CoSClassic || !slices.ContainsFunc(d.Templates, func(x ds.CoSTemplate) bool { return x.Name == "contractor" })) {
			t.Errorf("classic definition: %+v", d)
		}
	}
	if code, _ := do("POST", "/api/v1/cos", `{"parent":"`+people+`","name":"bad","kind":"pointer","attributes":["postalCode sometimes"],"templateDN":"`+pointerTpl+`"}`); code != 422 {
		t.Errorf("bad qualifier: %d", code)
	}
}
