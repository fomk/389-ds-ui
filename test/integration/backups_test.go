//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// Backup, restore, export and import through the HTTP API. The test DS's
// files are not visible here (Files is nil, like remote mode), which also
// checks that tasks work without file access.
func TestBackupRestoreExportImport(t *testing.T) {
	c := newClient(t)
	bus := &events.Bus{}
	tasks := ds.NewTasks(c, bus, nil)
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: tasks, Events: bus, Lifecycle: providers.NoLifecycle{}})
	ctx := context.Background()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(api.CSRFHeader, "ds-ui")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	runTask := func(method, path, body string) ds.Task {
		t.Helper()
		r := do(method, path, body)
		if r.Code != 202 {
			t.Fatalf("%s %s: %d %s", method, path, r.Code, r.Body)
		}
		var task ds.Task
		_ = json.Unmarshal(r.Body.Bytes(), &task)
		wctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		done, err := tasks.Wait(wctx, task.ID)
		if err != nil {
			t.Fatalf("%s: %v", task.ID, err)
		}
		if done.Status != ds.TaskSucceeded {
			t.Fatalf("%s %s: status %s, log %v", method, path, done.Status, done.Log)
		}
		return *done
	}

	runTask("POST", "/api/v1/backups", `{"name":"itest_backup"}`)

	// Change something after the backup; restore must undo it.
	svc, _ := newEntries(t)
	marker := "uid=jdoe,ou=People," + suffix
	if _, err := svc.Modify(ctx, marker, "", []entries.Change{{Op: "replace", Attr: "description", Values: []string{"after backup"}}}); err != nil {
		t.Fatal(err)
	}
	runTask("POST", "/api/v1/backups/itest_backup/restore", "")
	e, err := svc.Get(ctx, marker)
	if err != nil {
		t.Fatal(err)
	}
	if v := values(e, "description"); len(v) > 0 && v[0] == "after backup" {
		t.Error("restore did not roll back the change")
	}

	exp := runTask("POST", "/api/v1/backends/userroot/export", `{"file":"itest_export.ldif"}`)
	if !strings.Contains(exp.Target, "itest_export.ldif") {
		t.Errorf("export target = %q", exp.Target)
	}
	runTask("POST", "/api/v1/backends/userroot/import", `{"file":"itest_export.ldif"}`)
	if _, err := svc.Get(ctx, marker); err != nil {
		t.Errorf("entry missing after re-import: %v", err)
	}

	// Without file access the file listings are not available.
	if r := do("GET", "/api/v1/backups", ""); r.Code != 501 {
		t.Errorf("backups list without files: %d", r.Code)
	}
	if r := do("GET", "/api/v1/logs", ""); r.Code != 501 {
		t.Errorf("logs without files: %d", r.Code)
	}
	if r := do("POST", "/api/v1/backups", `{"name":"../evil"}`); r.Code != 400 {
		t.Errorf("invalid backup name: %d", r.Code)
	}
}
