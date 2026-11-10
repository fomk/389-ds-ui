package ds

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

var (
	// ErrSchemaInUse is returned when a definition to delete is still used.
	ErrSchemaInUse = errors.New("schema definition is in use")
	// ErrSchemaNotUser protects definitions that ship with the server.
	ErrSchemaNotUser = errors.New("only user-defined schema can be changed")
	// ErrSchemaExists is returned when a name or OID is taken.
	ErrSchemaExists = errors.New("schema name or OID already exists")
)

const schemaDN = "cn=schema"

// rawDefinition finds the stored value of a definition by OID, which is
// what a delete must name exactly.
func rawDefinition(c *ldap.Conn, attr, oid string) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest(schemaDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{attr}, nil))
	if err != nil {
		return "", err
	}
	if len(res.Entries) == 1 {
		for _, v := range res.Entries[0].GetAttributeValues(attr) {
			if strings.EqualFold(schema.ParsedOID(v), oid) {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNotFound, oid)
}

// namesTaken reports a name or OID of def that another definition uses.
func attrTaken(s *schema.Schema, oid string, names []string, self *schema.AttributeType) string {
	for _, n := range append([]string{oid}, names...) {
		if a := s.Attr(n); a != nil && a != self {
			return n
		}
	}
	return ""
}

func classTaken(s *schema.Schema, oid string, names []string, self *schema.ObjectClass) string {
	for _, n := range append([]string{oid}, names...) {
		if o := s.Class(n); o != nil && o != self {
			return n
		}
	}
	return ""
}

// AddAttributeType adds a user-defined attribute type.
func AddAttributeType(c *ldap.Conn, s *schema.Schema, d schema.AttributeTypeDef) error {
	if err := d.Normalize(s); err != nil {
		return err
	}
	if n := attrTaken(s, d.OID, d.Names, nil); n != "" {
		return fmt.Errorf("%w: %s", ErrSchemaExists, n)
	}
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Add("attributeTypes", []string{d.String()})
	return c.Modify(req)
}

// ReplaceAttributeType changes a user-defined attribute type (same OID).
func ReplaceAttributeType(c *ldap.Conn, s *schema.Schema, name string, d schema.AttributeTypeDef) error {
	cur := s.Attr(name)
	if cur == nil {
		return fmt.Errorf("%w: attribute type %s", ErrNotFound, name)
	}
	if !schema.UserDefined(cur.Origin) {
		return fmt.Errorf("%w: %s", ErrSchemaNotUser, name)
	}
	d.OID = cur.OID
	if err := d.Normalize(s); err != nil {
		return err
	}
	if n := attrTaken(s, d.OID, d.Names, cur); n != "" {
		return fmt.Errorf("%w: %s", ErrSchemaExists, n)
	}
	// 389-ds replaces a definition when one with the same OID is added;
	// delete+add is refused while a class references the type, and
	// "replace" is not allowed on the subschema entry.
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Add("attributeTypes", []string{d.String()})
	return c.Modify(req)
}

// DeleteAttributeType removes a user-defined attribute type that no class,
// other attribute type or entry uses.
func DeleteAttributeType(c *ldap.Conn, s *schema.Schema, name string) error {
	cur := s.Attr(name)
	if cur == nil {
		return fmt.Errorf("%w: attribute type %s", ErrNotFound, name)
	}
	if !schema.UserDefined(cur.Origin) {
		return fmt.Errorf("%w: %s", ErrSchemaNotUser, name)
	}
	if users := attrUsers(s, cur); len(users) > 0 {
		return fmt.Errorf("%w: %s is used by %s", ErrSchemaInUse, cur.Name(), strings.Join(users, ", "))
	}
	if err := unusedInEntries(c, "("+ldap.EscapeFilter(cur.Name())+"=*)"); err != nil {
		return fmt.Errorf("%w (%s)", err, cur.Name())
	}
	old, err := rawDefinition(c, "attributeTypes", cur.OID)
	if err != nil {
		return err
	}
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Delete("attributeTypes", []string{old})
	return c.Modify(req)
}

// attrUsers lists the classes and attribute types that reference a.
func attrUsers(s *schema.Schema, a *schema.AttributeType) []string {
	var out []string
	is := func(n string) bool { return s.Attr(n) == a }
	for _, o := range s.ObjectClasses {
		if slices.ContainsFunc(o.Must, is) || slices.ContainsFunc(o.May, is) {
			out = append(out, o.Name())
		}
	}
	for _, x := range s.AttributeTypes {
		if x != a && x.Sup != "" && is(x.Sup) {
			out = append(out, x.Name())
		}
	}
	return out
}

// AddObjectClass adds a user-defined object class.
func AddObjectClass(c *ldap.Conn, s *schema.Schema, d schema.ObjectClassDef) error {
	if err := d.Normalize(s); err != nil {
		return err
	}
	if n := classTaken(s, d.OID, d.Names, nil); n != "" {
		return fmt.Errorf("%w: %s", ErrSchemaExists, n)
	}
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Add("objectClasses", []string{d.String()})
	return c.Modify(req)
}

// ReplaceObjectClass changes a user-defined object class (same OID).
func ReplaceObjectClass(c *ldap.Conn, s *schema.Schema, name string, d schema.ObjectClassDef) error {
	cur := s.Class(name)
	if cur == nil {
		return fmt.Errorf("%w: object class %s", ErrNotFound, name)
	}
	if !schema.UserDefined(cur.Origin) {
		return fmt.Errorf("%w: %s", ErrSchemaNotUser, name)
	}
	d.OID = cur.OID
	if err := d.Normalize(s); err != nil {
		return err
	}
	if n := classTaken(s, d.OID, d.Names, cur); n != "" {
		return fmt.Errorf("%w: %s", ErrSchemaExists, n)
	}
	// 389-ds replaces a definition when one with the same OID is added;
	// delete+add is refused while a class references the type, and
	// "replace" is not allowed on the subschema entry.
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Add("objectClasses", []string{d.String()})
	return c.Modify(req)
}

// DeleteObjectClass removes a user-defined object class that no other class
// inherits from and no entry uses.
func DeleteObjectClass(c *ldap.Conn, s *schema.Schema, name string) error {
	cur := s.Class(name)
	if cur == nil {
		return fmt.Errorf("%w: object class %s", ErrNotFound, name)
	}
	if !schema.UserDefined(cur.Origin) {
		return fmt.Errorf("%w: %s", ErrSchemaNotUser, name)
	}
	var subs []string
	for _, o := range s.ObjectClasses {
		if slices.ContainsFunc(o.Sup, func(n string) bool { return s.Class(n) == cur }) {
			subs = append(subs, o.Name())
		}
	}
	if len(subs) > 0 {
		return fmt.Errorf("%w: %s is the superior of %s", ErrSchemaInUse, cur.Name(), strings.Join(subs, ", "))
	}
	if err := unusedInEntries(c, "(objectClass="+ldap.EscapeFilter(cur.Name())+")"); err != nil {
		return fmt.Errorf("%w (%s)", err, cur.Name())
	}
	old, err := rawDefinition(c, "objectClasses", cur.OID)
	if err != nil {
		return err
	}
	req := ldap.NewModifyRequest(schemaDN, nil)
	req.Delete("objectClasses", []string{old})
	return c.Modify(req)
}

// unusedInEntries searches every naming context for one entry matching
// filter. An unindexed search over a large directory may hit the time limit;
// then the definition counts as possibly in use.
func unusedInEntries(c *ldap.Conn, filter string) error {
	res, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"namingContexts"}, nil))
	if err != nil {
		return err
	}
	if len(res.Entries) != 1 {
		return nil
	}
	for _, base := range res.Entries[0].GetAttributeValues("namingContexts") {
		r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			1, 15, false, filter, []string{"1.1"}, nil))
		switch {
		case ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) || err == nil && len(r.Entries) > 0:
			dn := base
			if r != nil && len(r.Entries) > 0 {
				dn = r.Entries[0].DN
			}
			return fmt.Errorf("%w: entries use it, e.g. %s", ErrSchemaInUse, dn)
		case ldap.IsErrorWithCode(err, ldap.LDAPResultTimeLimitExceeded):
			return fmt.Errorf("%w: could not check all entries of %s in time", ErrSchemaInUse, base)
		case ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject):
			// suffix without a root entry
		case err != nil:
			return err
		}
	}
	return nil
}
