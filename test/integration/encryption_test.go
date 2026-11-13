//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/certsource"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// TestAttributeEncryption encrypts an attribute of a backend, checks that it
// is stored encrypted, that a decrypted export has it in clear text, and that
// while it is configured the server certificate cannot change its key.
func TestAttributeEncryption(t *testing.T) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(context.Background()) })
	n := startNode(t, ctx, nw, "cryptds")
	if n == nil {
		t.FailNow()
	}
	exec := func(cmd string) string {
		t.Helper()
		_, out, err := n.ct.Exec(ctx, []string{"sh", "-c", cmd}, tcexec.Multiplexed())
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(out)
		return string(b)
	}
	store := certs.New("/data/config", containerRunner{n.ct})
	bus := &events.Bus{}
	tasks := ds.NewTasks(n.c, bus, nil)
	h := api.NewHandler(api.Deps{LDAP: n.c, Schemas: schema.NewCache(0, nil), Certs: store, Restart: &ds.RestartRegistry{},
		Tasks: tasks, Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}

	// Turn encryption on for telephoneNumber.
	if code, body := do("POST", "/api/v1/backends/userroot/encrypted-attrs", `{"attr":"telephonenumber"}`); code != 204 {
		t.Fatalf("encrypt: %d %s", code, body)
	}
	if code, _ := do("POST", "/api/v1/backends/userroot/encrypted-attrs", `{"attr":"telephoneNumber"}`); code != 409 {
		t.Errorf("encrypt twice: %d", code)
	}
	if code, _ := do("POST", "/api/v1/backends/userroot/encrypted-attrs", `{"attr":"noSuchAttribute"}`); code != 422 {
		t.Errorf("unknown attribute: %d", code)
	}
	code, body := do("GET", "/api/v1/backends/userroot/encrypted-attrs", "")
	var list []ds.EncryptedAttr
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list) != 1 || list[0].Attr != "telephoneNumber" || list[0].Algorithm != "AES" {
		t.Fatalf("list: %d %s", code, body)
	}

	// New values are encrypted on disk and read back in clear text.
	dn := "uid=crypt,dc=example,dc=com"
	err = n.c.Do(ctx, func(c *ldap.Conn) error {
		req := ldap.NewAddRequest(dn, nil)
		req.Attribute("objectClass", []string{"inetOrgPerson"})
		for a, v := range map[string]string{"uid": "crypt", "cn": "c", "sn": "c", "telephoneNumber": "555-0199"} {
			req.Attribute(a, []string{v})
		}
		return c.Add(req)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !n.exists(t, dn) {
		t.Fatal("entry missing")
	}
	if got := exec("dbscan -D mdb -f /data/db/userroot/id2entry.db | grep -c 555-0199"); strings.TrimSpace(lastLine(got)) != "0" {
		t.Errorf("value stored in clear text: %q", got)
	}

	// Removing the setting would leave ciphertext: refused while values exist.
	if code, body := do("DELETE", "/api/v1/backends/userroot/encrypted-attrs/telephoneNumber", ""); code != 409 {
		t.Errorf("remove with values: %d %s", code, body)
	}

	// A decrypted export has the value in clear text.
	code, body = do("POST", "/api/v1/backends/userroot/export", `{"file":"itest_decrypted.ldif","decrypt":true}`)
	var task ds.Task
	if code != 202 || json.Unmarshal([]byte(body), &task) != nil {
		t.Fatalf("export: %d %s", code, body)
	}
	wctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if done, err := tasks.Wait(wctx, task.ID); err != nil || done.Status != ds.TaskSucceeded {
		t.Fatalf("export task: %v %+v", err, done)
	}
	if got := exec("grep -c 555-0199 /data/ldif/itest_decrypted.ldif"); strings.TrimSpace(lastLine(got)) != "1" {
		t.Errorf("decrypted export: %q", got)
	}

	// A server certificate with another key is refused, manually and from files.
	pc, pk := selfSignedPair(t, "newkey.itest.example")
	if code, body := do("POST", "/api/v1/security/certs/pem-pair", js(map[string]string{"nickname": "itest newkey", "cert": pc, "key": pk})); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if code, body := do("PUT", "/api/v1/security/server-cert", `{"nickname":"itest newkey"}`); code != 409 || !strings.Contains(body, "same key") {
		t.Errorf("switch to another key: %d %s", code, body)
	}
	dir := t.TempDir()
	cf, kf := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	_ = os.WriteFile(cf, []byte(pc), 0o600)
	_ = os.WriteFile(kf, []byte(pk), 0o600)
	w := certsource.New(certsource.Config{CertFile: cf, KeyFile: kf}, store, ds.ServerCert{LDAP: n.c}, certsource.Hooks{}, nil)
	if st, err := w.Check(ctx); err == nil || st.InUse {
		t.Errorf("files with another key: %v %+v", err, st)
	}

	// Forced removal (after a decrypted export) allows the key change again.
	if code, body := do("DELETE", "/api/v1/backends/userroot/encrypted-attrs/telephoneNumber?force=true", ""); code != 204 {
		t.Fatalf("forced removal: %d %s", code, body)
	}
	if code, body := do("PUT", "/api/v1/security/server-cert", `{"nickname":"itest newkey"}`); code != 204 {
		t.Errorf("switch after removal: %d %s", code, body)
	}
}

// TestSubSuffix creates a suffix below another one and checks that its
// entries show up under the parent and that the parent cannot be deleted.
func TestSubSuffix(t *testing.T) {
	c := newClient(t)
	bus := &events.Bus{}
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: ds.NewTasks(c, bus, nil), Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}
	sub := "ou=itest sub," + suffix
	if code, _ := do("POST", "/api/v1/backends", js(map[string]any{"name": "itestsub", "suffix": "ou=x,dc=other", "parentSuffix": suffix, "createRoot": false})); code != 400 {
		t.Errorf("not below the parent: %d", code)
	}
	if code, _ := do("POST", "/api/v1/backends", js(map[string]any{"name": "itestsub", "suffix": sub, "parentSuffix": "dc=nowhere", "createRoot": false})); code != 404 {
		t.Errorf("unknown parent: %d", code)
	}
	code, body := do("POST", "/api/v1/backends", js(map[string]any{"name": "itestsub", "suffix": sub, "parentSuffix": strings.ToUpper(suffix), "createRoot": true}))
	if code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	t.Cleanup(func() { do("DELETE", "/api/v1/backends/itestsub", "") })
	var b ds.Backend
	if json.Unmarshal([]byte(body), &b) != nil || !strings.EqualFold(b.ParentSuffix, suffix) || !b.RootExists {
		t.Errorf("backend: %s", body)
	}

	// The sub-suffix root is a child of the parent suffix's root.
	err := c.Do(context.Background(), func(conn *ldap.Conn) error {
		res, err := conn.Search(ldap.NewSearchRequest(suffix, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
			"(ou=itest sub)", []string{"1.1"}, nil))
		if err != nil {
			return err
		}
		if len(res.Entries) != 1 {
			t.Errorf("one-level search under the parent found %d entries", len(res.Entries))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if code, body := do("DELETE", "/api/v1/backends/userroot", ""); code != 409 || !strings.Contains(body, "sub-suffix") {
		t.Errorf("delete the parent: %d %s", code, body)
	}
	if code, body := do("DELETE", "/api/v1/backends/"+url.PathEscape("itestsub"), ""); code != 204 {
		t.Errorf("delete the sub-suffix: %d %s", code, body)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

func js(v any) string { b, _ := json.Marshal(v); return string(b) }
