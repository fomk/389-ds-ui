//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/auth"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// TestSignInAndProxiedAccess signs users in with their directory accounts
// and checks roles and that directory data is accessed as the user.
func TestSignInAndProxiedAccess(t *testing.T) {
	ctx := context.Background()
	c := newClient(t)
	people, groups := "ou=People,"+suffix, "ou=Groups,"+suffix
	admins, operators := "cn=ds-ui admins,"+groups, "cn=ds-ui operators,"+groups
	asmith, bjones, jdoe := "uid=asmith,"+people, "uid=bjones,"+people, "uid=jdoe,"+people

	err := c.Do(ctx, func(conn *ldap.Conn) error {
		for dn, member := range map[string]string{admins: asmith, operators: bjones} {
			req := ldap.NewAddRequest(dn, nil)
			req.Attribute("objectClass", []string{"groupOfNames"})
			req.Attribute("cn", []string{strings.TrimPrefix(strings.Split(dn, ",")[0], "cn=")})
			req.Attribute("member", []string{member})
			if err := conn.Add(req); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = c.Do(context.Background(), func(conn *ldap.Conn) error {
			_ = conn.Del(ldap.NewDelRequest(admins, nil))
			_ = conn.Del(ldap.NewDelRequest(operators, nil))
			return nil
		})
	})

	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	schemas := schema.NewCache(0, nil)
	bus := &events.Bus{}
	svc := auth.New(c, auth.Config{AdminGroups: []string{admins}, OperatorGroups: []string{operators}})
	h := api.NewHandler(api.Deps{LDAP: c, Schemas: schemas, Entries: entries.New(c, schemas, nil), Pages: pages.New(defs, nil),
		Tasks: ds.NewTasks(c, bus, nil), Events: bus, Auth: svc, Lifecycle: providers.NoLifecycle{}})
	do := func(cookie, method, path, body string) *http.Response {
		req := httptestRequest(method, path, body)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: api.SessionCookie, Value: cookie})
		}
		return serve(h, req).Result()
	}
	login := func(user, pw string) (string, int) {
		r := do("", "POST", "/api/v1/auth/login", `{"username":"`+user+`","password":"`+pw+`"}`)
		for _, ck := range r.Cookies() {
			if ck.Name == api.SessionCookie && ck.Value != "" {
				if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode {
					t.Errorf("cookie flags: %+v", ck)
				}
				return ck.Value, r.StatusCode
			}
		}
		return "", r.StatusCode
	}

	// Without a session only the auth endpoints answer.
	if r := do("", "GET", "/api/v1/monitor", ""); r.StatusCode != 401 {
		t.Errorf("no session: %d", r.StatusCode)
	}
	if r := do("", "GET", "/api/v1/auth/me", ""); r.StatusCode != 200 {
		t.Errorf("me without session: %d", r.StatusCode)
	}

	if _, code := login("asmith", "wrong"); code != 401 {
		t.Errorf("wrong password: %d", code)
	}
	if _, code := login("jdoe", "Secret123"); code != 403 {
		t.Errorf("account without a role: %d", code)
	}
	if _, code := login("nosuchuser", "x"); code != 401 {
		t.Errorf("unknown user: %d", code)
	}

	// Directory Manager: administrator, not proxied.
	dm, code := login("Directory Manager", dmPassword)
	if code != 200 || dm == "" {
		t.Fatalf("DM login: %d", code)
	}
	if r := do(dm, "GET", "/api/v1/monitor", ""); r.StatusCode != 200 {
		t.Errorf("DM monitor: %d", r.StatusCode)
	}

	// Operator: reads administration, may not change it.
	op, code := login("bjones", "Secret123")
	if code != 200 {
		t.Fatalf("operator login: %d", code)
	}
	r := do(op, "GET", "/api/v1/auth/me", "")
	var me api.Me
	_ = json.NewDecoder(r.Body).Decode(&me)
	if me.User == nil || me.User.Role != "operator" || !strings.EqualFold(me.User.DN, bjones) {
		t.Errorf("operator me: %+v", me)
	}
	if r := do(op, "GET", "/api/v1/pages/server-general", ""); r.StatusCode != 200 {
		t.Errorf("operator reads settings: %d", r.StatusCode)
	}
	if r := do(op, "PATCH", "/api/v1/pages/server-limits", `{"values":{"nsslapd-sizelimit":["2001"]}}`); r.StatusCode != 403 {
		t.Errorf("operator changes settings: %d", r.StatusCode)
	}

	// Data goes through the user's ACIs: without a grant the change is
	// refused even though ds-ui itself runs as Directory Manager...
	change := `{"changes":[{"op":"replace","attr":"description","values":["changed by operator"]}]}`
	if r := do(op, "PATCH", "/api/v1/entries?dn="+url.QueryEscape(jdoe), change); r.StatusCode != 403 {
		t.Errorf("operator writes without an ACI: %d", r.StatusCode)
	}
	// ...and with an ACI for the operators group it works, recorded as the user.
	read := `(targetattr="*")(version 3.0; acl "itest operators read"; allow (read, search, compare) groupdn="ldap:///` + operators + `";)`
	write := `(targetattr="description")(version 3.0; acl "itest operators write"; allow (write) groupdn="ldap:///` + operators + `";)`
	err = c.Do(ctx, func(conn *ldap.Conn) error { return ds.SetACIs(conn, people, []string{read, write}) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Do(context.Background(), func(conn *ldap.Conn) error { return ds.SetACIs(conn, people, nil) }) })
	if r := do(op, "PATCH", "/api/v1/entries?dn="+url.QueryEscape(jdoe), change); r.StatusCode != 200 {
		t.Errorf("operator writes with an ACI: %d", r.StatusCode)
	}
	// The change is made (the server records ds-ui's own bind DN in
	// modifiersName; the user is in the ds-ui history and logs).
	err = c.Do(ctx, func(conn *ldap.Conn) error {
		res, err := conn.Search(ldap.NewSearchRequest(jdoe, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"description"}, nil))
		if err != nil {
			return err
		}
		if got := res.Entries[0].GetAttributeValue("description"); got != "changed by operator" {
			t.Errorf("description = %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Administrator from the group.
	adm, code := login("asmith", "Secret123")
	if code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	if r := do(adm, "PATCH", "/api/v1/pages/server-limits", `{"values":{"nsslapd-sizelimit":["2000"]}}`); r.StatusCode != 200 {
		b, _ := io.ReadAll(r.Body)
		t.Errorf("admin changes settings: %d %s", r.StatusCode, b)
	}
	if r := do(adm, "POST", "/api/v1/accounts/lock?dn="+url.QueryEscape(jdoe), ""); r.StatusCode != 200 {
		t.Errorf("admin locks: %d", r.StatusCode)
	}
	do(adm, "POST", "/api/v1/accounts/unlock?dn="+url.QueryEscape(jdoe), "")
	r = do(adm, "GET", "/api/v1/tasks", "")
	var tasks []ds.Task
	_ = json.NewDecoder(r.Body).Decode(&tasks)
	if len(tasks) == 0 || !strings.EqualFold(tasks[0].By, asmith) {
		t.Errorf("task author: %+v", tasks)
	}

	// Sign out ends the session.
	do(op, "POST", "/api/v1/auth/logout", "")
	if r := do(op, "GET", "/api/v1/monitor", ""); r.StatusCode != 401 {
		t.Errorf("after logout: %d", r.StatusCode)
	}

	// Repeated failures block the user name for a while.
	for range 5 {
		login("bjones", "wrong")
	}
	if _, code := login("bjones", "Secret123"); code != 429 {
		t.Errorf("after failures: %d", code)
	}
}
