//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestSchemaWrites(t *testing.T) {
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Schemas: schema.NewCache(0, nil), Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		req := httptestRequest(method, path, body)
		rec := serve(h, req)
		return rec.Code, rec.Body.String()
	}
	const at = `{"oid":"1.3.6.1.4.1.99999.1.1","names":["itestBadge"],"desc":"Badge number","equality":"caseIgnoreMatch","syntax":"1.3.6.1.4.1.1466.115.121.1.15","singleValue":true}`
	const oc = `{"oid":"1.3.6.1.4.1.99999.2.1","names":["itestBadgeHolder"],"kind":"auxiliary","sup":["top"],"must":[],"may":["itestBadge"]}`

	if code, body := do("POST", "/api/v1/schema/attributetypes", at); code != 201 {
		t.Fatalf("add attribute type: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/schema/attributetypes/itestBadge", "")
	if code, _ := do("POST", "/api/v1/schema/attributetypes", at); code != 409 {
		t.Errorf("duplicate: %d", code)
	}
	if code, body := do("POST", "/api/v1/schema/attributetypes", `{"oid":"x","names":["y"]}`); code != 422 {
		t.Errorf("invalid: %d %s", code, body)
	}
	if code, body := do("POST", "/api/v1/schema/objectclasses", oc); code != 201 {
		t.Fatalf("add object class: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/schema/objectclasses/itestBadgeHolder", "")

	// The cache is invalidated: GET /schema shows both.
	if code, body := do("GET", "/api/v1/schema", ""); code != 200 || !strings.Contains(body, `"itestBadge"`) || !strings.Contains(body, `"itestBadgeHolder"`) {
		t.Errorf("schema after add: %d", code)
	}

	// Change the description; the OID stays.
	if code, body := do("PUT", "/api/v1/schema/attributetypes/itestBadge",
		`{"oid":"ignored","names":["itestBadge"],"desc":"Badge ID","equality":"caseIgnoreMatch","syntax":"1.3.6.1.4.1.1466.115.121.1.15","singleValue":true}`); code != 204 {
		t.Errorf("replace: %d %s", code, body)
	}
	err := c.Do(context.Background(), func(conn *ldap.Conn) error {
		s, _, _, err := schema.Fetch(conn)
		if err != nil {
			return err
		}
		a := s.Attr("itestBadge")
		if a == nil || a.Desc != "Badge ID" || a.OID != "1.3.6.1.4.1.99999.1.1" || !schema.UserDefined(a.Origin) {
			t.Errorf("after replace: %+v", a)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// In use by an entry and by the class.
	dn := "uid=jdoe,ou=People," + suffix
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		req := ldap.NewModifyRequest(dn, nil)
		req.Add("objectClass", []string{"itestBadgeHolder"})
		req.Add("itestBadge", []string{"42"})
		return conn.Modify(req)
	})
	if err != nil {
		t.Fatalf("use the class: %v", err)
	}
	if code, body := do("DELETE", "/api/v1/schema/objectclasses/itestBadgeHolder", ""); code != 409 || !strings.Contains(body, "jdoe") {
		t.Errorf("delete class in use: %d %s", code, body)
	}
	if code, body := do("DELETE", "/api/v1/schema/attributetypes/itestBadge", ""); code != 409 || !strings.Contains(body, "itestBadgeHolder") {
		t.Errorf("delete attribute used by a class: %d %s", code, body)
	}
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		req := ldap.NewModifyRequest(dn, nil)
		req.Delete("itestBadge", nil)
		req.Delete("objectClass", []string{"itestBadgeHolder"})
		return conn.Modify(req)
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, body := do("DELETE", "/api/v1/schema/objectclasses/itestBadgeHolder", ""); code != 204 {
		t.Errorf("delete class: %d %s", code, body)
	}
	if code, body := do("DELETE", "/api/v1/schema/attributetypes/itestBadge", ""); code != 204 {
		t.Errorf("delete attribute: %d %s", code, body)
	}
	// Built-in definitions are not touched.
	if code, _ := do("DELETE", "/api/v1/schema/attributetypes/cn", ""); code != 409 {
		t.Errorf("delete cn: %d", code)
	}
	if code, _ := do("DELETE", "/api/v1/schema/attributetypes/itestBadge", ""); code != 404 {
		t.Errorf("delete again: %d", code)
	}
}
