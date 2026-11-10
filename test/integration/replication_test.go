//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// node is one DS container with a ds-ui API handler talking to it.
type node struct {
	name string
	url  string
	c    *ldapx.Client
	h    http.Handler
	ct   testcontainers.Container
}

func (n *node) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	rec := serve(n.h, httptestRequest(method, path, body))
	return rec.Code, rec.Body.String()
}

func startNode(t *testing.T, ctx context.Context, nw *testcontainers.DockerNetwork, alias string) *node {
	t.Helper()
	image := os.Getenv("DS_IMAGE")
	if image == "" {
		image = "389-ds-ui/ds:dev"
	}
	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"3389/tcp"},
			Env:          map[string]string{"DS_DM_PASSWORD": dmPassword, "DS_SUFFIX": suffix},
			WaitingFor: wait.ForLog("ds-entrypoint: suffix " + suffix + " ready").
				WithStartupTimeout(3 * time.Minute).WithPollInterval(time.Second),
		},
		Started: true,
	}
	// Runs in its own goroutine: report with Errorf, never FailNow.
	if err := network.WithNetwork([]string{alias}, nw)(&req); err != nil {
		t.Error(err)
		return nil
	}
	ct, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Errorf("start %s: %v", alias, err)
		return nil
	}
	t.Cleanup(func() { _ = ct.Terminate(context.Background()) })
	ep, err := ct.PortEndpoint(ctx, "3389/tcp", "ldap")
	if err != nil {
		t.Error(err)
		return nil
	}
	c, err := ldapx.New(ldapx.Options{URL: ep, BindDN: dmDN, BindPassword: dmPassword, Timeout: 10 * time.Second})
	if err != nil {
		t.Error(err)
		return nil
	}
	t.Cleanup(func() { _ = c.Close() })
	bus := &events.Bus{}
	h := api.NewHandler(api.Deps{LDAP: c, Tasks: ds.NewTasks(c, bus, nil), Events: bus, Lifecycle: providers.NoLifecycle{}})
	return &node{name: alias, url: ep, c: c, h: h, ct: ct}
}

func (n *node) exists(t *testing.T, dn string) bool {
	t.Helper()
	found := false
	err := n.c.Do(context.Background(), func(conn *ldap.Conn) error {
		r, err := conn.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"1.1"}, nil))
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return nil
		}
		found = err == nil && len(r.Entries) == 1
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func (n *node) add(t *testing.T, dn string, attrs map[string][]string) {
	t.Helper()
	err := n.c.Do(context.Background(), func(conn *ldap.Conn) error {
		req := ldap.NewAddRequest(dn, nil)
		for k, v := range attrs {
			req.Attribute(k, v)
		}
		return conn.Add(req)
	})
	if err != nil {
		t.Fatalf("add %s on %s: %v", dn, n.name, err)
	}
}

func waitFor(t *testing.T, what string, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", what)
}

// TestTwoSupplierReplication sets up multi-supplier replication between two
// servers only through the ds-ui API.
func TestTwoSupplierReplication(t *testing.T) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(context.Background()) })

	var ds1, ds2 *node
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ds1 = startNode(t, ctx, nw, "ds1") }()
	go func() { defer wg.Done(); ds2 = startNode(t, ctx, nw, "ds2") }()
	wg.Wait()
	if ds1 == nil || ds2 == nil {
		t.FailNow()
	}
	q := "?suffix=" + url.QueryEscape(suffix)
	const replPw = "Repl-Manager-1"
	mgr := ds.DefaultReplManager

	// Fixtures only on ds1; ds2 gets them by initialization.
	if err := loadFixtures(ctx, ds1.ct); err != nil {
		t.Fatal(err)
	}
	jdoe := "uid=jdoe,ou=People," + suffix

	for i, n := range []*node{ds1, ds2} {
		if code, body := n.do(t, "POST", "/api/v1/replication/manager", `{"password":"`+replPw+`"}`); code != 200 {
			t.Fatalf("%s manager: %d %s", n.name, code, body)
		}
		rid := []string{"1", "2"}[i]
		if code, body := n.do(t, "POST", "/api/v1/replication"+q, `{"role":"supplier","replicaId":`+rid+`,"bindDNs":["`+mgr+`"]}`); code != 201 {
			t.Fatalf("%s enable: %d %s", n.name, code, body)
		}
	}
	if code, _ := ds1.do(t, "POST", "/api/v1/replication"+q, `{"role":"supplier","replicaId":1}`); code != 409 {
		t.Errorf("enable twice: %d", code)
	}
	code, body := ds1.do(t, "GET", "/api/v1/replication"+q, "")
	var rep ds.Replica
	if code != 200 || json.Unmarshal([]byte(body), &rep) != nil || rep.Role != ds.RoleSupplier || rep.ReplicaID != 1 || !slices.Contains(rep.BindDNs, mgr) {
		t.Fatalf("replica: %d %s", code, body)
	}

	// Changelog of a supplier.
	if code, body := ds1.do(t, "GET", "/api/v1/replication/changelog"+q, ""); code != 200 || !strings.Contains(body, `"exists":true`) {
		t.Errorf("changelog: %d %s", code, body)
	}
	if code, body := ds1.do(t, "PATCH", "/api/v1/replication/changelog"+q, `{"maxAge":"7d"}`); code != 200 || !strings.Contains(body, `"maxAge":"7d"`) {
		t.Errorf("changelog max age: %d %s", code, body)
	}

	agreementTo := func(from, to *node) {
		t.Helper()
		body := `{"name":"to-` + to.name + `","host":"` + to.name + `","port":3389,"transport":"LDAP","bindMethod":"SIMPLE","bindDN":"` + mgr + `","credentials":"` + replPw + `"}`
		if code, out := from.do(t, "POST", "/api/v1/replication/agreements"+q, body); code != 201 {
			t.Fatalf("agreement %s->%s: %d %s", from.name, to.name, code, out)
		}
	}
	agreementTo(ds1, ds2)

	// Initialize ds2 from ds1.
	if code, body := ds1.do(t, "POST", "/api/v1/replication/agreements/to-ds2/init"+q, ""); code != 200 {
		t.Fatalf("init: %d %s", code, body)
	}
	waitFor(t, "initialization of ds2", 90*time.Second, func() bool {
		_, body := ds1.do(t, "GET", "/api/v1/replication/agreements"+q, "")
		var list []ds.Agreement
		_ = json.Unmarshal([]byte(body), &list)
		if len(list) == 1 && list[0].Status.InitState == "error" {
			t.Fatalf("init failed: %s", list[0].Status.LastInit)
		}
		return len(list) == 1 && list[0].Status.InitState == "ok" && !list[0].Status.InProgress
	})
	waitFor(t, "jdoe on ds2", 30*time.Second, func() bool { return ds2.exists(t, jdoe) })

	agreementTo(ds2, ds1)

	// Changes flow both ways.
	person := func(uid string) map[string][]string {
		return map[string][]string{"objectClass": {"inetOrgPerson"}, "uid": {uid}, "cn": {uid}, "sn": {uid}}
	}
	ds1.add(t, "uid=from-ds1,ou=People,"+suffix, person("from-ds1"))
	ds2.add(t, "uid=from-ds2,ou=People,"+suffix, person("from-ds2"))
	waitFor(t, "from-ds1 on ds2", 60*time.Second, func() bool { return ds2.exists(t, "uid=from-ds1,ou=People,"+suffix) })
	waitFor(t, "from-ds2 on ds1", 60*time.Second, func() bool { return ds1.exists(t, "uid=from-ds2,ou=People,"+suffix) })

	// Status and RUV.
	waitFor(t, "agreement status ok", 30*time.Second, func() bool {
		_, body := ds1.do(t, "GET", "/api/v1/replication/agreements"+q, "")
		var list []ds.Agreement
		_ = json.Unmarshal([]byte(body), &list)
		return len(list) == 1 && list[0].Status.State == "ok" && list[0].Status.LastUpdateEnd != nil
	})
	waitFor(t, "both replica IDs in the RUV of ds1", 30*time.Second, func() bool {
		_, body := ds1.do(t, "GET", "/api/v1/replication/ruv"+q, "")
		var ruv ds.RUV
		_ = json.Unmarshal([]byte(body), &ruv)
		ids := []int{}
		for _, r := range ruv.Replicas {
			if r.MaxCSN != "" {
				ids = append(ids, r.ReplicaID)
			}
		}
		return ruv.Generation != "" && slices.Contains(ids, 1) && slices.Contains(ids, 2)
	})

	// Agreement changes: fractional replication, schedule, disable/enable.
	if code, body := ds1.do(t, "PATCH", "/api/v1/replication/agreements/to-ds2"+q, `{"excluded":["telephoneNumber","jpegPhoto"],"description":"itest"}`); code != 200 ||
		!strings.Contains(body, `"excluded":["telephoneNumber","jpegPhoto"]`) {
		t.Errorf("fractional: %d %s", code, body)
	}
	if code, _ := ds1.do(t, "PATCH", "/api/v1/replication/agreements/to-ds2"+q, `{"schedule":"8am-5pm"}`); code != 422 {
		t.Errorf("bad schedule: %d", code)
	}

	// A conflict: the same DN added on both servers while they are apart.
	for _, p := range []struct {
		n    *node
		name string
	}{{ds1, "to-ds2"}, {ds2, "to-ds1"}} {
		if code, body := p.n.do(t, "PATCH", "/api/v1/replication/agreements/"+p.name+q, `{"enabled":false}`); code != 200 || !strings.Contains(body, `"enabled":false`) {
			t.Fatalf("disable %s: %d %s", p.name, code, body)
		}
	}
	twin := "uid=twin,ou=People," + suffix
	ds1.add(t, twin, person("twin"))
	time.Sleep(1100 * time.Millisecond) // a later CSN on ds2
	ds2.add(t, twin, person("twin"))
	for _, p := range []struct {
		n    *node
		name string
	}{{ds1, "to-ds2"}, {ds2, "to-ds1"}} {
		if code, body := p.n.do(t, "PATCH", "/api/v1/replication/agreements/"+p.name+q, `{"enabled":true}`); code != 200 {
			t.Fatalf("enable %s: %d %s", p.name, code, body)
		}
	}
	var conflicts []ds.Conflict
	waitFor(t, "conflict entry", 90*time.Second, func() bool {
		for _, n := range []*node{ds1, ds2} {
			_, body := n.do(t, "GET", "/api/v1/replication/conflicts"+q, "")
			_ = json.Unmarshal([]byte(body), &conflicts)
			if len(conflicts) > 0 {
				return true
			}
		}
		return false
	})
	if conflicts[0].Reason == "" || !strings.Contains(strings.ToLower(conflicts[0].DN), "uid=twin") {
		t.Errorf("conflict = %+v", conflicts[0])
	}

	// Resolve it by keeping the conflict entry under a new RDN; any other
	// conflict entries are deleted. 404: a change from the other server has
	// already replicated.
	kept := false
	for _, n := range []*node{ds1, ds2} {
		_, body := n.do(t, "GET", "/api/v1/replication/conflicts"+q, "")
		var list []ds.Conflict
		_ = json.Unmarshal([]byte(body), &list)
		for _, cf := range list {
			if !kept && !cf.Glue {
				if code, out := n.do(t, "POST", "/api/v1/replication/conflicts/convert?dn="+url.QueryEscape(cf.DN), `{"rdn":"uid=twin-kept"}`); code != 204 {
					t.Errorf("convert conflict on %s: %d %s", n.name, code, out)
				}
				kept = true
				continue
			}
			if code, out := n.do(t, "DELETE", "/api/v1/replication/conflicts?dn="+url.QueryEscape(cf.DN), ""); code != 204 && code != 404 {
				t.Errorf("delete conflict on %s: %d %s", n.name, code, out)
			}
		}
	}
	waitFor(t, "no conflicts left", 60*time.Second, func() bool {
		for _, n := range []*node{ds1, ds2} {
			_, body := n.do(t, "GET", "/api/v1/replication/conflicts"+q, "")
			var left []ds.Conflict
			if json.Unmarshal([]byte(body), &left) != nil || len(left) > 0 {
				return false
			}
		}
		return true
	})
	if !ds1.exists(t, twin) || !ds2.exists(t, twin) {
		t.Error("the winning twin entry is gone")
	}
	kept2 := "uid=twin-kept,ou=People," + suffix
	waitFor(t, "converted entry on both servers", 60*time.Second, func() bool { return ds1.exists(t, kept2) && ds2.exists(t, kept2) })

	// CleanAllRUV refuses the server's own ID.
	if code, _ := ds1.do(t, "POST", "/api/v1/replication/cleanallruv"+q, `{"replicaId":1}`); code != 422 {
		t.Errorf("cleanallruv own id: %d", code)
	}

	// Disable replication on ds2: its agreement goes too.
	if code, body := ds2.do(t, "DELETE", "/api/v1/replication"+q, ""); code != 204 {
		t.Errorf("disable: %d %s", code, body)
	}
	if code, body := ds2.do(t, "GET", "/api/v1/replication"+q, ""); code != 200 || !strings.Contains(body, `"enabled":false`) {
		t.Errorf("after disable: %d %s", code, body)
	}
	if code, _ := ds2.do(t, "GET", "/api/v1/replication/agreements"+q, ""); code != 409 {
		t.Errorf("agreements without replica: %d", code)
	}
}
