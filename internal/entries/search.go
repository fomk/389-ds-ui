package entries

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/go-ldap/ldif"

	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

// ErrInvalidFilter is returned for a syntactically invalid LDAP filter.
var ErrInvalidFilter = errors.New("invalid LDAP filter")

// MaxSearchResults caps interactive search results.
const MaxSearchResults = 5000

// SearchRequest describes an interactive search.
type SearchRequest struct {
	Base   string   `json:"base" minLength:"1"`
	Scope  string   `json:"scope" enum:"base,one,sub" default:"sub"`
	Filter string   `json:"filter" default:"(objectClass=*)"`
	Attrs  []string `json:"attrs,omitempty" doc:"Attributes to return; empty returns all user attributes"`
	Limit  int      `json:"limit,omitempty" minimum:"1" maximum:"5000" default:"500"`
}

// SearchHit is one search result.
type SearchHit struct {
	DN         string      `json:"dn"`
	Attributes []Attribute `json:"attributes"`
}

// SearchResult holds results; Truncated means the limit was reached.
type SearchResult struct {
	Entries   []SearchHit `json:"entries"`
	Truncated bool        `json:"truncated"`
}

func scopeOf(s string) (int, error) {
	switch s {
	case "base":
		return ldap.ScopeBaseObject, nil
	case "one":
		return ldap.ScopeSingleLevel, nil
	case "", "sub":
		return ldap.ScopeWholeSubtree, nil
	}
	return 0, fmt.Errorf("%w: unknown scope %q", ErrInvalidChange, s)
}

func validateQuery(base, filter string) (string, error) {
	if !ldapx.ValidDN(base) {
		return "", ErrInvalidDN
	}
	if filter == "" {
		filter = "(objectClass=*)"
	}
	if _, err := ldap.CompileFilter(filter); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidFilter, err)
	}
	return filter, nil
}

// Search runs an interactive search limited to r.Limit entries.
func (s *Service) Search(ctx context.Context, r SearchRequest) (*SearchResult, error) {
	filter, err := validateQuery(r.Base, r.Filter)
	if err != nil {
		return nil, err
	}
	scope, err := scopeOf(r.Scope)
	if err != nil {
		return nil, err
	}
	limit := r.Limit
	if limit <= 0 || limit > MaxSearchResults {
		limit = 500
	}
	sch, _, err := s.schemas.Get(ctx, s.ldap)
	if err != nil {
		return nil, err
	}
	attrs := r.Attrs
	if len(attrs) == 0 {
		attrs = []string{"*"}
	}
	out := &SearchResult{Entries: []SearchHit{}}
	err = s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest(r.Base, scope, ldap.NeverDerefAliases,
			limit, 0, false, filter, attrs, nil))
		if ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			out.Truncated = true
			err = nil
		}
		if err != nil {
			return err
		}
		for _, e := range res.Entries {
			hit := SearchHit{DN: e.DN, Attributes: make([]Attribute, 0, len(e.Attributes))}
			for _, a := range e.Attributes {
				hit.Attributes = append(hit.Attributes, encodeAttribute(a, sch.Attr(a.Name)))
			}
			sortAttributes(hit.Attributes)
			out.Entries = append(out.Entries, hit)
		}
		return nil
	})
	return out, err
}

// ExportRequest describes a streamed export.
type ExportRequest struct {
	Base        string
	Scope       string
	Filter      string
	Attrs       []string // empty: all user attributes ("*")
	Operational bool     // also export operational attributes
	Passwords   bool     // include password hashes (off by default: they are secrets)
}

// ValidateExport checks an export request before streaming starts, so that
// errors can still be reported with a proper HTTP status.
func ValidateExport(r ExportRequest) error {
	if r.Base == "" {
		return ErrInvalidDN
	}
	if _, err := validateQuery(r.Base, r.Filter); err != nil {
		return err
	}
	_, err := scopeOf(r.Scope)
	return err
}

// eachEntry streams search results page by page (Paged Results) to fn.
func (s *Service) eachEntry(ctx context.Context, r ExportRequest, fn func(*ldap.Entry) error) error {
	filter, err := validateQuery(r.Base, r.Filter)
	if err != nil {
		return err
	}
	scope, err := scopeOf(r.Scope)
	if err != nil {
		return err
	}
	attrs := r.Attrs
	if len(attrs) == 0 {
		attrs = []string{"*"}
	}
	if r.Operational {
		attrs = append(attrs, "+")
	}
	return s.ldap.Do(ctx, func(c *ldap.Conn) error {
		paging := ldap.NewControlPaging(500)
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			res, err := c.Search(ldap.NewSearchRequest(r.Base, scope, ldap.NeverDerefAliases,
				0, 0, false, filter, attrs, []ldap.Control{paging}))
			if err != nil {
				return err
			}
			for _, e := range res.Entries {
				if err := fn(e); err != nil {
					return err
				}
			}
			ctrl, ok := ldap.FindControl(res.Controls, ldap.ControlTypePaging).(*ldap.ControlPaging)
			if !ok || len(ctrl.Cookie) == 0 {
				return nil
			}
			paging.SetCookie(ctrl.Cookie)
		}
	})
}

// ExportLDIF writes matching entries as LDIF (RFC 2849) and returns their count.
func (s *Service) ExportLDIF(ctx context.Context, w io.Writer, r ExportRequest) (int, error) {
	sch, _, err := s.schemas.Get(ctx, s.ldap)
	if err != nil {
		return 0, err
	}
	if _, err := io.WriteString(w, "version: 1\n\n"); err != nil {
		return 0, err
	}
	n := 0
	err = s.eachEntry(ctx, r, func(e *ldap.Entry) error {
		n++
		if !r.Passwords {
			stripPasswords(e, sch)
		}
		return ldif.Dump(w, 76, e)
	})
	return n, err
}

// stripPasswords removes password attributes (hashes) from an entry.
func stripPasswords(e *ldap.Entry, sch *schema.Schema) {
	kept := e.Attributes[:0]
	for _, a := range e.Attributes {
		if at := sch.Attr(a.Name); at == nil || at.Kind != schema.KindPassword {
			kept = append(kept, a)
		}
	}
	e.Attributes = kept
}

// ExportCSV writes one row per entry: dn followed by the requested attributes.
// Multiple values are joined with newlines; binary values are base64.
func (s *Service) ExportCSV(ctx context.Context, w io.Writer, r ExportRequest) (int, error) {
	if len(r.Attrs) == 0 {
		return 0, fmt.Errorf("%w: CSV export needs an explicit attribute list", ErrInvalidChange)
	}
	sch, _, err := s.schemas.Get(ctx, s.ldap)
	if err != nil {
		return 0, err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(append([]string{"dn"}, r.Attrs...)); err != nil {
		return 0, err
	}
	n := 0
	err = s.eachEntry(ctx, r, func(e *ldap.Entry) error {
		n++
		if !r.Passwords {
			stripPasswords(e, sch)
		}
		row := []string{e.DN}
		for _, name := range r.Attrs {
			a := findAttr(e, name)
			if a == nil {
				row = append(row, "")
				continue
			}
			enc := encodeAttribute(a, sch.Attr(a.Name))
			row = append(row, strings.Join(enc.Values, "\n"))
		}
		return cw.Write(row)
	})
	cw.Flush()
	if err == nil {
		err = cw.Error()
	}
	return n, err
}

func findAttr(e *ldap.Entry, name string) *ldap.EntryAttribute {
	for _, a := range e.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a
		}
	}
	return nil
}

// --- LDIF import -----------------------------------------------------------

// ApplyResult reports the outcome of one LDIF record.
type ApplyResult struct {
	DN    string `json:"dn"`
	Op    string `json:"op" enum:"add,modify,delete"`
	Error string `json:"error,omitempty"`
}

// ApplySummary summarises an LDIF import.
type ApplySummary struct {
	Applied int           `json:"applied"`
	Failed  int           `json:"failed"`
	Results []ApplyResult `json:"results"`
}

// urlValueRE finds "attr:< url" lines. go-ldap/ldif would read file:// URLs
// from the ds-ui host, so such values are refused before parsing.
var urlValueRE = regexp.MustCompile(`(?m)^[A-Za-z0-9][A-Za-z0-9;.-]*:<`)

// ErrInvalidLDIF is returned for LDIF that cannot be parsed or is not allowed.
var ErrInvalidLDIF = errors.New("invalid LDIF")

func parseLDIF(text string) (*ldif.LDIF, error) {
	if urlValueRE.MatchString(text) {
		return nil, fmt.Errorf("%w: URL values (attr:< …) are not supported", ErrInvalidLDIF)
	}
	l, err := ldif.Parse(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidLDIF, err)
	}
	return l, nil
}

// ApplyLDIF applies content records (as adds) and change records
// (add/modify/delete) in order. modrdn is not supported by the parser.
func (s *Service) ApplyLDIF(ctx context.Context, text string, stopOnError bool) (*ApplySummary, error) {
	l, err := parseLDIF(text)
	if err != nil {
		return nil, err
	}
	sum := &ApplySummary{Results: []ApplyResult{}}
	defer s.children.clear()
	err = s.ldap.Do(ctx, func(c *ldap.Conn) error {
		for _, rec := range l.Entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			var r ApplyResult
			var opErr error
			switch {
			case rec.Entry != nil:
				r = ApplyResult{DN: rec.Entry.DN, Op: "add"}
				req := ldap.NewAddRequest(rec.Entry.DN, nil)
				for _, a := range rec.Entry.Attributes {
					req.Attribute(a.Name, a.Values)
				}
				opErr = c.Add(req)
			case rec.Add != nil:
				r = ApplyResult{DN: rec.Add.DN, Op: "add"}
				opErr = c.Add(rec.Add)
			case rec.Modify != nil:
				r = ApplyResult{DN: rec.Modify.DN, Op: "modify"}
				opErr = c.Modify(rec.Modify)
			case rec.Del != nil:
				r = ApplyResult{DN: rec.Del.DN, Op: "delete"}
				opErr = c.Del(rec.Del)
			default:
				continue
			}
			if opErr != nil {
				if ldapx.IsConnError(opErr) {
					return opErr
				}
				r.Error = opErr.Error()
				sum.Failed++
			} else {
				sum.Applied++
			}
			sum.Results = append(sum.Results, r)
			if opErr != nil && stopOnError {
				return nil
			}
		}
		return nil
	})
	return sum, err
}

// ReplaceFromLDIF makes the entry dn match a single LDIF content record:
// attributes that differ are replaced, missing ones deleted, new ones added.
// The DN must not change (use Rename for that).
func (s *Service) ReplaceFromLDIF(ctx context.Context, dn, ifMatch, text string) (*Entry, error) {
	l, err := parseLDIF(text)
	if err != nil {
		return nil, err
	}
	if len(l.Entries) != 1 || l.Entries[0].Entry == nil {
		return nil, fmt.Errorf("%w: expected exactly one entry record (without changetype)", ErrInvalidLDIF)
	}
	want := l.Entries[0].Entry
	if ldapx.NormDN(want.DN) != ldapx.NormDN(dn) {
		return nil, fmt.Errorf("%w: the DN cannot be changed here; use rename", ErrInvalidLDIF)
	}
	sch, _, err := s.schemas.Get(ctx, s.ldap)
	if err != nil {
		return nil, err
	}
	var cur *ldap.Entry
	err = s.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			1, 0, false, "(objectClass=*)", []string{"*"}, nil))
		if err != nil {
			return err
		}
		if len(res.Entries) == 0 {
			return ldap.NewError(ldap.LDAPResultNoSuchObject, errors.New("entry not found: "+dn))
		}
		cur = res.Entries[0]
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Check the precondition even when nothing changes: a stale editor must
	// learn that the entry moved on.
	if ifMatch != "" && etag(cur, sch) != ifMatch {
		return nil, ErrPreconditionFailed
	}
	changes := diffRaw(cur, want, sch)
	if len(changes) == 0 {
		return s.Get(ctx, dn)
	}
	return s.Modify(ctx, dn, ifMatch, changes)
}

// diffRaw computes changes turning cur into want. Password attributes that
// are absent from want are left alone (the UI shows them redacted).
func diffRaw(cur, want *ldap.Entry, sch *schema.Schema) []Change {
	key := func(n string) string { return strings.ToLower(n) }
	curBy := map[string]*ldap.EntryAttribute{}
	for _, a := range cur.Attributes {
		curBy[key(a.Name)] = a
	}
	wantBy := map[string]*ldap.EntryAttribute{}
	for _, a := range want.Attributes {
		wantBy[key(a.Name)] = a
	}
	var changes []Change
	enc := func(a *ldap.EntryAttribute) Change {
		vals := make([]string, len(a.ByteValues))
		for i, v := range a.ByteValues {
			vals[i] = base64.StdEncoding.EncodeToString(v)
		}
		return Change{Attr: a.Name, Values: vals, Binary: true}
	}
	// objectClass first so that new MUST/MAY attributes are accepted.
	if w, ok := wantBy["objectclass"]; ok && !sameValues(curBy["objectclass"], w) {
		c := enc(w)
		c.Op = "replace"
		changes = append(changes, c)
	}
	for k, w := range wantBy {
		if k == "objectclass" {
			continue
		}
		if c, ok := curBy[k]; !ok || !sameValues(c, w) {
			ch := enc(w)
			ch.Op = "replace"
			changes = append(changes, ch)
		}
	}
	for k, c := range curBy {
		if _, ok := wantBy[k]; ok {
			continue
		}
		if at := sch.Attr(c.Name); at != nil && (at.Kind == schema.KindPassword || at.Operational()) {
			continue
		}
		changes = append(changes, Change{Op: "delete", Attr: c.Name})
	}
	return changes
}

func sameValues(a, b *ldap.EntryAttribute) bool {
	if a == nil || b == nil || len(a.ByteValues) != len(b.ByteValues) {
		return false
	}
	set := map[string]int{}
	for _, v := range a.ByteValues {
		set[string(v)]++
	}
	for _, v := range b.ByteValues {
		if set[string(v)] == 0 {
			return false
		}
		set[string(v)]--
	}
	return true
}
