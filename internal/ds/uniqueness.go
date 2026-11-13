package ds

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// UniquenessPath is the library of the Attribute Uniqueness plug-in; every
// instance (one per set of rules) is a plug-in entry of its own.
const UniquenessPath = "libattr-unique-plugin"

// defaultUniqueness is the instance the server ships; it stays.
const defaultUniqueness = "attribute uniqueness"

var pluginNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)

// NewUniqueness is a request for another Attribute Uniqueness instance.
type NewUniqueness struct {
	Name       string   `json:"name" minLength:"1" doc:"Plug-in cn, e.g. mail uniqueness"`
	Attributes []string `json:"attributes" minItems:"1" doc:"Attributes whose values must be unique"`
	Subtrees   []string `json:"subtrees,omitempty" doc:"Where values must be unique; empty: the suffixes of the server"`
}

// CreateUniqueness adds an Attribute Uniqueness instance, disabled: enabling
// it (and, without dynamic plug-ins, a restart) makes it effective.
func CreateUniqueness(c *ldap.Conn, nu NewUniqueness) error {
	if !pluginNameRE.MatchString(nu.Name) {
		return fmt.Errorf("%w: plug-in name %q (letters, digits, spaces, . _ -)", ErrInvalidName, nu.Name)
	}
	for _, a := range nu.Attributes {
		if !attrRE.MatchString(a) {
			return fmt.Errorf("%w: attribute %q", ErrInvalidName, a)
		}
	}
	for _, s := range nu.Subtrees {
		if _, err := ldap.ParseDN(s); err != nil || s == "" {
			return fmt.Errorf("%w: subtree %q", ErrInvalidValue, s)
		}
	}
	req := ldap.NewAddRequest("cn="+ldap.EscapeDN(nu.Name)+","+DNPlugins, nil)
	req.Attribute("objectClass", []string{"top", "nsSlapdPlugin", "extensibleObject"})
	req.Attribute("cn", []string{nu.Name})
	req.Attribute("nsslapd-pluginPath", []string{UniquenessPath})
	req.Attribute("nsslapd-pluginInitfunc", []string{"NSUniqueAttr_Init"})
	req.Attribute("nsslapd-pluginType", []string{"betxnpreoperation"})
	req.Attribute("nsslapd-pluginEnabled", []string{"off"})
	req.Attribute("nsslapd-plugin-depends-on-type", []string{"database"})
	req.Attribute("nsslapd-pluginId", []string{"NSUniqueAttr"})
	req.Attribute("nsslapd-pluginVersion", []string{"none"})
	req.Attribute("nsslapd-pluginVendor", []string{"none"})
	req.Attribute("nsslapd-pluginDescription", []string{"Enforce unique attribute values"})
	req.Attribute("uniqueness-attribute-name", nu.Attributes)
	if len(nu.Subtrees) > 0 {
		req.Attribute("uniqueness-subtrees", nu.Subtrees)
	}
	if err := c.Add(req); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
			return fmt.Errorf("%w: plug-in %s", ErrExists, nu.Name)
		}
		return err
	}
	return nil
}

// DeleteUniqueness removes an Attribute Uniqueness instance added earlier.
// The shipped instance stays; an enabled one must be disabled first, since
// without dynamic plug-ins the server keeps running it until a restart.
func DeleteUniqueness(c *ldap.Conn, name string) error {
	p, err := GetPlugin(c, name)
	if err != nil {
		return err
	}
	if !strings.EqualFold(p.Path, UniquenessPath) {
		return fmt.Errorf("%w: %s is not an Attribute Uniqueness instance", ErrInvalidValue, p.Name)
	}
	if strings.EqualFold(p.Name, defaultUniqueness) {
		return fmt.Errorf("%w: %s is the server's own instance", ErrProtectedPlugin, p.Name)
	}
	if p.Enabled {
		return fmt.Errorf("%w: disable %s first", ErrInvalidValue, p.Name)
	}
	return c.Del(ldap.NewDelRequest(p.DN, nil))
}
