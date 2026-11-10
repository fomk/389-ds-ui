package pages

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// ErrNotFound is returned for an unknown page or one that does not apply to
// this server.
var ErrNotFound = errors.New("page not found")

const ldbmConfigDN = "cn=config,cn=ldbm database,cn=plugins,cn=config"

// Service serves the loaded pages against a server connection.
type Service struct {
	pages []*Page
	cols  []*Collection
}

// New wraps loaded pages and collections.
func New(pages []*Page, cols []*Collection) *Service { return &Service{pages: pages, cols: cols} }

// PageSummary is a page in the menu.
type PageSummary struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	Order   int    `json:"order"`
	Plugin  string `json:"plugin,omitempty"`
	// Pages for entries named by the caller are not in the settings menu.
	EntryClass string `json:"entryClass,omitempty"`
}

// PageState is a page together with the current values on the server.
type PageState struct {
	Page    *Page               `json:"page"`
	Values  map[string][]string `json:"values" doc:"Current values by attribute; absent means the server default"`
	Options map[string][]string `json:"options,omitempty" doc:"Enum values read from the server, by attribute"`
}

// Facts reads the server properties pages can depend on.
func Facts(c *ldap.Conn) (map[string]string, error) {
	res, err := c.Search(ldap.NewSearchRequest(ldbmConfigDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"nsslapd-backend-implement"}, nil))
	if err != nil {
		return nil, err
	}
	facts := map[string]string{"dbImpl": "bdb"}
	if len(res.Entries) == 1 {
		if v := res.Entries[0].GetAttributeValue("nsslapd-backend-implement"); v != "" {
			facts["dbImpl"] = strings.ToLower(v)
		}
	}
	return facts, nil
}

// List returns the pages that apply to the server.
func (s *Service) List(c *ldap.Conn) ([]PageSummary, error) {
	facts, err := Facts(c)
	if err != nil {
		return nil, err
	}
	out := []PageSummary{}
	for _, p := range s.pages {
		if p.Applies(facts) {
			out = append(out, PageSummary{ID: p.ID, Section: p.Section, Order: p.Order, Plugin: p.Plugin, EntryClass: p.EntryClass})
		}
	}
	return out, nil
}

// page finds a page; for entryClass pages it returns a copy bound to dn,
// after checking that the entry exists and has the class.
func (s *Service) page(c *ldap.Conn, id, dn string) (*Page, error) {
	p, err := s.find(c, id)
	if err != nil {
		return nil, err
	}
	if p.EntryClass == "" {
		if dn != "" && !strings.EqualFold(dn, p.DN) {
			return nil, fmt.Errorf("%w: page %s has a fixed DN", ErrInvalidValue, id)
		}
		return p, nil
	}
	if _, err := ldap.ParseDN(dn); err != nil || dn == "" {
		return nil, fmt.Errorf("%w: page %s needs the DN of a %s entry", ErrInvalidValue, id, p.EntryClass)
	}
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass="+ldap.EscapeFilter(p.EntryClass)+")", []string{"1.1"}, nil))
	if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, err
	}
	if err != nil || len(res.Entries) == 0 {
		return nil, fmt.Errorf("%w: %s is not a %s entry", ErrNotFound, dn, p.EntryClass)
	}
	bound := *p
	bound.DN = dn
	return &bound, nil
}

func (s *Service) find(c *ldap.Conn, id string) (*Page, error) {
	for _, p := range s.pages {
		if p.ID != id {
			continue
		}
		if len(p.When) > 0 {
			facts, err := Facts(c)
			if err != nil {
				return nil, err
			}
			if !p.Applies(facts) {
				break
			}
		}
		return p, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
}

// Get reads a page and its current values. dn names the entry of an
// entryClass page and is empty otherwise.
func (s *Service) Get(c *ldap.Conn, id, dn string) (*PageState, error) {
	p, err := s.page(c, id, dn)
	if err != nil {
		return nil, err
	}
	return read(c, p)
}

func read(c *ldap.Conn, p *Page) (*PageState, error) {
	attrs := make([]string, len(p.Fields))
	for i, f := range p.Fields {
		attrs[i] = f.Attr
	}
	res, err := c.Search(ldap.NewSearchRequest(p.DN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", attrs, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		// cn=config answers a missing base with success and no entries.
		return nil, fmt.Errorf("%w: entry %s is missing", ErrNotFound, p.DN)
	}
	st := &PageState{Page: p, Values: map[string][]string{}}
	for _, f := range p.Fields {
		if v := res.Entries[0].GetEqualFoldAttributeValues(f.Attr); len(v) > 0 {
			st.Values[f.Attr] = v
		}
		if f.OptionsFrom != nil {
			opts, err := options(c, f.OptionsFrom)
			if err != nil {
				return nil, err
			}
			if st.Options == nil {
				st.Options = map[string][]string{}
			}
			st.Options[f.Attr] = opts
		}
	}
	return st, nil
}

func options(c *ldap.Conn, o *OptionsFrom) ([]string, error) {
	res, err := c.Search(ldap.NewSearchRequest(o.Base, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=*)", []string{o.Attr}, nil))
	if err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			return []string{}, nil
		}
		return nil, err
	}
	out := []string{}
	for _, e := range res.Entries {
		if v := e.GetEqualFoldAttributeValue(o.Attr); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out, nil
}

// PageUpdate is the result of an update.
type PageUpdate struct {
	State   *PageState `json:"state"`
	Changed []string   `json:"changed" doc:"Attributes that were modified"`
	Restart []string   `json:"restart" doc:"Changed attributes that apply only after a server restart"`
	// Values before the update and the values written, by changed attribute
	// (for the restart registry).
	Before map[string][]string `json:"-"`
	After  map[string][]string `json:"-"`
}

// Update writes the given values; attributes whose values are unchanged are
// skipped, an empty list removes the attribute (back to the server default).
func (s *Service) Update(c *ldap.Conn, id, dn string, values map[string][]string) (*PageUpdate, error) {
	p, err := s.page(c, id, dn)
	if err != nil {
		return nil, err
	}
	cur, err := read(c, p)
	if err != nil {
		return nil, err
	}
	req := ldap.NewModifyRequest(p.DN, nil)
	res := &PageUpdate{Changed: []string{}, Restart: []string{}, Before: map[string][]string{}, After: map[string][]string{}}
	for attr, vals := range values {
		f := p.Field(attr)
		if f == nil {
			return nil, fmt.Errorf("%w: %s is not on page %s", ErrInvalidValue, attr, p.ID)
		}
		old := cur.Values[f.Attr]
		if sameValues(old, vals) {
			continue
		}
		norm, err := f.Normalize(vals, cur.Options[f.Attr])
		if err != nil {
			return nil, err
		}
		if sameValues(old, norm) {
			continue
		}
		switch {
		case len(norm) > 0:
			req.Replace(f.Attr, norm)
		case len(old) > 0:
			req.Delete(f.Attr, nil)
		default:
			continue
		}
		res.Changed = append(res.Changed, f.Attr)
		res.Before[f.Attr], res.After[f.Attr] = clean(old), norm
		if f.Restart {
			res.Restart = append(res.Restart, f.Attr)
		}
	}
	slices.Sort(res.Changed)
	slices.Sort(res.Restart)
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	if res.State, err = read(c, p); err != nil {
		return nil, err
	}
	return res, nil
}

// sameValues compares value lists ignoring surrounding blanks and empties;
// order matters only for lists, where the server keeps it anyway.
func sameValues(a, b []string) bool {
	a, b = clean(a), clean(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clean drops blank values ("attr:" in cn=config means unset).
func clean(v []string) []string {
	out := []string{}
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
