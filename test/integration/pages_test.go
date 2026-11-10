//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// Every attribute of every page must exist on a fresh server, so typos in the
// YAML definitions show up here (cn=config returns all its attributes).
func TestPagesMatchServer(t *testing.T) {
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	svc := pages.New(defs)
	c := newClient(t)
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		sch, _, _, err := schema.Fetch(conn)
		if err != nil {
			return err
		}
		list, err := svc.List(conn)
		if err != nil {
			return err
		}
		for _, s := range list {
			st, err := svc.Get(conn, s.ID)
			if err != nil {
				t.Errorf("page %s: %v", s.ID, err)
				continue
			}
			// Sparse pages (plug-ins): absent attributes must at least be
			// storable in the entry.
			var classes []string
			if st.Page.Sparse {
				res, err := conn.Search(ldap.NewSearchRequest(st.Page.DN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
					1, 0, false, "(objectClass=*)", []string{"objectClass"}, nil))
				if err != nil || len(res.Entries) != 1 {
					t.Errorf("page %s: entry %s: %v", s.ID, st.Page.DN, err)
					continue
				}
				classes = res.Entries[0].GetAttributeValues("objectClass")
			}
			_, may := sch.Allowed(classes)
			extensible := slices.ContainsFunc(classes, func(c string) bool { return strings.EqualFold(c, "extensibleObject") })
			for _, f := range st.Page.Fields {
				if _, ok := st.Values[f.Attr]; !ok {
					switch {
					case !st.Page.Sparse:
						t.Errorf("page %s: attribute %s not on the server", s.ID, f.Attr)
					case extensible:
					case !slices.ContainsFunc(may, func(a string) bool { return strings.EqualFold(a, f.Attr) }):
						t.Errorf("page %s: %s is not allowed in %s (%v)", s.ID, f.Attr, st.Page.DN, classes)
					}
				}
				if f.Type == pages.TypeEnum && len(f.Values) == 0 && len(st.Options[f.Attr]) == 0 {
					t.Errorf("page %s: no options for %s", s.ID, f.Attr)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPagesAPI(t *testing.T) {
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	restart := &ds.RestartRegistry{}
	h := api.NewHandler(api.Deps{LDAP: newClient(t), Pages: pages.New(defs), Restart: restart, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(api.CSRFHeader, "ds-ui")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	r := do("GET", "/api/v1/pages", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"id":"db-mdb"`) {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	r = do("GET", "/api/v1/pages/password-policy", "")
	var st pages.PageState
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &st) != nil {
		t.Fatalf("get: %d %s", r.Code, r.Body)
	}
	if !slices.Contains(st.Options["passwordStorageScheme"], "PBKDF2-SHA512") {
		t.Errorf("scheme options = %v", st.Options["passwordStorageScheme"])
	}
	if r := do("GET", "/api/v1/pages/nope", ""); r.Code != 404 {
		t.Errorf("unknown page: %d", r.Code)
	}

	// A dynamic change, then back.
	orig := st.Values["passwordMinLength"][0]
	r = do("PATCH", "/api/v1/pages/password-policy", `{"values":{"passwordMinLength":["12"],"passwordChange":["on"]}}`)
	var res pages.PageUpdate
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &res) != nil {
		t.Fatalf("patch: %d %s", r.Code, r.Body)
	}
	if !slices.Equal(res.Changed, []string{"passwordMinLength"}) || res.State.Values["passwordMinLength"][0] != "12" {
		t.Errorf("patch result: %+v", res)
	}
	defer do("PATCH", "/api/v1/pages/password-policy", `{"values":{"passwordMinLength":["`+orig+`"]}}`)

	// Validation happens before anything is written.
	if r := do("PATCH", "/api/v1/pages/password-policy", `{"values":{"passwordMinLength":["1"]}}`); r.Code != 422 {
		t.Errorf("out of range: %d %s", r.Code, r.Body)
	}
	if r := do("PATCH", "/api/v1/pages/password-policy", `{"values":{"passwordStorageScheme":["ROT13"]}}`); r.Code != 422 {
		t.Errorf("unknown scheme: %d %s", r.Code, r.Body)
	}
	if r := do("PATCH", "/api/v1/pages/password-policy", `{"values":{"nsslapd-port":["1"]}}`); r.Code != 422 {
		t.Errorf("attribute of another page: %d %s", r.Code, r.Body)
	}

	// A restart-only setting is marked for restart.
	r = do("GET", "/api/v1/pages/server-general", "")
	_ = json.Unmarshal(r.Body.Bytes(), &st)
	exact := st.Values["nsslapd-return-exact-case"][0]
	flip := map[string]string{"on": "off", "off": "on"}[exact]
	r = do("PATCH", "/api/v1/pages/server-general", `{"values":{"nsslapd-return-exact-case":["`+flip+`"]}}`)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"restart":["nsslapd-return-exact-case"]`) {
		t.Errorf("restart patch: %d %s", r.Code, r.Body)
	}
	do("PATCH", "/api/v1/pages/server-general", `{"values":{"nsslapd-return-exact-case":["`+exact+`"]}}`)
	if !slices.Contains(restart.Reasons(), "nsslapd-return-exact-case") {
		t.Errorf("restart reasons = %v", restart.Reasons())
	}
}

// A sparse page writes attributes that did not exist and removes them again.
func TestPluginPageRoundTrip(t *testing.T) {
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	svc := pages.New(defs)
	c := newClient(t)
	err = c.Do(context.Background(), func(conn *ldap.Conn) error {
		res, err := svc.Update(conn, "plugin-memberof", map[string][]string{
			"memberOfSkipNested": {"on"}, "memberOfEntryScope": {suffix},
		})
		if err != nil {
			return err
		}
		if !slices.Equal(res.Changed, []string{"memberOfEntryScope", "memberOfSkipNested"}) {
			t.Errorf("changed = %v", res.Changed)
		}
		res, err = svc.Update(conn, "plugin-memberof", map[string][]string{"memberOfSkipNested": {}, "memberOfEntryScope": {}})
		if err != nil {
			return err
		}
		if _, ok := res.State.Values["memberOfSkipNested"]; ok {
			t.Errorf("memberOfSkipNested not removed: %v", res.State.Values)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPluginsAPI(t *testing.T) {
	restart := &ds.RestartRegistry{}
	bus := &events.Bus{}
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Restart: restart, Tasks: ds.NewTasks(c, bus, nil), Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(api.CSRFHeader, "ds-ui")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	r := do("GET", "/api/v1/plugins", "")
	var list api.PluginList
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &list) != nil {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	byName := map[string]ds.Plugin{}
	for _, p := range list.Plugins {
		byName[p.Name] = p
	}
	if len(list.Plugins) < 50 || !byName["ldbm database"].Protected || byName["USN"].Protected {
		t.Fatalf("plugins: %d, ldbm %+v", len(list.Plugins), byName["ldbm database"])
	}
	// Space in the name: the path is URL-encoded.
	if r := do("GET", "/api/v1/plugins/MemberOf%20Plugin", ""); r.Code != 200 {
		t.Errorf("get: %d %s", r.Code, r.Body)
	}
	if r := do("GET", "/api/v1/plugins/nope", ""); r.Code != 404 {
		t.Errorf("unknown: %d", r.Code)
	}
	if r := do("PATCH", "/api/v1/plugins/ldbm%20database", `{"enabled":false}`); r.Code != 409 {
		t.Errorf("disable protected: %d %s", r.Code, r.Body)
	}
	usn := byName["USN"].Enabled
	r = do("PATCH", "/api/v1/plugins/USN", fmt.Sprintf(`{"enabled":%v}`, !usn))
	var ch api.PluginChange
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &ch) != nil || ch.Plugin.Enabled == usn {
		t.Fatalf("toggle: %d %s", r.Code, r.Body)
	}
	do("PATCH", "/api/v1/plugins/USN", fmt.Sprintf(`{"enabled":%v}`, usn))
	if ch.Restart != !list.Dynamic || ch.Restart && !slices.Contains(restart.Reasons(), "USN") {
		t.Errorf("restart = %v, dynamic = %v, reasons = %v", ch.Restart, list.Dynamic, restart.Reasons())
	}
	if r := do("GET", "/api/v1/tasks", ""); !strings.Contains(r.Body.String(), `"type":"enable-plugin"`) {
		t.Errorf("history lacks plug-in changes: %s", r.Body)
	}
}
