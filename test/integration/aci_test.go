//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

func TestACIsAndEffectiveRights(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	people := "ou=People," + suffix
	user := "uid=jdoe," + people
	const selfRead = `(targetattr="cn || sn || mail")(version 3.0; acl "itest self"; allow (read,search,compare) userdn="ldap:///self";)`

	err := c.Do(ctx, func(conn *ldap.Conn) error {
		before, err := ds.ListACIs(conn, people, false)
		if err != nil {
			return err
		}
		var keep []string
		if len(before) == 1 {
			keep = before[0].ACIs
		}
		defer func() { _ = ds.SetACIs(conn, people, keep) }()

		if err := ds.SetACIs(conn, people, append(append([]string{}, keep...), selfRead)); err != nil {
			t.Fatalf("set: %v", err)
		}
		// Invalid ACIs are rejected by the server.
		if err := ds.SetACIs(conn, people, []string{`(targetattr="cn")(version 3.0; acl "x"; allow (read)`}); err == nil {
			t.Error("invalid ACI accepted")
		}
		list, err := ds.ListACIs(conn, suffix, true)
		if err != nil {
			return err
		}
		found := false
		for _, e := range list {
			for _, a := range e.ACIs {
				found = found || a == selfRead
			}
		}
		if !found {
			t.Errorf("ACI not listed: %+v", list)
		}
		inh, err := ds.InheritedACIs(conn, user)
		if err != nil {
			return err
		}
		if len(inh) == 0 || inh[0].DN != people {
			t.Errorf("inherited: %+v", inh)
		}

		// Effective rights: the user may read its own cn but not write it;
		// Directory Manager may do everything; anonymous sees less.
		self, err := ds.GetEffectiveRights(conn, user, user, nil)
		if err != nil {
			t.Fatalf("GER self: %v", err)
		}
		if r := self.Attrs["cn"]; r == "" || !containsAll(r, "rsc") || containsAll(r, "w") {
			t.Errorf("self rights on cn = %q (%+v)", r, self)
		}
		dm, err := ds.GetEffectiveRights(conn, user, dmDN, []string{"telephoneNumber"})
		if err != nil {
			return err
		}
		if !containsAll(dm.Entry, "vadn") || !containsAll(dm.Attrs["telephoneNumber"], "rscwo") {
			t.Errorf("DM rights = %+v", dm)
		}
		anon, err := ds.GetEffectiveRights(conn, user, "", nil)
		if err != nil {
			t.Fatalf("GER anonymous: %v", err)
		}
		if containsAll(anon.Attrs["userPassword"], "r") || containsAll(anon.Entry, "d") {
			t.Errorf("anonymous rights = %+v", anon)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func containsAll(s, chars string) bool {
	for _, ch := range chars {
		found := false
		for _, x := range s {
			found = found || x == ch
		}
		if !found {
			return false
		}
	}
	return true
}
