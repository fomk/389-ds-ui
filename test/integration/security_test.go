//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func apiHandler(t *testing.T) (func(method, path, body string) *httptest.ResponseRecorder, *ds.Tasks) {
	t.Helper()
	c := newClient(t)
	bus := &events.Bus{}
	tasks := ds.NewTasks(c, bus, nil)
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: tasks, Events: bus, Lifecycle: providers.NoLifecycle{}})
	return func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(api.CSRFHeader, "ds-ui")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}, tasks
}

func TestRootPassword(t *testing.T) {
	do, _ := apiHandler(t)
	if r := do("POST", "/api/v1/root-password", `{"password":"short"}`); r.Code != 422 {
		t.Errorf("short password: %d %s", r.Code, r.Body)
	}
	if r := do("POST", "/api/v1/root-password", `{"password":"{SSHA}abcdefgh"}`); r.Code != 422 {
		t.Errorf("pre-hashed password: %d %s", r.Code, r.Body)
	}
	const newPw = "Changed-Root-Pw-1"
	r := do("POST", "/api/v1/root-password", `{"password":"`+newPw+`"}`)
	var res api.RootPasswordResult
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &res) != nil {
		t.Fatalf("change: %d %s", r.Code, r.Body)
	}
	defer func() {
		// Restore for the other tests, binding with the new password.
		c, err := newClientWith(dmDN, newPw)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.Close() }()
		if err := c.Do(context.Background(), func(conn *ldap.Conn) error { return ds.SetRootPassword(conn, dmPassword) }); err != nil {
			t.Fatalf("restore: %v", err)
		}
	}()
	// The test client binds as Directory Manager with a password.
	if !strings.EqualFold(res.RootDN, dmDN) || !res.CredentialsUpdated {
		t.Errorf("result = %+v", res)
	}
	c, err := newClientWith(dmDN, newPw)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Do(context.Background(), func(conn *ldap.Conn) error { return searchBase(conn, suffix) }); err != nil {
		t.Errorf("bind with the new password: %v", err)
	}
}

func TestSASLMappingsAPI(t *testing.T) {
	do, _ := apiHandler(t)
	body := `{"name":"itest map","regex":"^(.*)@itest$","baseDN":"` + suffix + `","filter":"(uid=\\1)","priority":10}`
	if r := do("POST", "/api/v1/sasl-mappings", body); r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	defer do("DELETE", "/api/v1/sasl-mappings/itest%20map", "")
	if r := do("POST", "/api/v1/sasl-mappings", body); r.Code != 409 {
		t.Errorf("duplicate: %d", r.Code)
	}
	if r := do("POST", "/api/v1/sasl-mappings", `{"name":"x","regex":"","baseDN":"a","filter":"b"}`); r.Code != 422 {
		t.Errorf("empty regex: %d %s", r.Code, r.Body)
	}
	r := do("GET", "/api/v1/sasl-mappings", "")
	var list []ds.SASLMapping
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &list) != nil || len(list) < 3 || list[0].Name != "itest map" {
		t.Fatalf("list (priority 10 first): %d %s", r.Code, r.Body)
	}
	if list[0].Filter != `(uid=\1)` {
		t.Errorf("filter = %q", list[0].Filter)
	}
	if r := do("PUT", "/api/v1/sasl-mappings/itest%20map", `{"name":"ignored","regex":"^(.*)$","baseDN":"`+suffix+`","filter":"(cn=\\1)"}`); r.Code != 204 {
		t.Errorf("update: %d %s", r.Code, r.Body)
	}
	r = do("GET", "/api/v1/sasl-mappings", "")
	if !strings.Contains(r.Body.String(), `"filter":"(cn=\\1)"`) || strings.Contains(r.Body.String(), `"priority":10`) {
		t.Errorf("after update: %s", r.Body)
	}
	if r := do("DELETE", "/api/v1/sasl-mappings/itest%20map", ""); r.Code != 204 {
		t.Errorf("delete: %d %s", r.Code, r.Body)
	}
	if r := do("DELETE", "/api/v1/sasl-mappings/itest%20map", ""); r.Code != 404 {
		t.Errorf("delete again: %d", r.Code)
	}
}

func TestVLVLifecycle(t *testing.T) {
	do, tasks := apiHandler(t)
	body := `{"name":"itest people","base":"` + suffix + `","scope":"sub","filter":"(objectClass=person)","sorts":["sn givenName","-cn"]}`
	if r := do("POST", "/api/v1/backends/userroot/vlv", `{"name":"bad","base":"x","scope":"sub","filter":"(","sorts":["sn"]}`); r.Code != 422 {
		t.Errorf("bad VLV: %d %s", r.Code, r.Body)
	}
	if r := do("POST", "/api/v1/backends/userroot/vlv", body); r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	defer do("DELETE", "/api/v1/backends/userroot/vlv/itest%20people", "")
	r := do("GET", "/api/v1/backends/userroot/vlv", "")
	var list []ds.VLVIndex
	_ = json.Unmarshal(r.Body.Bytes(), &list)
	var v *ds.VLVIndex
	for i := range list {
		if list[i].Name == "itest people" {
			v = &list[i]
		}
	}
	if v == nil || v.Scope != "sub" || len(v.Sorts) != 2 || v.Sorts[1] != "-cn" || len(v.IndexNames) != 2 {
		t.Fatalf("listed: %+v", v)
	}
	r = do("POST", "/api/v1/backends/userroot/reindex", `{"vlv":["itest people"]}`)
	var task ds.Task
	if r.Code != 202 || json.Unmarshal(r.Body.Bytes(), &task) != nil {
		t.Fatalf("build: %d %s", r.Code, r.Body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	done, err := tasks.Wait(ctx, task.ID)
	if err != nil || done.Status != ds.TaskSucceeded {
		t.Errorf("build task: %+v %v", done, err)
	}
	if r := do("DELETE", "/api/v1/backends/userroot/vlv/itest%20people", ""); r.Code != 204 {
		t.Errorf("delete: %d %s", r.Code, r.Body)
	}
	if r := do("DELETE", "/api/v1/backends/userroot/vlv/itest%20people", ""); r.Code != 404 {
		t.Errorf("delete again: %d", r.Code)
	}
}

func TestDefaultIndexes(t *testing.T) {
	do, _ := apiHandler(t)
	r := do("GET", "/api/v1/default-indexes", "")
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"attribute":"objectclass"`) {
		t.Fatalf("list: %d %s", r.Code, r.Body)
	}
	if r := do("POST", "/api/v1/default-indexes", `{"attribute":"employeeNumber","types":["eq"],"matchingRules":[],"system":false}`); r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	defer do("DELETE", "/api/v1/default-indexes/employeeNumber", "")
	if r := do("PUT", "/api/v1/default-indexes/employeeNumber", `{"types":["eq","pres"]}`); r.Code != 204 {
		t.Errorf("update: %d %s", r.Code, r.Body)
	}
	// A backend created now gets the default index.
	if r := do("POST", "/api/v1/backends", `{"name":"defix","suffix":"o=defix","createRoot":false}`); r.Code != 201 {
		t.Fatalf("create backend: %d %s", r.Code, r.Body)
	}
	defer do("DELETE", "/api/v1/backends/defix", "")
	if r := do("GET", "/api/v1/backends/defix/indexes", ""); !strings.Contains(r.Body.String(), `"attribute":"employeeNumber"`) {
		t.Errorf("new backend lacks the default index: %s", r.Body)
	}
	if r := do("DELETE", "/api/v1/default-indexes/objectclass", ""); r.Code != 409 {
		t.Errorf("delete system default index: %d", r.Code)
	}
	if r := do("DELETE", "/api/v1/default-indexes/employeeNumber", ""); r.Code != 204 {
		t.Errorf("delete: %d %s", r.Code, r.Body)
	}
}
