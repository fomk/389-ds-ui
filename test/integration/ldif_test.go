//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/go-ldap/ldif"

	"github.com/fomk/389-ds-ui/internal/entries"
)

func TestSearch(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	res, err := svc.Search(ctx, entries.SearchRequest{
		Base: suffix, Scope: "sub", Filter: "(&(objectClass=inetOrgPerson)(departmentNumber=Sales))", Attrs: []string{"uid", "cn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 || res.Truncated {
		t.Fatalf("result = %+v", res)
	}
	for _, e := range res.Entries {
		if len(e.Attributes) != 2 {
			t.Errorf("only requested attributes expected: %+v", e.Attributes)
		}
	}
	res, err = svc.Search(ctx, entries.SearchRequest{Base: suffix, Filter: "(objectClass=*)", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 3 || !res.Truncated {
		t.Errorf("limit: %d entries, truncated=%v", len(res.Entries), res.Truncated)
	}
	if _, err := svc.Search(ctx, entries.SearchRequest{Base: suffix, Filter: "(cn=*"}); !errors.Is(err, entries.ErrInvalidFilter) {
		t.Errorf("bad filter: %v", err)
	}
}

func TestExportLDIFAndCSV(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	var buf bytes.Buffer
	n, err := svc.ExportLDIF(ctx, &buf, entries.ExportRequest{Base: "ou=People," + suffix, Scope: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	if n < 7 {
		t.Errorf("exported %d entries", n)
	}
	parsed, err := ldif.Parse(buf.String())
	if err != nil {
		t.Fatalf("export is not valid LDIF: %v", err)
	}
	if len(parsed.Entries) != n {
		t.Errorf("parsed %d of %d entries", len(parsed.Entries), n)
	}
	if !strings.Contains(buf.String(), "cn:: ") {
		t.Error("non-ASCII values must be base64 (cn:: ...) per RFC 2849")
	}
	if strings.Contains(strings.ToLower(buf.String()), "userpassword") {
		t.Error("password hashes must not be exported by default")
	}
	buf.Reset()
	_, _ = svc.ExportLDIF(ctx, &buf, entries.ExportRequest{Base: "uid=jdoe,ou=People," + suffix, Scope: "base", Passwords: true})
	if !strings.Contains(buf.String(), "userPassword") {
		t.Error("passwords=true must include the hash")
	}

	buf.Reset()
	if _, err := svc.ExportCSV(ctx, &buf, entries.ExportRequest{Base: "ou=Groups," + suffix, Scope: "one",
		Filter: "(cn=admins)", Attrs: []string{"cn", "member"}}); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][0] != "dn" || rows[1][1] != "admins" || !strings.HasPrefix(rows[1][2], "uid=jdoe") {
		t.Errorf("csv = %q", rows)
	}
}

func TestApplyLDIF(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	text := `
dn: cn=imp1,` + ou + `
objectClass: top
objectClass: device
cn: imp1

dn: cn=imp2,` + ou + `
changetype: add
objectClass: top
objectClass: device
cn: imp2

dn: cn=imp1,` + ou + `
changetype: modify
add: description
description: imported
-

dn: cn=missing,` + ou + `
changetype: delete

dn: cn=imp2,` + ou + `
changetype: delete
`
	sum, err := svc.ApplyLDIF(ctx, text, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Applied != 4 || sum.Failed != 1 || sum.Results[3].Error == "" {
		t.Errorf("summary = %+v", sum)
	}
	e, err := svc.Get(ctx, "cn=imp1,"+ou)
	if err != nil || values(e, "description")[0] != "imported" {
		t.Errorf("imp1 = %v, %v", e, err)
	}

	// stopOnError stops at the first failure.
	sum, _ = svc.ApplyLDIF(ctx, "dn: cn=missing,"+ou+"\nchangetype: delete\n\ndn: cn=imp3,"+ou+"\nobjectClass: device\ncn: imp3\n", true)
	if len(sum.Results) != 1 || sum.Failed != 1 {
		t.Errorf("stopOnError summary = %+v", sum)
	}
	if _, err := svc.ApplyLDIF(ctx, "dn: cn=x,"+ou+"\nobjectClass: device\njpegPhoto:< file:///etc/passwd\n", false); !errors.Is(err, entries.ErrInvalidLDIF) {
		t.Errorf("URL values must be rejected: %v", err)
	}
}

func TestReplaceFromLDIF(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()
	ou := branch(t, svc)
	e := addUser(t, svc, ou, "ldifuser")
	if _, err := svc.Modify(ctx, e.DN, "", []entries.Change{{Op: "add", Attr: "description", Values: []string{"to be removed"}}}); err != nil {
		t.Fatal(err)
	}
	cur, _ := svc.Get(ctx, e.DN)
	text := "dn: " + e.DN + `
objectClass: top
objectClass: person
objectClass: organizationalPerson
objectClass: inetOrgPerson
uid: ldifuser
cn: Edited In LDIF
sn: ldifuser
mail: ldif@example.com
`
	upd, err := svc.ReplaceFromLDIF(ctx, e.DN, cur.ETag, text)
	if err != nil {
		t.Fatal(err)
	}
	if values(upd, "cn")[0] != "Edited In LDIF" || values(upd, "mail") == nil || values(upd, "description") != nil {
		t.Errorf("after LDIF replace: %+v", upd.Attributes)
	}
	if _, err := svc.ReplaceFromLDIF(ctx, e.DN, "", "dn: cn=other,"+ou+"\nobjectClass: top\ncn: other\n"); !errors.Is(err, entries.ErrInvalidLDIF) {
		t.Errorf("DN change must be rejected: %v", err)
	}
	if _, err := svc.ReplaceFromLDIF(ctx, e.DN, cur.ETag, text); !errors.Is(err, entries.ErrPreconditionFailed) {
		t.Errorf("stale etag: %v", err)
	}
}
