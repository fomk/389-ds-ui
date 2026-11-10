package ds

import (
	"fmt"
	"slices"
	"strings"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
)

// EntryACIs are the access control instructions stored in one entry.
type EntryACIs struct {
	DN   string   `json:"dn"`
	ACIs []string `json:"acis"`
}

// ListACIs returns the entries under base (subtree) or base alone that hold
// ACIs.
func ListACIs(c *ldap.Conn, base string, subtree bool) ([]EntryACIs, error) {
	scope := ldap.ScopeBaseObject
	if subtree {
		scope = ldap.ScopeWholeSubtree
	}
	res, err := c.Search(ldap.NewSearchRequest(base, scope, ldap.NeverDerefAliases, 0, 0, false,
		"(aci=*)", []string{"aci"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, base)
	}
	if err != nil {
		return nil, err
	}
	out := make([]EntryACIs, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, EntryACIs{DN: e.DN, ACIs: e.GetAttributeValues("aci")})
	}
	// Parents before children: sort by depth, then DN.
	depth := func(dn string) int {
		d, err := ldap.ParseDN(dn)
		if err != nil {
			return 0
		}
		return len(d.RDNs)
	}
	slices.SortStableFunc(out, func(a, b EntryACIs) int {
		if d := depth(a.DN) - depth(b.DN); d != 0 {
			return d
		}
		return strings.Compare(strings.ToLower(a.DN), strings.ToLower(b.DN))
	})
	return out, nil
}

// InheritedACIs returns the ACIs of the ancestors of dn that exist, nearest
// first: they apply to dn too unless their targets exclude it.
func InheritedACIs(c *ldap.Conn, dn string) ([]EntryACIs, error) {
	out := []EntryACIs{}
	cur := dn
	for {
		parent, err := parentDN(cur)
		if err != nil {
			return out, nil
		}
		list, err := ListACIs(c, parent, false)
		if err == nil && len(list) > 0 {
			out = append(out, list[0])
		}
		cur = parent
	}
}

// SetACIs replaces the ACIs of an entry; an empty list removes them. The
// server checks the syntax and rejects invalid ACIs.
func SetACIs(c *ldap.Conn, dn string, acis []string) error {
	var clean []string
	for _, a := range acis {
		if a = strings.TrimSpace(a); a != "" {
			clean = append(clean, a)
		}
	}
	req := ldap.NewModifyRequest(dn, nil)
	req.Replace("aci", clean)
	err := c.Modify(req)
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: entry %s", ErrNotFound, dn)
	}
	return err
}

// --- Get Effective Rights ----------------------------------------------------

// ControlGetEffectiveRights asks the server to report the rights a subject
// has on the returned entries (entryLevelRights, attributeLevelRights).
const ControlGetEffectiveRights = "1.3.6.1.4.1.42.2.27.9.5.2"

type gerControl struct{ authzID string }

func (g gerControl) GetControlType() string { return ControlGetEffectiveRights }
func (g gerControl) String() string         { return "GetEffectiveRights(" + g.authzID + ")" }

// Encode: 389-ds reads the control value as a single OCTET STRING holding
// the authzId ("dn: <subject>"), not the SEQUENCE { authzId, attributes } of
// the draft (a SEQUENCE is refused with "subject is not dnAuthzId"); the
// attributes are requested in the search itself.
func (g gerControl) Encode() *ber.Packet {
	p := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "Control")
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, ControlGetEffectiveRights, "Control Type"))
	p.AppendChild(ber.NewBoolean(ber.ClassUniversal, ber.TypePrimitive, ber.TagBoolean, true, "Criticality"))
	value := ber.Encode(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, nil, "Control Value")
	value.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, g.authzID, "authzId"))
	p.AppendChild(value)
	return p
}

// EffectiveRights are the rights of a subject on an entry.
type EffectiveRights struct {
	Target  string            `json:"target"`
	Subject string            `json:"subject" doc:"Empty: anonymous"`
	Entry   string            `json:"entry" doc:"entryLevelRights: v view, a add, d delete, n rename"`
	Attrs   map[string]string `json:"attributes" doc:"attributeLevelRights by attribute: r read, s search, c compare, w write, o obliterate (delete value), W/S self write"`
}

// GetEffectiveRights evaluates the ACIs for subject (a DN, or "" for
// anonymous) on target. attrs adds attributes the entry does not have yet
// ("*" covers the present ones).
func GetEffectiveRights(c *ldap.Conn, target, subject string, attrs []string) (*EffectiveRights, error) {
	if subject != "" {
		if _, err := ldap.ParseDN(subject); err != nil {
			return nil, fmt.Errorf("%w: subject %q is not a DN", ErrInvalidValue, subject)
		}
	}
	req := []string{"*", "entryLevelRights", "attributeLevelRights"}
	req = append(req, attrs...)
	res, err := c.Search(ldap.NewSearchRequest(target, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(|(objectClass=*)(objectClass=ldapSubEntry))", req, []ldap.Control{gerControl{authzID: "dn: " + subject}}))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, target)
	}
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, target)
	}
	e := res.Entries[0]
	// 389-ds writes "none" for no rights; its letters would read as rights.
	none := func(r string) string {
		if strings.EqualFold(r, "none") {
			return ""
		}
		return r
	}
	out := &EffectiveRights{Target: e.DN, Subject: subject, Entry: none(e.GetAttributeValue("entryLevelRights")), Attrs: map[string]string{}}
	// "cn:rscwo, sn:rscwo, uid:none, ..."
	for _, part := range strings.Split(e.GetAttributeValue("attributeLevelRights"), ",") {
		if name, rights, ok := strings.Cut(strings.TrimSpace(part), ":"); ok {
			out.Attrs[name] = none(rights)
		}
	}
	return out, nil
}
