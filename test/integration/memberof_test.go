//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// The fixture groups exist before the MemberOf plug-in is enabled, so only
// the fixup task fills memberOf. Dynamic plug-ins avoid a server restart.
func TestMemberOfFixup(t *testing.T) {
	c := newClient(t)
	bus := &events.Bus{}
	tasks := ds.NewTasks(c, bus, nil)
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: tasks, Events: bus, Restart: &ds.RestartRegistry{}, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}
	ctx := context.Background()
	user := "uid=jdoe,ou=People," + suffix
	memberOf := func() []string {
		t.Helper()
		var out []string
		err := c.Do(ctx, func(conn *ldap.Conn) error {
			r, err := conn.Search(ldap.NewSearchRequest(user, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
				"(objectClass=*)", []string{"memberOf"}, nil))
			if err != nil {
				return err
			}
			for _, v := range r.Entries[0].GetAttributeValues("memberOf") {
				out = append(out, ldapx.NormDN(v))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(out)
		return out
	}
	setConfig := func(attr, v string) {
		t.Helper()
		err := c.Do(ctx, func(conn *ldap.Conn) error {
			req := ldap.NewModifyRequest("cn=config", nil)
			req.Replace(attr, []string{v})
			return conn.Modify(req)
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if got := memberOf(); len(got) != 0 {
		t.Fatalf("memberOf before the plug-in: %v", got)
	}

	setConfig("nsslapd-dynamic-plugins", "on")
	defer setConfig("nsslapd-dynamic-plugins", "off")
	code, body := do("PATCH", "/api/v1/plugins/MemberOf%20Plugin", `{"enabled":true}`)
	var ch api.PluginChange
	if code != 200 || json.Unmarshal([]byte(body), &ch) != nil || !ch.Plugin.Enabled || ch.Restart {
		t.Fatalf("enable MemberOf: %d %s", code, body)
	}
	defer do("PATCH", "/api/v1/plugins/MemberOf%20Plugin", `{"enabled":false}`)

	code, body = do("POST", "/api/v1/plugin-tasks/memberof-fixup", `{"params":{"basedn":"`+suffix+`"}}`)
	var task ds.Task
	if code != 202 || json.Unmarshal([]byte(body), &task) != nil {
		t.Fatalf("start fixup: %d %s", code, body)
	}
	if !strings.HasPrefix(task.DN, "cn=memberof task_") && !strings.Contains(strings.ToLower(task.DN), ",cn=memberof task,cn=tasks,cn=config") {
		t.Errorf("task DN = %s", task.DN)
	}
	wctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	done, err := tasks.Wait(wctx, task.ID)
	if err != nil || done.Status != ds.TaskSucceeded {
		t.Fatalf("fixup task: %+v %v", done, err)
	}

	groups := "ou=Groups," + suffix
	want := []string{
		ldapx.NormDN("cn=admins," + groups),
		ldapx.NormDN("cn=all-staff," + groups), // through cn=engineering (nested)
		ldapx.NormDN("cn=engineering," + groups),
	}
	slices.Sort(want)
	if got := memberOf(); !slices.Equal(got, want) {
		t.Errorf("memberOf after fixup = %v, want %v (task log: %v)", got, want, done.Log)
	}

	// With the plug-in on, a membership change updates memberOf at once,
	// and removing it takes the value away again.
	sales := "cn=sales," + groups
	modMember := func(add bool) {
		t.Helper()
		err := c.Do(ctx, func(conn *ldap.Conn) error {
			req := ldap.NewModifyRequest(sales, nil)
			if add {
				req.Add("member", []string{user})
			} else {
				req.Delete("member", []string{user})
			}
			return conn.Modify(req)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	modMember(true)
	if got := memberOf(); !slices.Contains(got, ldapx.NormDN(sales)) {
		t.Errorf("memberOf after adding to sales = %v", got)
	}
	modMember(false)
	if got := memberOf(); slices.Contains(got, ldapx.NormDN(sales)) || !slices.Equal(got, want) {
		t.Errorf("memberOf after removing from sales = %v", got)
	}
}
