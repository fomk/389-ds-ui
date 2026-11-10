package pages

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// Collection describes a set of similar entries below one container, such
// as the definitions of a plug-in: each item is cn=<name>,<container> with
// the given object classes and fields. A nested collection keeps its items
// below the items of another one (Auto Membership regex rules below their
// definition).
type Collection struct {
	ID            string      `yaml:"id" json:"id"`
	Plugin        string      `yaml:"plugin,omitempty" json:"plugin,omitempty" doc:"cn of the plug-in the items configure"`
	Order         int         `yaml:"order" json:"order"`
	Parent        string      `yaml:"parent,omitempty" json:"-" doc:"Container DN"`
	Under         string      `yaml:"under,omitempty" json:"under,omitempty" doc:"Items live below the items of this collection (?parent=<name>)"`
	ObjectClasses []string    `yaml:"objectClasses" json:"-"`
	Use           []string    `yaml:"use,omitempty" json:"-"`
	Fields        []PageField `yaml:"fields" json:"fields"`
}

// Item is one entry of a collection.
type Item struct {
	Name   string              `json:"name" doc:"cn"`
	DN     string              `json:"dn"`
	Values map[string][]string `json:"values"`
}

// CollectionState is a collection with its items under one parent.
type CollectionState struct {
	Collection *Collection         `json:"collection"`
	Parent     string              `json:"parent,omitempty" doc:"Name of the parent item for nested collections"`
	Items      []Item              `json:"items"`
	Options    map[string][]string `json:"options,omitempty"`
}

// CollectionSummary lists a collection.
type CollectionSummary struct {
	ID     string `json:"id"`
	Plugin string `json:"plugin,omitempty"`
	Order  int    `json:"order"`
	Under  string `json:"under,omitempty"`
}

var itemNameRE = regexp.MustCompile(`^[^,=+"\\<>;#][^,=+"\\<>;]*$`)

func (c *Collection) validate() error {
	if !idRE.MatchString(c.ID) {
		return fmt.Errorf("invalid collection id %q", c.ID)
	}
	switch {
	case c.Parent != "" && c.Under != "":
		return fmt.Errorf("collection %s: parent and under exclude each other", c.ID)
	case c.Parent != "":
		if _, err := ldap.ParseDN(c.Parent); err != nil {
			return fmt.Errorf("collection %s: invalid parent %q", c.ID, c.Parent)
		}
	case c.Under == "":
		return fmt.Errorf("collection %s: parent or under is needed", c.ID)
	}
	if len(c.ObjectClasses) == 0 {
		return fmt.Errorf("collection %s: objectClasses are needed", c.ID)
	}
	if len(c.Fields) == 0 {
		return fmt.Errorf("collection %s: no fields", c.ID)
	}
	return validateFields("collection "+c.ID, c.Fields)
}

func (c *Collection) field(attr string) *PageField {
	for i := range c.Fields {
		if strings.EqualFold(c.Fields[i].Attr, attr) {
			return &c.Fields[i]
		}
	}
	return nil
}

// Collections lists the loaded collections.
func (s *Service) Collections() []CollectionSummary {
	out := []CollectionSummary{}
	for _, c := range s.cols {
		out = append(out, CollectionSummary{ID: c.ID, Plugin: c.Plugin, Order: c.Order, Under: c.Under})
	}
	return out
}

func (s *Service) collection(id string) (*Collection, error) {
	for _, c := range s.cols {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: collection %s", ErrNotFound, id)
}

func itemDN(name, parent string) string { return "cn=" + ldap.EscapeDN(name) + "," + parent }

// containerDN resolves where the items of col live; parent names the item
// of the enclosing collection for nested ones.
func (s *Service) containerDN(c *ldap.Conn, col *Collection, parent string) (string, error) {
	if col.Under == "" {
		if parent != "" {
			return "", fmt.Errorf("%w: collection %s is not nested", ErrInvalidValue, col.ID)
		}
		return col.Parent, nil
	}
	if parent == "" {
		return "", fmt.Errorf("%w: collection %s needs the parent item", ErrInvalidValue, col.ID)
	}
	up, err := s.collection(col.Under)
	if err != nil {
		return "", err
	}
	upDN, err := s.containerDN(c, up, "")
	if err != nil {
		return "", err
	}
	dn := itemDN(parent, upDN)
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"1.1"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || err == nil && len(res.Entries) == 0 {
		return "", fmt.Errorf("%w: %s item %s", ErrNotFound, up.ID, parent)
	}
	return dn, err
}

func (col *Collection) attrs() []string {
	out := []string{"cn"}
	for _, f := range col.Fields {
		out = append(out, f.Attr)
	}
	return out
}

func toItem(col *Collection, e *ldap.Entry) Item {
	it := Item{Name: e.GetAttributeValue("cn"), DN: e.DN, Values: map[string][]string{}}
	for _, f := range col.Fields {
		if v := clean(e.GetEqualFoldAttributeValues(f.Attr)); len(v) > 0 {
			it.Values[f.Attr] = v
		}
	}
	return it
}

// ListItems returns the items of a collection (below parent, if nested).
func (s *Service) ListItems(c *ldap.Conn, id, parent string) (*CollectionState, error) {
	col, err := s.collection(id)
	if err != nil {
		return nil, err
	}
	container, err := s.containerDN(c, col, parent)
	if err != nil {
		return nil, err
	}
	st := &CollectionState{Collection: col, Parent: parent, Items: []Item{}}
	res, err := c.Search(ldap.NewSearchRequest(container, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass="+ldap.EscapeFilter(col.ObjectClasses[len(col.ObjectClasses)-1])+")", col.attrs(), nil))
	if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, err
	}
	if res != nil {
		for _, e := range res.Entries {
			st.Items = append(st.Items, toItem(col, e))
		}
	}
	slices.SortFunc(st.Items, func(a, b Item) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, f := range col.Fields {
		if f.OptionsFrom != nil {
			o, err := options(c, f.OptionsFrom)
			if err != nil {
				return nil, err
			}
			if st.Options == nil {
				st.Options = map[string][]string{}
			}
			st.Options[f.Attr] = o
		}
	}
	return st, nil
}

// normalizeAll checks values against the fields; unknown attributes and
// empty required fields are errors.
func normalizeAll(col *Collection, values map[string][]string, options map[string][]string, creating bool) (map[string][]string, error) {
	out := map[string][]string{}
	for attr, vals := range values {
		f := col.field(attr)
		if f == nil {
			return nil, fmt.Errorf("%w: %s is not a field of %s", ErrInvalidValue, attr, col.ID)
		}
		norm, err := f.Normalize(vals, options[f.Attr])
		if err != nil {
			return nil, err
		}
		out[f.Attr] = norm
	}
	for _, f := range col.Fields {
		v, given := out[f.Attr]
		if f.Required && len(v) == 0 && (creating || given) {
			return nil, fmt.Errorf("%w: %s is required", ErrInvalidValue, f.Attr)
		}
	}
	return out, nil
}

// CreateItem adds cn=<name> to the collection.
func (s *Service) CreateItem(c *ldap.Conn, id, parent, name string, values map[string][]string) (*Item, error) {
	col, err := s.collection(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if !itemNameRE.MatchString(name) {
		return nil, fmt.Errorf("%w: name %q", ErrInvalidValue, name)
	}
	container, err := s.containerDN(c, col, parent)
	if err != nil {
		return nil, err
	}
	st, err := s.ListItems(c, id, parent)
	if err != nil {
		return nil, err
	}
	norm, err := normalizeAll(col, values, st.Options, true)
	if err != nil {
		return nil, err
	}
	req := ldap.NewAddRequest(itemDN(name, container), nil)
	req.Attribute("objectClass", col.ObjectClasses)
	req.Attribute("cn", []string{name})
	for k, v := range norm {
		if len(v) > 0 {
			req.Attribute(k, v)
		}
	}
	if err := c.Add(req); err != nil {
		return nil, err
	}
	return s.getItem(c, col, itemDN(name, container))
}

func (s *Service) getItem(c *ldap.Conn, col *Collection, dn string) (*Item, error) {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", col.attrs(), nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || err == nil && len(res.Entries) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, dn)
	}
	if err != nil {
		return nil, err
	}
	it := toItem(col, res.Entries[0])
	return &it, nil
}

// UpdateItem changes the given fields of an item; unchanged ones are skipped.
func (s *Service) UpdateItem(c *ldap.Conn, id, parent, name string, values map[string][]string) (*Item, error) {
	col, err := s.collection(id)
	if err != nil {
		return nil, err
	}
	container, err := s.containerDN(c, col, parent)
	if err != nil {
		return nil, err
	}
	dn := itemDN(name, container)
	cur, err := s.getItem(c, col, dn)
	if err != nil {
		return nil, err
	}
	st, err := s.ListItems(c, id, parent)
	if err != nil {
		return nil, err
	}
	norm, err := normalizeAll(col, values, st.Options, false)
	if err != nil {
		return nil, err
	}
	req := ldap.NewModifyRequest(dn, nil)
	for attr, v := range norm {
		old := cur.Values[attr]
		switch {
		case sameValues(old, v):
		case len(v) > 0:
			req.Replace(attr, v)
		default:
			req.Delete(attr, nil)
		}
	}
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	return s.getItem(c, col, dn)
}

// DeleteItem removes an item together with the entries below it.
func (s *Service) DeleteItem(c *ldap.Conn, id, parent, name string) error {
	col, err := s.collection(id)
	if err != nil {
		return err
	}
	container, err := s.containerDN(c, col, parent)
	if err != nil {
		return err
	}
	dn := itemDN(name, container)
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=*)", []string{"1.1"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: %s", ErrNotFound, dn)
	}
	if err != nil {
		return err
	}
	// Deepest first.
	depth := func(d string) int { return strings.Count(d, ",") }
	slices.SortFunc(res.Entries, func(a, b *ldap.Entry) int { return depth(b.DN) - depth(a.DN) })
	for _, e := range res.Entries {
		if err := c.Del(ldap.NewDelRequest(e.DN, nil)); err != nil {
			return err
		}
	}
	return nil
}
