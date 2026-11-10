package ds

import (
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// Role kinds.
const (
	RoleManaged  = "managed"  // members name the role in nsRoleDN
	RoleFiltered = "filtered" // members match nsRoleFilter
	RoleNested   = "nested"   // members of the roles in nsRoleDN
)

// Role is a role definition (an ldapSubEntry); its members get the role DN
// in the computed attribute nsRole.
type Role struct {
	DN          string   `json:"dn"`
	Name        string   `json:"name"`
	Kind        string   `json:"kind" enum:"managed,filtered,nested"`
	Description string   `json:"description,omitempty"`
	Filter      string   `json:"filter,omitempty" doc:"nsRoleFilter (filtered roles)"`
	Roles       []string `json:"roles" doc:"nsRoleDN: the roles a nested role contains"`
	Scope       string   `json:"scope,omitempty" doc:"nsRoleScopeDN: extends the role beyond its subtree"`
}

// NewRole creates a role below Parent.
type NewRole struct {
	Parent      string   `json:"parent" minLength:"1" doc:"Entry below which the role is defined; it applies to that subtree"`
	Name        string   `json:"name" minLength:"1"`
	Kind        string   `json:"kind" enum:"managed,filtered,nested"`
	Description string   `json:"description,omitempty"`
	Filter      string   `json:"filter,omitempty"`
	Roles       []string `json:"roles,omitempty"`
}

// RolePatch changes a role; nil fields stay.
type RolePatch struct {
	Description *string  `json:"description,omitempty"`
	Filter      *string  `json:"filter,omitempty"`
	Roles       []string `json:"roles,omitempty" doc:"Replaces the contained roles (nested roles)"`
}

var roleAttrs = []string{"cn", "objectClass", "description", "nsRoleFilter", "nsRoleDN", "nsRoleScopeDN"}

func hasClass(e *ldap.Entry, class string) bool {
	return slices.ContainsFunc(e.GetAttributeValues("objectClass"), func(c string) bool { return strings.EqualFold(c, class) })
}

func roleFromEntry(e *ldap.Entry) Role {
	r := Role{DN: e.DN, Name: e.GetAttributeValue("cn"), Description: e.GetAttributeValue("description"),
		Filter: e.GetAttributeValue("nsRoleFilter"), Scope: e.GetAttributeValue("nsRoleScopeDN"), Roles: []string{}}
	switch {
	case hasClass(e, "nsManagedRoleDefinition"):
		r.Kind = RoleManaged
	case hasClass(e, "nsFilteredRoleDefinition"):
		r.Kind = RoleFiltered
	case hasClass(e, "nsNestedRoleDefinition"):
		r.Kind = RoleNested
		r.Roles = append(r.Roles, e.GetAttributeValues("nsRoleDN")...)
	}
	return r
}

func namingContexts(c *ldap.Conn) ([]string, error) {
	res, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"namingContexts"}, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 {
		return nil, nil
	}
	return res.Entries[0].GetAttributeValues("namingContexts"), nil
}

// searchSuffixes runs a subtree search over every naming context.
func searchSuffixes(c *ldap.Conn, filter string, attrs []string, limit int) ([]*ldap.Entry, error) {
	bases, err := namingContexts(c)
	if err != nil {
		return nil, err
	}
	var out []*ldap.Entry
	for _, base := range bases {
		r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			limit, 15, false, filter, attrs, nil))
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			continue
		}
		if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			return nil, err
		}
		if r != nil {
			out = append(out, r.Entries...)
		}
	}
	return out, nil
}

// ListRoles returns the role definitions of all suffixes.
func ListRoles(c *ldap.Conn) ([]Role, error) {
	entries, err := searchSuffixes(c, "(&(objectClass=ldapSubEntry)(objectClass=nsRoleDefinition))", roleAttrs, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Role, 0, len(entries))
	for _, e := range entries {
		if r := roleFromEntry(e); r.Kind != "" {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b Role) int { return strings.Compare(strings.ToLower(a.DN), strings.ToLower(b.DN)) })
	return out, nil
}

// GetRole reads one role definition.
func GetRole(c *ldap.Conn, dn string) (*Role, error) {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(|(objectClass=nsRoleDefinition)(objectClass=ldapSubEntry))", roleAttrs, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: role %s", ErrNotFound, dn)
	}
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 || !hasClass(res.Entries[0], "nsRoleDefinition") {
		return nil, fmt.Errorf("%w: role %s", ErrNotFound, dn)
	}
	r := roleFromEntry(res.Entries[0])
	return &r, nil
}

func checkRoleDNs(c *ldap.Conn, dns []string) ([]string, error) {
	var out []string
	for _, d := range dns {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if _, err := GetRole(c, d); err != nil {
			return nil, fmt.Errorf("%w: %s is not a role", ErrInvalidValue, d)
		}
		out = append(out, d)
	}
	return out, nil
}

// CreateRole adds a role definition.
func CreateRole(c *ldap.Conn, n NewRole) (*Role, error) {
	n.Name = strings.TrimSpace(n.Name)
	if !itemName(n.Name) {
		return nil, fmt.Errorf("%w: role name %q", ErrInvalidName, n.Name)
	}
	if ok, err := exists(c, n.Parent); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, n.Parent)
	}
	attrs := map[string][]string{"cn": {n.Name}}
	if d := strings.TrimSpace(n.Description); d != "" {
		attrs["description"] = []string{d}
	}
	base := []string{"top", "ldapSubEntry", "nsRoleDefinition"}
	switch n.Kind {
	case RoleManaged:
		attrs["objectClass"] = append(base, "nsSimpleRoleDefinition", "nsManagedRoleDefinition")
	case RoleFiltered:
		if _, err := ldap.CompileFilter(n.Filter); err != nil || strings.TrimSpace(n.Filter) == "" {
			return nil, fmt.Errorf("%w: filter %q", ErrInvalidValue, n.Filter)
		}
		attrs["objectClass"] = append(base, "nsComplexRoleDefinition", "nsFilteredRoleDefinition")
		attrs["nsRoleFilter"] = []string{n.Filter}
	case RoleNested:
		roles, err := checkRoleDNs(c, n.Roles)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			return nil, fmt.Errorf("%w: a nested role needs at least one role", ErrInvalidValue)
		}
		attrs["objectClass"] = append(base, "nsComplexRoleDefinition", "nsNestedRoleDefinition")
		attrs["nsRoleDN"] = roles
	default:
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidValue, n.Kind)
	}
	dn := "cn=" + ldap.EscapeDN(n.Name) + "," + n.Parent
	if err := addEntry(c, dn, attrs); err != nil {
		return nil, err
	}
	return GetRole(c, dn)
}

// UpdateRole changes the description, filter or contained roles.
func UpdateRole(c *ldap.Conn, dn string, p RolePatch) (*Role, error) {
	r, err := GetRole(c, dn)
	if err != nil {
		return nil, err
	}
	req := ldap.NewModifyRequest(r.DN, nil)
	if p.Description != nil {
		req.Replace("description", nonEmpty(*p.Description))
	}
	if p.Filter != nil {
		if r.Kind != RoleFiltered {
			return nil, fmt.Errorf("%w: only filtered roles have a filter", ErrInvalidValue)
		}
		if _, err := ldap.CompileFilter(*p.Filter); err != nil {
			return nil, fmt.Errorf("%w: filter: %v", ErrInvalidValue, err)
		}
		req.Replace("nsRoleFilter", []string{*p.Filter})
	}
	if p.Roles != nil {
		if r.Kind != RoleNested {
			return nil, fmt.Errorf("%w: only nested roles contain roles", ErrInvalidValue)
		}
		roles, err := checkRoleDNs(c, p.Roles)
		if err != nil {
			return nil, err
		}
		if len(roles) == 0 {
			return nil, fmt.Errorf("%w: a nested role needs at least one role", ErrInvalidValue)
		}
		req.Replace("nsRoleDN", roles)
	}
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	return GetRole(c, r.DN)
}

func nonEmpty(s string) []string {
	if s = strings.TrimSpace(s); s == "" {
		return []string{}
	}
	return []string{s}
}

// DeleteRole removes a role definition. Entries keep nsRoleDN values that
// name it (managed roles); the server ignores them.
func DeleteRole(c *ldap.Conn, dn string) error {
	r, err := GetRole(c, dn)
	if err != nil {
		return err
	}
	return c.Del(ldap.NewDelRequest(r.DN, nil))
}

// RoleMembers lists entries that have the role (nsRole), up to limit.
func RoleMembers(c *ldap.Conn, dn string, limit int) ([]string, bool, error) {
	r, err := GetRole(c, dn)
	if err != nil {
		return nil, false, err
	}
	entries, err := searchSuffixes(c, "(nsRole="+ldap.EscapeFilter(r.DN)+")", []string{"1.1"}, limit+1)
	if err != nil {
		return nil, false, err
	}
	out := []string{}
	for _, e := range entries {
		out = append(out, e.DN)
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	truncated := len(out) > limit
	if truncated {
		out = out[:limit]
	}
	return out, truncated, nil
}

// SetManagedMember adds or removes an entry from a managed role.
func SetManagedMember(c *ldap.Conn, role, member string, add bool) error {
	r, err := GetRole(c, role)
	if err != nil {
		return err
	}
	if r.Kind != RoleManaged {
		return fmt.Errorf("%w: members are set only for managed roles", ErrInvalidValue)
	}
	req := ldap.NewModifyRequest(member, nil)
	if add {
		req.Add("nsRoleDN", []string{r.DN})
	} else {
		req.Delete("nsRoleDN", []string{r.DN})
	}
	err = c.Modify(req)
	switch {
	case ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject):
		return fmt.Errorf("%w: entry %s", ErrNotFound, member)
	case ldap.IsErrorWithCode(err, ldap.LDAPResultAttributeOrValueExists), ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchAttribute):
		return nil
	}
	return err
}

func itemName(s string) bool {
	return s != "" && !strings.ContainsAny(s, ",=+\"\\<>;") && !strings.HasPrefix(s, "#")
}
