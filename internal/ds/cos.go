package ds

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// CoS kinds.
const (
	CoSPointer  = "pointer"  // one template, cosTemplateDN
	CoSIndirect = "indirect" // the template is the entry named by cosIndirectSpecifier
	CoSClassic  = "classic"  // the template below cosTemplateDN named by the value of cosSpecifier
)

// CoSDefinition is a Class of Service definition: it adds the attributes in
// cosAttribute to entries of its subtree, with values from templates.
type CoSDefinition struct {
	DN                string        `json:"dn"`
	Name              string        `json:"name"`
	Kind              string        `json:"kind" enum:"pointer,indirect,classic"`
	Description       string        `json:"description,omitempty"`
	Attributes        []string      `json:"attributes" doc:"cosAttribute values: name [default|override|operational|operational-default|merge-schemes]"`
	TemplateDN        string        `json:"templateDN,omitempty" doc:"Pointer: the template; classic: the container of the templates"`
	Specifier         string        `json:"specifier,omitempty" doc:"Classic: attribute whose value names the template"`
	IndirectSpecifier string        `json:"indirectSpecifier,omitempty" doc:"Indirect: attribute holding the template DN"`
	Templates         []CoSTemplate `json:"templates"`
}

// CoSTemplate holds values for a CoS definition.
type CoSTemplate struct {
	DN       string              `json:"dn"`
	Name     string              `json:"name"`
	Priority *int                `json:"priority,omitempty" doc:"cosPriority: 0 wins over higher numbers"`
	Values   map[string][]string `json:"values" doc:"Values of the CoS attributes"`
}

// NewCoS creates a definition below Parent.
type NewCoS struct {
	Parent            string   `json:"parent" minLength:"1"`
	Name              string   `json:"name" minLength:"1"`
	Kind              string   `json:"kind" enum:"pointer,indirect,classic"`
	Description       string   `json:"description,omitempty"`
	Attributes        []string `json:"attributes" minItems:"1"`
	TemplateDN        string   `json:"templateDN,omitempty"`
	Specifier         string   `json:"specifier,omitempty"`
	IndirectSpecifier string   `json:"indirectSpecifier,omitempty"`
}

// TemplateValues creates or replaces the values of a template.
type TemplateValues struct {
	Priority *int                `json:"priority,omitempty"`
	Values   map[string][]string `json:"values"`
}

var cosQualifiers = []string{"default", "override", "operational", "operational-default", "merge-schemes"}

var cosAttrs = []string{"cn", "objectClass", "description", "cosAttribute", "cosTemplateDN", "cosSpecifier", "cosIndirectSpecifier"}

// cosAttrName is the attribute name of a cosAttribute value ("postalCode default").
func cosAttrName(v string) string {
	f := strings.Fields(v)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

func cosFromEntry(e *ldap.Entry) CoSDefinition {
	d := CoSDefinition{DN: e.DN, Name: e.GetAttributeValue("cn"), Description: e.GetAttributeValue("description"),
		Attributes: e.GetAttributeValues("cosAttribute"), TemplateDN: e.GetAttributeValue("cosTemplateDN"),
		Specifier: e.GetAttributeValue("cosSpecifier"), IndirectSpecifier: e.GetAttributeValue("cosIndirectSpecifier"),
		Templates: []CoSTemplate{}}
	if d.Attributes == nil {
		d.Attributes = []string{}
	}
	switch {
	case hasClass(e, "cosPointerDefinition"):
		d.Kind = CoSPointer
	case hasClass(e, "cosClassicDefinition"):
		d.Kind = CoSClassic
	case hasClass(e, "cosIndirectDefinition"):
		d.Kind = CoSIndirect
	}
	return d
}

func templateFromEntry(e *ldap.Entry, attrs []string) CoSTemplate {
	t := CoSTemplate{DN: e.DN, Name: e.GetAttributeValue("cn"), Values: map[string][]string{}}
	if p, err := strconv.Atoi(e.GetAttributeValue("cosPriority")); err == nil {
		t.Priority = &p
	}
	for _, a := range attrs {
		if v := e.GetEqualFoldAttributeValues(a); len(v) > 0 {
			t.Values[a] = v
		}
	}
	return t
}

func (d *CoSDefinition) attrNames() []string {
	out := []string{}
	for _, a := range d.Attributes {
		if n := cosAttrName(a); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// readTemplates fills the templates of pointer and classic definitions.
func readTemplates(c *ldap.Conn, d *CoSDefinition) error {
	if d.TemplateDN == "" || d.Kind == CoSIndirect {
		return nil
	}
	scope := ldap.ScopeBaseObject
	if d.Kind == CoSClassic {
		scope = ldap.ScopeSingleLevel
	}
	attrs := append([]string{"cn", "cosPriority"}, d.attrNames()...)
	res, err := c.Search(ldap.NewSearchRequest(d.TemplateDN, scope, ldap.NeverDerefAliases, 0, 0, false,
		"(|(objectClass=costemplate)(&(objectClass=costemplate)(objectClass=ldapSubEntry)))", attrs, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range res.Entries {
		d.Templates = append(d.Templates, templateFromEntry(e, d.attrNames()))
	}
	slices.SortFunc(d.Templates, func(a, b CoSTemplate) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return nil
}

// ListCoS returns the CoS definitions of all suffixes with their templates.
func ListCoS(c *ldap.Conn) ([]CoSDefinition, error) {
	entries, err := searchSuffixes(c, "(&(objectClass=ldapSubEntry)(objectClass=cosSuperDefinition))", cosAttrs, 0)
	if err != nil {
		return nil, err
	}
	out := make([]CoSDefinition, 0, len(entries))
	for _, e := range entries {
		d := cosFromEntry(e)
		if d.Kind == "" {
			continue
		}
		if err := readTemplates(c, &d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b CoSDefinition) int { return strings.Compare(strings.ToLower(a.DN), strings.ToLower(b.DN)) })
	return out, nil
}

// GetCoS reads one definition with its templates.
func GetCoS(c *ldap.Conn, dn string) (*CoSDefinition, error) {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(|(objectClass=cosSuperDefinition)(objectClass=ldapSubEntry))", cosAttrs, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: CoS definition %s", ErrNotFound, dn)
	}
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 || !hasClass(res.Entries[0], "cosSuperDefinition") {
		return nil, fmt.Errorf("%w: CoS definition %s", ErrNotFound, dn)
	}
	d := cosFromEntry(res.Entries[0])
	if err := readTemplates(c, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func validCoSAttributes(list []string) ([]string, error) {
	var out []string
	for _, v := range list {
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		if !attrRE.MatchString(f[0]) {
			return nil, fmt.Errorf("%w: attribute %q", ErrInvalidValue, f[0])
		}
		for _, q := range f[1:] {
			if !slices.Contains(cosQualifiers, strings.ToLower(q)) {
				return nil, fmt.Errorf("%w: qualifier %q (one of %s)", ErrInvalidValue, q, strings.Join(cosQualifiers, ", "))
			}
		}
		out = append(out, strings.Join(f, " "))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one CoS attribute is needed", ErrInvalidValue)
	}
	return out, nil
}

// CreateCoS adds a CoS definition (templates are added separately).
func CreateCoS(c *ldap.Conn, n NewCoS) (*CoSDefinition, error) {
	n.Name = strings.TrimSpace(n.Name)
	if !itemName(n.Name) {
		return nil, fmt.Errorf("%w: name %q", ErrInvalidName, n.Name)
	}
	if ok, err := exists(c, n.Parent); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, n.Parent)
	}
	attrs, err := validCoSAttributes(n.Attributes)
	if err != nil {
		return nil, err
	}
	entry := map[string][]string{"cn": {n.Name}, "cosAttribute": attrs}
	if d := strings.TrimSpace(n.Description); d != "" {
		entry["description"] = []string{d}
	}
	base := []string{"top", "ldapSubEntry", "cosSuperDefinition"}
	needDN := func(v, what string) error {
		if _, err := ldap.ParseDN(v); err != nil || strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: %s %q is not a DN", ErrInvalidValue, what, v)
		}
		return nil
	}
	needAttr := func(v, what string) error {
		if !attrRE.MatchString(strings.TrimSpace(v)) {
			return fmt.Errorf("%w: %s %q", ErrInvalidValue, what, v)
		}
		return nil
	}
	switch n.Kind {
	case CoSPointer:
		if err := needDN(n.TemplateDN, "template"); err != nil {
			return nil, err
		}
		entry["objectClass"] = append(base, "cosPointerDefinition")
		entry["cosTemplateDN"] = []string{n.TemplateDN}
	case CoSClassic:
		if err := needDN(n.TemplateDN, "template container"); err != nil {
			return nil, err
		}
		if err := needAttr(n.Specifier, "specifier"); err != nil {
			return nil, err
		}
		entry["objectClass"] = append(base, "cosClassicDefinition")
		entry["cosTemplateDN"] = []string{n.TemplateDN}
		entry["cosSpecifier"] = []string{strings.TrimSpace(n.Specifier)}
	case CoSIndirect:
		if err := needAttr(n.IndirectSpecifier, "indirect specifier"); err != nil {
			return nil, err
		}
		entry["objectClass"] = append(base, "cosIndirectDefinition")
		entry["cosIndirectSpecifier"] = []string{strings.TrimSpace(n.IndirectSpecifier)}
	default:
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidValue, n.Kind)
	}
	dn := "cn=" + ldap.EscapeDN(n.Name) + "," + n.Parent
	if err := addEntry(c, dn, entry); err != nil {
		return nil, err
	}
	return GetCoS(c, dn)
}

// DeleteCoS removes a definition; its templates stay (they may be shared).
func DeleteCoS(c *ldap.Conn, dn string) error {
	d, err := GetCoS(c, dn)
	if err != nil {
		return err
	}
	return c.Del(ldap.NewDelRequest(d.DN, nil))
}

// SetTemplate creates the template entry dn or replaces its values. Empty
// value lists remove the attribute.
func SetTemplate(c *ldap.Conn, dn string, v TemplateValues) error {
	d, err := ldap.ParseDN(dn)
	if err != nil || len(d.RDNs) < 2 || len(d.RDNs[0].Attributes) != 1 {
		return fmt.Errorf("%w: template DN %q", ErrInvalidValue, dn)
	}
	for a := range v.Values {
		if !attrRE.MatchString(a) {
			return fmt.Errorf("%w: attribute %q", ErrInvalidValue, a)
		}
	}
	if v.Priority != nil && *v.Priority < 0 {
		return fmt.Errorf("%w: priority must not be negative", ErrInvalidValue)
	}
	ok, err := exists(c, dn)
	if err != nil {
		return err
	}
	if !ok {
		rdn := d.RDNs[0].Attributes[0]
		entry := map[string][]string{"objectClass": {"top", "extensibleObject", "cosTemplate", "ldapSubEntry"}, rdn.Type: {rdn.Value}}
		for a, vals := range v.Values {
			if vals = clean(vals); len(vals) > 0 {
				entry[a] = vals
			}
		}
		if v.Priority != nil {
			entry["cosPriority"] = []string{strconv.Itoa(*v.Priority)}
		}
		return addEntry(c, dn, entry)
	}
	req := ldap.NewModifyRequest(dn, nil)
	for a, vals := range v.Values {
		req.Replace(a, clean(vals))
	}
	if v.Priority != nil {
		req.Replace("cosPriority", []string{strconv.Itoa(*v.Priority)})
	}
	if len(req.Changes) == 0 {
		return nil
	}
	return c.Modify(req)
}

// DeleteTemplate removes a template entry.
func DeleteTemplate(c *ldap.Conn, dn string) error {
	err := c.Del(ldap.NewDelRequest(dn, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: template %s", ErrNotFound, dn)
	}
	return err
}

func clean(v []string) []string {
	out := []string{}
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
