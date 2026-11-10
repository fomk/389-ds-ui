//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
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
			for _, f := range st.Page.Fields {
				if _, ok := st.Values[f.Attr]; !ok {
					t.Errorf("page %s: attribute %s not on the server", s.ID, f.Attr)
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
