//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestCollectionsAndPluginTasks(t *testing.T) {
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	cols, err := pages.LoadCollections()
	if err != nil {
		t.Fatal(err)
	}
	c := newClient(t)
	bus := &events.Bus{}
	tasks := ds.NewTasks(c, bus, nil)
	h := api.NewHandler(api.Deps{LDAP: c, Pages: pages.New(defs, cols), Tasks: tasks, Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}
	people := "ou=People," + suffix
	groups := "ou=Groups," + suffix

	// Every collection can be listed on a fresh server.
	for _, col := range cols {
		if col.Under != "" {
			continue
		}
		if code, body := do("GET", "/api/v1/collections/"+col.ID, ""); code != 200 {
			t.Errorf("list %s: %d %s", col.ID, code, body)
		}
	}

	// Linked Attributes: create, change, required field, delete.
	if code, body := do("POST", "/api/v1/collections/linked-attributes", `{"name":"itest manager","values":{"linkType":["manager"]}}`); code != 422 {
		t.Errorf("missing required: %d %s", code, body)
	}
	if code, body := do("POST", "/api/v1/collections/linked-attributes",
		`{"name":"itest manager","values":{"linkType":["manager"],"managedType":["directReports"],"linkScope":["`+suffix+`"]}}`); code != 201 {
		t.Fatalf("create linked: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/collections/linked-attributes/items/itest%20manager", "")
	if code, body := do("PATCH", "/api/v1/collections/linked-attributes/items/itest%20manager", `{"values":{"linkScope":[]}}`); code != 200 ||
		strings.Contains(body, "linkScope") {
		t.Errorf("clear linkScope: %d %s", code, body)
	}
	if code, body := do("PATCH", "/api/v1/collections/linked-attributes/items/itest%20manager", `{"values":{"managedType":[]}}`); code != 422 {
		t.Errorf("clear required: %d %s", code, body)
	}

	// Auto Membership: a definition with a nested rule; deleting the
	// definition removes the rule.
	if code, body := do("POST", "/api/v1/collections/automember-definitions",
		`{"name":"itest people","values":{"autoMemberScope":["`+people+`"],"autoMemberFilter":["objectClass=inetOrgPerson"],"autoMemberGroupingAttr":["member:dn"],"autoMemberDefaultGroup":["cn=Staff,`+groups+`"]}}`); code != 201 {
		t.Fatalf("create definition: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/collections/automember-definitions/items/itest%20people", "")
	if code, _ := do("GET", "/api/v1/collections/automember-rules", ""); code != 422 {
		t.Errorf("nested without parent: %d", code)
	}
	if code, body := do("POST", "/api/v1/collections/automember-rules?parent=itest%20people",
		`{"name":"admins","values":{"autoMemberTargetGroup":["cn=Admins,`+groups+`"],"autoMemberInclusiveRegex":["uid=^a.*"]}}`); code != 201 {
		t.Fatalf("create rule: %d %s", code, body)
	}
	code, body := do("GET", "/api/v1/collections/automember-rules?parent=itest%20people", "")
	var st pages.CollectionState
	if code != 200 || json.Unmarshal([]byte(body), &st) != nil || len(st.Items) != 1 || st.Items[0].Values["autoMemberInclusiveRegex"][0] != "uid=^a.*" {
		t.Errorf("rules: %d %s", code, body)
	}

	// DNA range.
	if code, body := do("POST", "/api/v1/collections/dna-ranges",
		`{"name":"itest uid numbers","values":{"dnaType":["uidNumber","gidNumber"],"dnaScope":["`+suffix+`"],"dnaFilter":["(objectClass=posixAccount)"],"dnaNextValue":["5000"],"dnaMagicRegen":["-1"]}}`); code != 201 {
		t.Errorf("create DNA range: %d %s", code, body)
	}
	defer do("DELETE", "/api/v1/collections/dna-ranges/items/itest%20uid%20numbers", "")

	// Plug-in tasks.
	if code, body := do("POST", "/api/v1/plugin-tasks/memberof-fixup", `{"params":{}}`); code != 422 {
		t.Errorf("memberof fixup without basedn: %d %s", code, body)
	}
	for _, run := range []struct{ id, params string }{
		{"automember-rebuild", `{"params":{"basedn":"` + people + `","filter":"(objectClass=inetOrgPerson)"}}`},
		{"linkedattrs-fixup", `{"params":{}}`},
	} {
		code, body := do("POST", "/api/v1/plugin-tasks/"+run.id, run.params)
		var task ds.Task
		if code != 202 || json.Unmarshal([]byte(body), &task) != nil {
			t.Errorf("%s: %d %s", run.id, code, body)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		done, err := tasks.Wait(ctx, task.ID)
		cancel()
		if err != nil || done.Status != ds.TaskSucceeded {
			t.Errorf("%s task: %+v %v", run.id, done, err)
		}
	}

	if code, _ := do("DELETE", "/api/v1/collections/automember-definitions/items/itest%20people", ""); code != 204 {
		t.Errorf("delete definition: %d", code)
	}
	if code, _ := do("GET", "/api/v1/collections/automember-rules?parent=itest%20people", ""); code != 404 {
		t.Errorf("rules of a deleted definition: %d", code)
	}
}
