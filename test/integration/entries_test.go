//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func newEntries(t *testing.T) (*entries.Service, *schema.Cache) {
	t.Helper()
	c := newClient(t)
	cache := schema.NewCache(time.Minute, nil)
	return entries.New(c, cache, nil), cache
}

func TestSchemaFrom389(t *testing.T) {
	c := newClient(t)
	cache := schema.NewCache(time.Minute, nil)
	s, version, err := cache.Get(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if version == "" || len(s.AttributeTypes) < 500 || len(s.ObjectClasses) < 100 {
		t.Fatalf("suspiciously small schema: %d attrs, %d classes", len(s.AttributeTypes), len(s.ObjectClasses))
	}
	must, may := s.Allowed([]string{"top", "inetOrgPerson", "posixAccount"})
	for _, a := range []string{"cn", "sn", "uid", "uidNumber", "gidNumber", "homeDirectory", "objectClass"} {
		if !slices.Contains(must, a) {
			t.Errorf("MUST lacks %s: %v", a, must)
		}
	}
	if !slices.Contains(may, "mail") || !slices.Contains(may, "jpegPhoto") {
		t.Errorf("MAY lacks mail/jpegPhoto")
	}
	if s.Attr("userPassword").Kind != schema.KindPassword || s.Attr("member").Kind != schema.KindDN {
		t.Error("kinds of userPassword/member")
	}
}

func TestGetEntry(t *testing.T) {
	svc, _ := newEntries(t)
	e, err := svc.Get(context.Background(), "uid=ivanov,ou=People,dc=example,dc=com")
	if err != nil {
		t.Fatal(err)
	}
	if e.RDN != "uid=ivanov" || !slices.Contains(e.ObjectClasses, "inetOrgPerson") || e.ETag == "" {
		t.Errorf("entry = %+v", e)
	}
	byName := map[string]entries.Attribute{}
	for _, a := range e.Attributes {
		byName[strings.ToLower(a.Name)] = a
	}
	if cn := byName["cn"]; cn.Binary || cn.Values[0] != "Иван Иванов" {
		t.Errorf("cn = %+v", cn)
	}
	if pw := byName["userpassword"]; !pw.Redacted || !strings.HasPrefix(pw.Values[0], "{") || strings.Contains(pw.Values[0], "$") {
		t.Errorf("password must be reduced to its scheme: %+v", pw)
	}
	if ts := byName["createtimestamp"]; !ts.Operational {
		t.Errorf("createTimestamp must be operational: %+v", ts)
	}

	again, _ := svc.Get(context.Background(), "UID=IVANOV, ou=people, dc=example, dc=com")
	if again.ETag != e.ETag {
		t.Error("etag must be stable between reads")
	}
	if _, err := svc.Get(context.Background(), "uid=nobody,dc=example,dc=com"); err == nil {
		t.Error("missing entry must fail")
	}
}

func TestChildrenAndRoots(t *testing.T) {
	svc, _ := newEntries(t)
	ctx := context.Background()

	roots, err := svc.Roots(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	var dns []string
	for _, r := range roots {
		dns = append(dns, r.DN)
	}
	if !slices.Contains(dns, suffix) || !slices.Contains(dns, "cn=config") {
		t.Errorf("roots = %v", dns)
	}

	page, err := svc.Children(ctx, "ou=People,dc=example,dc=com", "", 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 6 || len(page.Items) != 4 || page.NextCursor == "" {
		t.Fatalf("first page = %+v", page)
	}
	if page.Items[0].RDN != "uid=asmith" {
		t.Errorf("children must be sorted by RDN: %s", page.Items[0].RDN)
	}
	next, err := svc.Children(ctx, "ou=People,dc=example,dc=com", page.NextCursor, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 2 || next.NextCursor != "" {
		t.Errorf("second page = %+v", next)
	}

	top, _ := svc.Children(ctx, suffix, "", 100, true)
	for _, it := range top.Items {
		if it.RDN == "ou=People" && (!it.HasChildren || it.NumSubordinates != 6) {
			t.Errorf("ou=People summary = %+v", it)
		}
	}
}

func TestBrowserAPI(t *testing.T) {
	svc, cache := newEntries(t)
	h := api.NewHandler(api.Deps{
		LDAP: newClient(t), Schemas: cache, Entries: svc,
		Lifecycle: providers.NoLifecycle{}, Version: "itest",
	})
	get := func(path string, hdr ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/api/v1/schema")
	if rec.Code != 200 || rec.Header().Get("ETag") == "" {
		t.Fatalf("schema: %d", rec.Code)
	}
	if rec2 := get("/api/v1/schema", "If-None-Match", rec.Header().Get("ETag")); rec2.Code != http.StatusNotModified {
		t.Errorf("schema revalidation: %d", rec2.Code)
	}

	rec = get("/api/v1/entries?dn=" + url.QueryEscape("cn=admins,ou=Groups,dc=example,dc=com"))
	if rec.Code != 200 || rec.Header().Get("ETag") == "" {
		t.Fatalf("entry: %d %s", rec.Code, rec.Body)
	}
	var e entries.Entry
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if e.DN == "" || len(e.Attributes) == 0 {
		t.Errorf("entry = %+v", e)
	}

	if rec = get("/api/v1/entries?dn=" + url.QueryEscape("uid=nobody,dc=example,dc=com")); rec.Code != 404 {
		t.Errorf("missing entry: %d", rec.Code)
	}
	if rec = get("/api/v1/entries?dn=not-a-dn"); rec.Code != 400 {
		t.Errorf("invalid DN: %d", rec.Code)
	}
	if rec = get("/api/v1/entries/children?dn=" + url.QueryEscape(suffix) + "&cursor=x"); rec.Code != 400 {
		t.Errorf("invalid cursor: %d", rec.Code)
	}
}

func TestChildrenServerPaging(t *testing.T) {
	svc, _ := newEntries(t)
	svc.SetSortThreshold(3) // ou=People has 6+ children → server-side paging
	ctx := context.Background()
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		page, err := svc.Children(ctx, "ou=People,"+suffix, cursor, 2, false)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if page.Sorted {
			t.Fatal("large branch must be listed in server order")
		}
		for _, it := range page.Items {
			if seen[it.DN] {
				t.Errorf("duplicate %s", it.DN)
			}
			seen[it.DN] = true
		}
		if page.NextCursor == "" {
			break
		}
		if !strings.HasPrefix(page.NextCursor, "p:") {
			t.Fatalf("server paging cursor expected, got %q", page.NextCursor)
		}
		cursor = page.NextCursor
		if pages > 20 {
			t.Fatal("paging does not terminate")
		}
	}
	total, _ := svc.Children(ctx, "ou=People,"+suffix, "", 1000, true)
	if len(seen) != total.Total || pages < 3 {
		t.Errorf("paged through %d entries in %d pages, total %d", len(seen), pages, total.Total)
	}
}
