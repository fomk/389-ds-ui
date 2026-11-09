//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/entries"
)

// branch creates an isolated OU for one test and removes it afterwards.
func branch(t *testing.T, svc *entries.Service) string {
	t.Helper()
	dn := fmt.Sprintf("ou=t%d,%s", time.Now().UnixNano(), suffix)
	_, err := svc.Add(context.Background(), entries.NewEntry{DN: dn, Attributes: []entries.NewValues{
		{Name: "objectClass", Values: []string{"top", "organizationalUnit"}},
		{Name: "ou", Values: []string{strings.TrimPrefix(strings.SplitN(dn, ",", 2)[0], "ou=")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = svc.Delete(context.Background(), dn, true) })
	return dn
}

func addUser(t *testing.T, svc *entries.Service, parent, uid string) *entries.Entry {
	t.Helper()
	e, err := svc.Add(context.Background(), entries.NewEntry{
		DN: "uid=" + uid + "," + parent,
		Attributes: []entries.NewValues{
			{Name: "objectClass", Values: []string{"top", "person", "organizationalPerson", "inetOrgPerson"}},
			{Name: "uid", Values: []string{uid}},
			{Name: "cn", Values: []string{"User " + uid}},
			{Name: "sn", Values: []string{uid}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func values(e *entries.Entry, name string) []string {
	for _, a := range e.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a.Values
		}
	}
	return nil
}

func TestCreateAndModify(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	e := addUser(t, svc, ou, "alice")

	// Replace, add and delete in one atomic request, guarded by the ETag.
	upd, err := svc.Modify(ctx, e.DN, e.ETag, []entries.Change{
		{Op: "replace", Attr: "cn", Values: []string{"Alice Liddell"}},
		{Op: "add", Attr: "mail", Values: []string{"alice@example.com", "a@example.com"}},
		{Op: "add", Attr: "description", Values: []string{"Иван"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(values(upd, "cn"), []string{"Alice Liddell"}) || len(values(upd, "mail")) != 2 {
		t.Errorf("after modify: %+v", upd.Attributes)
	}
	if upd.ETag == e.ETag {
		t.Error("etag must change after a modification")
	}

	// Stale ETag → precondition failed, nothing applied.
	_, err = svc.Modify(ctx, e.DN, e.ETag, []entries.Change{{Op: "replace", Attr: "cn", Values: []string{"x"}}})
	if !errors.Is(err, entries.ErrPreconditionFailed) {
		t.Errorf("stale etag: %v", err)
	}

	// Delete one value, then the whole attribute; add an auxiliary class with its MUST attributes.
	upd, err = svc.Modify(ctx, e.DN, upd.ETag, []entries.Change{
		{Op: "delete", Attr: "mail", Values: []string{"a@example.com"}},
		{Op: "delete", Attr: "description"},
		{Op: "add", Attr: "objectClass", Values: []string{"posixAccount"}},
		{Op: "add", Attr: "uidNumber", Values: []string{"20001"}},
		{Op: "add", Attr: "gidNumber", Values: []string{"20000"}},
		{Op: "add", Attr: "homeDirectory", Values: []string{"/home/alice"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(values(upd, "mail"), []string{"alice@example.com"}) || values(upd, "description") != nil ||
		!slices.Contains(upd.ObjectClasses, "posixAccount") {
		t.Errorf("after second modify: %v / %+v", upd.ObjectClasses, upd.Attributes)
	}

	// Schema violation from the server is an LDAP error (mapped to 422 by the API).
	_, err = svc.Modify(ctx, e.DN, "", []entries.Change{{Op: "delete", Attr: "sn"}})
	if !ldap.IsErrorWithCode(err, ldap.LDAPResultObjectClassViolation) {
		t.Errorf("removing a MUST attribute: %v", err)
	}

	// Binary values round-trip through base64.
	upd, err = svc.Modify(ctx, e.DN, "", []entries.Change{
		{Op: "add", Attr: "jpegPhoto", Values: []string{"/9j/4AAQ"}, Binary: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := values(upd, "jpegPhoto"); len(v) != 1 || v[0] != "/9j/4AAQ" {
		t.Errorf("jpegPhoto = %v", v)
	}
	if _, err := svc.Modify(ctx, e.DN, "", []entries.Change{{Op: "add", Attr: "jpegPhoto", Values: []string{"%%%"}, Binary: true}}); !errors.Is(err, entries.ErrInvalidChange) {
		t.Errorf("invalid base64: %v", err)
	}
}

func TestRenameAndMove(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	other := branch(t, svc)
	e := addUser(t, svc, ou, "bob")

	res, err := svc.Rename(ctx, entries.RenameRequest{DN: e.DN, NewRDN: "uid=robert", DeleteOldRDN: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.DN != "uid=robert,"+ou {
		t.Errorf("renamed DN = %s", res.DN)
	}
	got, err := svc.Get(ctx, res.DN)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(values(got, "uid"), []string{"robert"}) {
		t.Errorf("deleteOldRDN must drop the old uid: %v", values(got, "uid"))
	}

	moved, err := svc.Rename(ctx, entries.RenameRequest{DN: res.DN, NewRDN: "uid=robert", DeleteOldRDN: true, NewParent: other})
	if err != nil {
		t.Fatal(err)
	}
	if moved.DN != "uid=robert,"+other {
		t.Errorf("moved DN = %s", moved.DN)
	}
	page, _ := svc.Children(ctx, ou, "", 10, false)
	if page.Total != 0 {
		t.Errorf("old parent must be empty (cache invalidated), got %d", page.Total)
	}

	if _, err := svc.Rename(ctx, entries.RenameRequest{DN: moved.DN, NewRDN: "uid=a,ou=b"}); !errors.Is(err, entries.ErrInvalidDN) {
		t.Errorf("multi-RDN newRdn must be rejected: %v", err)
	}
}

func TestPassword(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	e := addUser(t, svc, ou, "carol")

	if _, err := svc.SetPassword(ctx, entries.PasswordRequest{DN: e.DN, Password: "N3w-Passw0rd!"}); err != nil {
		t.Fatal(err)
	}
	if c, err := newClientWith(e.DN, "N3w-Passw0rd!"); err != nil {
		t.Fatal(err)
	} else if err := c.Do(ctx, func(*ldap.Conn) error { return nil }); err != nil {
		t.Errorf("bind with the new password: %v", err)
	}
	got, _ := svc.Get(ctx, e.DN)
	if v := values(got, "userPassword"); len(v) != 1 || !strings.HasPrefix(v[0], "{") {
		t.Errorf("stored password must be hashed and redacted: %v", v)
	}

	// Tests connect over plain LDAP: 389-ds refuses the extended operation there,
	// so generation is unavailable (it works over LDAPI/TLS in real deployments).
	if _, err := svc.SetPassword(ctx, entries.PasswordRequest{DN: e.DN}); !errors.Is(err, entries.ErrPasswordGenerationUnavailable) {
		t.Errorf("generation over plain LDAP: %v", err)
	}
}

func TestDelete(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	sub := "ou=sub," + ou
	if _, err := svc.Add(ctx, entries.NewEntry{DN: sub, Attributes: []entries.NewValues{
		{Name: "objectClass", Values: []string{"top", "organizationalUnit"}}, {Name: "ou", Values: []string{"sub"}},
	}}); err != nil {
		t.Fatal(err)
	}
	addUser(t, svc, sub, "d1")
	addUser(t, svc, sub, "d2")
	addUser(t, svc, ou, "d3")

	if _, err := svc.Delete(ctx, ou, false); !ldap.IsErrorWithCode(err, ldap.LDAPResultNotAllowedOnNonLeaf) {
		t.Errorf("deleting a non-leaf without subtree: %v", err)
	}
	res, err := svc.Delete(ctx, ou, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 5 {
		t.Errorf("deleted = %d, want 5", res.Deleted)
	}
	if _, err := svc.Get(ctx, ou); !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		t.Errorf("branch still exists: %v", err)
	}
}
