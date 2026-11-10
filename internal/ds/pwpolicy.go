package ds

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
)

// Local (fine-grained) password policies, as 389-ds lays them out:
//
//	cn=nsPwPolicyContainer,<base>                        nsContainer
//	cn="cn=nsPwPolicyEntry_subtree,<subtree>",<container> passwordPolicy, ldapSubEntry
//	cn="cn=nsPwTemplateEntry,<subtree>",<container>       CoS template: pwdpolicysubentry
//	cn=nsPwPolicy_CoS,<subtree>                           CoS pointer definition
//
// A user policy is cn="cn=nsPwPolicyEntry_user,<user>" in the container of
// the user's parent, and the user entry names it in pwdpolicysubentry.

// Policy kinds.
const (
	PolicySubtree = "subtree"
	PolicyUser    = "user"
)

// ErrExists is returned when the object to create is already there.
var ErrExists = errors.New("already exists")

// LocalPolicy is a subtree or user password policy.
type LocalPolicy struct {
	DN     string `json:"dn" doc:"The passwordPolicy entry (edit it with GET/PATCH /pages/password-policy-local?dn=)"`
	Kind   string `json:"kind" enum:"subtree,user"`
	Target string `json:"target" doc:"The subtree or user the policy applies to"`
}

// NewLocalPolicy creates a policy.
type NewLocalPolicy struct {
	Kind   string `json:"kind" enum:"subtree,user"`
	Target string `json:"target" minLength:"1"`
}

// LocalPolicyCreated reports a new policy.
type LocalPolicyCreated struct {
	Policy LocalPolicy `json:"policy"`
	// The server ignores local policies unless nsslapd-pwpolicy-local is on.
	EnabledLocalPolicies bool `json:"enabledLocalPolicies" doc:"nsslapd-pwpolicy-local was off and has been turned on"`
}

const policyContainerRDN = "cn=nsPwPolicyContainer"

func policyContainer(base string) string { return policyContainerRDN + "," + base }

func policyName(kind, target string) string {
	return "cn=nsPwPolicyEntry_" + kind + "," + target
}

func policyDN(kind, target, base string) string {
	return "cn=" + ldap.EscapeDN(policyName(kind, target)) + "," + policyContainer(base)
}

func templateDN(subtree string) string {
	return "cn=" + ldap.EscapeDN("cn=nsPwTemplateEntry,"+subtree) + "," + policyContainer(subtree)
}

func cosDefDN(subtree string) string { return "cn=nsPwPolicy_CoS," + subtree }

// parentDN returns the DN without its first RDN.
func parentDN(dn string) (string, error) {
	d, err := ldap.ParseDN(dn)
	if err != nil || len(d.RDNs) < 2 {
		return "", fmt.Errorf("%w: %q has no parent", ErrInvalidValue, dn)
	}
	parts := make([]string, 0, len(d.RDNs)-1)
	for _, r := range d.RDNs[1:] {
		parts = append(parts, r.String())
	}
	return strings.Join(parts, ","), nil
}

// ListLocalPolicies returns the subtree and user policies of all suffixes.
func ListLocalPolicies(c *ldap.Conn) ([]LocalPolicy, error) {
	res, err := c.Search(ldap.NewSearchRequest("", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"namingContexts"}, nil))
	if err != nil {
		return nil, err
	}
	out := []LocalPolicy{}
	if len(res.Entries) != 1 {
		return out, nil
	}
	for _, base := range res.Entries[0].GetAttributeValues("namingContexts") {
		// Subentries are returned only when the filter asks for them.
		r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
			0, 0, false, "(&(objectClass=ldapSubEntry)(objectClass=passwordPolicy))", []string{"cn"}, nil))
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range r.Entries {
			p := LocalPolicy{DN: e.DN}
			for _, cn := range e.GetAttributeValues("cn") {
				for _, kind := range []string{PolicySubtree, PolicyUser} {
					if prefix := "cn=nspwpolicyentry_" + kind + ","; strings.HasPrefix(strings.ToLower(cn), prefix) {
						p.Kind, p.Target = kind, cn[len(prefix):]
					}
				}
			}
			if p.Kind != "" {
				out = append(out, p)
			}
		}
	}
	slices.SortFunc(out, func(a, b LocalPolicy) int {
		return strings.Compare(strings.ToLower(a.Target), strings.ToLower(b.Target))
	})
	return out, nil
}

func exists(c *ldap.Conn, dn string) (bool, error) {
	_, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(|(objectClass=*)(objectClass=ldapSubEntry))", []string{"1.1"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return false, nil
	}
	return err == nil, err
}

func addEntry(c *ldap.Conn, dn string, attrs map[string][]string) error {
	req := ldap.NewAddRequest(dn, nil)
	for k, v := range attrs {
		req.Attribute(k, v)
	}
	return c.Add(req)
}

// CreateLocalPolicy creates an (empty) subtree or user policy and turns on
// local policies in cn=config if they are off.
func CreateLocalPolicy(c *ldap.Conn, n NewLocalPolicy) (*LocalPolicyCreated, error) {
	target := strings.TrimSpace(n.Target)
	if _, err := ldap.ParseDN(target); err != nil || target == "" {
		return nil, fmt.Errorf("%w: target %q is not a DN", ErrInvalidValue, n.Target)
	}
	if n.Kind != PolicySubtree && n.Kind != PolicyUser {
		return nil, fmt.Errorf("%w: kind %q", ErrInvalidValue, n.Kind)
	}
	if ok, err := exists(c, target); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, target)
	}
	base := target
	if n.Kind == PolicyUser {
		var err error
		if base, err = parentDN(target); err != nil {
			return nil, err
		}
	}
	pdn := policyDN(n.Kind, target, base)
	if ok, err := exists(c, pdn); err != nil {
		return nil, err
	} else if ok {
		return nil, fmt.Errorf("%w: a %s policy for %s", ErrExists, n.Kind, target)
	}

	if ok, err := exists(c, policyContainer(base)); err != nil {
		return nil, err
	} else if !ok {
		if err := addEntry(c, policyContainer(base), map[string][]string{
			"objectClass": {"top", "nsContainer"}, "cn": {"nsPwPolicyContainer"},
		}); err != nil {
			return nil, err
		}
	}
	if err := addEntry(c, pdn, map[string][]string{
		"objectClass": {"top", "ldapSubEntry", "passwordPolicy"}, "cn": {policyName(n.Kind, target)},
	}); err != nil {
		return nil, err
	}

	switch n.Kind {
	case PolicySubtree:
		if err := addEntry(c, templateDN(target), map[string][]string{
			"objectClass":       {"top", "extensibleObject", "costemplate", "ldapSubEntry"},
			"cn":                {"cn=nsPwTemplateEntry," + target},
			"cosPriority":       {"1"},
			"pwdpolicysubentry": {pdn},
		}); err != nil {
			_ = c.Del(ldap.NewDelRequest(pdn, nil))
			return nil, err
		}
		if err := addEntry(c, cosDefDN(target), map[string][]string{
			"objectClass":   {"top", "ldapSubEntry", "cosSuperDefinition", "cosPointerDefinition"},
			"cn":            {"nsPwPolicy_CoS"},
			"cosTemplateDn": {templateDN(target)},
			"cosAttribute":  {"pwdpolicysubentry default operational-default"},
		}); err != nil {
			_ = c.Del(ldap.NewDelRequest(templateDN(target), nil))
			_ = c.Del(ldap.NewDelRequest(pdn, nil))
			return nil, err
		}
	case PolicyUser:
		req := ldap.NewModifyRequest(target, nil)
		req.Replace("pwdpolicysubentry", []string{pdn})
		if err := c.Modify(req); err != nil {
			_ = c.Del(ldap.NewDelRequest(pdn, nil))
			return nil, err
		}
	}

	out := &LocalPolicyCreated{Policy: LocalPolicy{DN: pdn, Kind: n.Kind, Target: target}}
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"nsslapd-pwpolicy-local"}, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 1 && !strings.EqualFold(res.Entries[0].GetAttributeValue("nsslapd-pwpolicy-local"), "on") {
		req := ldap.NewModifyRequest("cn=config", nil)
		req.Replace("nsslapd-pwpolicy-local", []string{"on"})
		if err := c.Modify(req); err != nil {
			return nil, err
		}
		out.EnabledLocalPolicies = true
	}
	return out, nil
}

// DeleteLocalPolicy removes a policy with its CoS entries (subtree) or the
// pointer in the user entry, and the container once it is empty.
func DeleteLocalPolicy(c *ldap.Conn, dn string) error {
	list, err := ListLocalPolicies(c)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(p LocalPolicy) bool { return strings.EqualFold(ldapx.NormDN(p.DN), ldapx.NormDN(dn)) })
	if i < 0 {
		return fmt.Errorf("%w: password policy %s", ErrNotFound, dn)
	}
	p := list[i]
	ignoreMissing := func(err error) error {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchAttribute) {
			return nil
		}
		return err
	}
	switch p.Kind {
	case PolicySubtree:
		for _, d := range []string{cosDefDN(p.Target), templateDN(p.Target)} {
			if err := ignoreMissing(c.Del(ldap.NewDelRequest(d, nil))); err != nil {
				return err
			}
		}
	case PolicyUser:
		req := ldap.NewModifyRequest(p.Target, nil)
		req.Delete("pwdpolicysubentry", nil)
		if err := ignoreMissing(c.Modify(req)); err != nil {
			return err
		}
	}
	if err := c.Del(ldap.NewDelRequest(p.DN, nil)); err != nil {
		return err
	}
	// Remove the container when nothing is left in it.
	container, err := parentDN(p.DN)
	if err != nil {
		return nil
	}
	res, err := c.Search(ldap.NewSearchRequest(container, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
		1, 0, false, "(|(objectClass=*)(objectClass=ldapSubEntry))", []string{"1.1"}, nil))
	if err == nil && len(res.Entries) == 0 {
		_ = c.Del(ldap.NewDelRequest(container, nil))
	}
	return nil
}
