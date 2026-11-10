//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestBackendLifecycle(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	run := func(fn func(*ldap.Conn) error) error { return c.Do(ctx, fn) }
	const name, sfx = "itestbe", "dc=itest,dc=org"

	if err := run(func(l *ldap.Conn) error {
		return ds.CreateBackend(l, ds.NewBackend{Name: name, Suffix: sfx, CreateRoot: true})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run(func(l *ldap.Conn) error { return ds.DeleteBackend(l, name) }) })

	var b *ds.Backend
	_ = run(func(l *ldap.Conn) (err error) { b, err = ds.GetBackend(l, name); return })
	if b == nil || b.Suffix != sfx || !b.RootExists || b.State != "backend" || b.ReadOnly {
		t.Fatalf("backend = %+v", b)
	}

	on := true
	if err := run(func(l *ldap.Conn) error { return ds.UpdateBackend(l, name, ds.BackendPatch{ReadOnly: &on}) }); err != nil {
		t.Fatal(err)
	}
	_ = run(func(l *ldap.Conn) (err error) { b, err = ds.GetBackend(l, name); return })
	if !b.ReadOnly {
		t.Error("readOnly not applied")
	}
	off := false
	_ = run(func(l *ldap.Conn) error { return ds.UpdateBackend(l, name, ds.BackendPatch{ReadOnly: &off}) })

	if err := run(func(l *ldap.Conn) error {
		return ds.CreateBackend(l, ds.NewBackend{Name: "bad name!", Suffix: sfx})
	}); !errors.Is(err, ds.ErrInvalidName) {
		t.Errorf("invalid name: %v", err)
	}

	if err := run(func(l *ldap.Conn) error { return ds.DeleteBackend(l, name) }); err != nil {
		t.Fatalf("delete backend: %v", err)
	}
	if err := run(func(l *ldap.Conn) (err error) { _, err = ds.GetBackend(l, name); return }); !errors.Is(err, ds.ErrNotFound) {
		t.Errorf("backend still listed after delete: %v", err)
	}
	var roots *ldap.SearchResult
	_ = run(func(l *ldap.Conn) (err error) {
		roots, err = l.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, 0, 0, 0, false, "(objectClass=*)", []string{"namingContexts"}, nil))
		return
	})
	if slices.Contains(roots.Entries[0].GetAttributeValues("namingContexts"), sfx) {
		t.Error("suffix still in namingContexts")
	}
}

func TestIndexesAndReindexTask(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	run := func(fn func(*ldap.Conn) error) error { return c.Do(ctx, fn) }

	ix := ds.Index{Attribute: "departmentNumber", Types: []string{"eq", "sub"}}
	if err := run(func(l *ldap.Conn) error { return ds.CreateIndex(l, "userroot", ix) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run(func(l *ldap.Conn) error { return ds.DeleteIndex(l, "userroot", "departmentNumber") }) })

	var list []ds.Index
	_ = run(func(l *ldap.Conn) (err error) { list, err = ds.ListIndexes(l, "userroot"); return })
	i := slices.IndexFunc(list, func(x ds.Index) bool { return strings.EqualFold(x.Attribute, "departmentNumber") })
	if i < 0 || list[i].System || len(list[i].Types) != 2 {
		t.Fatalf("index not listed: %+v", list)
	}

	if err := run(func(l *ldap.Conn) error {
		return ds.UpdateIndex(l, "userroot", ds.Index{Attribute: "departmentNumber", Types: []string{"eq"}, MatchingRules: []string{}})
	}); err != nil {
		t.Fatal(err)
	}

	bus := &events.Bus{}
	sub, unsub := bus.Subscribe()
	defer unsub()
	tasks := ds.NewTasks(c, bus, nil)
	var attrs map[string][]string
	_ = run(func(l *ldap.Conn) (err error) {
		attrs, err = ds.ReindexAttrs(l, "userroot", []string{"departmentNumber"}, nil)
		return
	})
	task, err := tasks.Start(ctx, ds.TaskIndex, "userroot", attrs)
	if err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	done, err := tasks.Wait(wctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != ds.TaskSucceeded || done.ExitCode == nil || *done.ExitCode != 0 || len(done.Log) == 0 {
		t.Errorf("task = %+v", done)
	}
	if e := <-sub; e.Type != "task" {
		t.Errorf("task events must be published, got %+v", e)
	}

	if err := run(func(l *ldap.Conn) error { return ds.DeleteIndex(l, "userroot", "parentid") }); !errors.Is(err, ds.ErrSystemIndex) {
		t.Errorf("system index deletion: %v", err)
	}
	if err := run(func(l *ldap.Conn) error {
		return ds.CreateIndex(l, "userroot", ds.Index{Attribute: "cn", Types: []string{"fulltext"}})
	}); !errors.Is(err, ds.ErrInvalidName) {
		t.Errorf("bad index type: %v", err)
	}
}

// Exercises the routes through HTTP: path parameters must reach the handlers.
func TestDatabasesAPI(t *testing.T) {
	c := newClient(t)
	bus := &events.Bus{}
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: ds.NewTasks(c, bus, nil), Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(api.CSRFHeader, "ds-ui")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if r := do("POST", "/api/v1/backends", `{"name":"apibe","suffix":"o=apitest","createRoot":true}`); r.Code != 201 {
		t.Fatalf("create: %d %s", r.Code, r.Body)
	}
	defer do("DELETE", "/api/v1/backends/apibe", "")
	if r := do("PATCH", "/api/v1/backends/apibe", `{"readOnly":true}`); r.Code != 200 || !strings.Contains(r.Body.String(), `"readOnly":true`) {
		t.Errorf("patch: %d %s", r.Code, r.Body)
	}
	if r := do("POST", "/api/v1/backends/apibe/indexes", `{"attribute":"title","types":["eq"],"matchingRules":[],"system":false}`); r.Code != 201 {
		t.Errorf("create index: %d %s", r.Code, r.Body)
	}
	if r := do("PUT", "/api/v1/backends/apibe/indexes/title", `{"types":["eq","pres"]}`); r.Code != 204 {
		t.Errorf("update index: %d %s", r.Code, r.Body)
	}
	if r := do("POST", "/api/v1/backends/apibe/reindex", `{"attributes":["title"]}`); r.Code != 202 || !strings.Contains(r.Body.String(), `"target":"apibe"`) {
		t.Errorf("reindex: %d %s", r.Code, r.Body)
	}
	if r := do("DELETE", "/api/v1/backends/apibe/indexes/title", ""); r.Code != 204 {
		t.Errorf("delete index: %d %s", r.Code, r.Body)
	}
	if r := do("DELETE", "/api/v1/backends/apibe/indexes/parentid", ""); r.Code != 409 {
		t.Errorf("delete system index: %d", r.Code)
	}
	if r := do("DELETE", "/api/v1/backends/apibe", ""); r.Code != 204 {
		t.Errorf("delete backend: %d %s", r.Code, r.Body)
	}
	if r := do("GET", "/api/v1/backends/apibe/indexes", ""); r.Code != 404 {
		t.Errorf("indexes of a deleted backend: %d", r.Code)
	}
	// Immediate operations are recorded in the task history too.
	if r := do("GET", "/api/v1/tasks", ""); !strings.Contains(r.Body.String(), `"type":"delete-backend"`) ||
		!strings.Contains(r.Body.String(), `"type":"create-backend"`) {
		t.Errorf("history lacks create/delete-backend: %s", r.Body)
	}
}
