package ds

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// ErrInvalidValue is returned for a value the server would reject anyway, so
// the user gets a clear message instead of an LDAP error.
var ErrInvalidValue = errors.New("invalid value")

// RootDN returns nsslapd-rootdn (Directory Manager).
func RootDN(c *ldap.Conn) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"nsslapd-rootdn"}, nil))
	if err != nil {
		return "", err
	}
	if len(res.Entries) != 1 {
		return "", fmt.Errorf("%w: cn=config", ErrNotFound)
	}
	return res.Entries[0].GetAttributeValue("nsslapd-rootdn"), nil
}

// MinRootPasswordLength is the shortest root DN password accepted (389-ds
// itself requires 8 characters).
const MinRootPasswordLength = 8

// SetRootPassword changes the Directory Manager password. The server hashes
// the clear text with nsslapd-rootpwstoragescheme.
func SetRootPassword(c *ldap.Conn, password string) error {
	if len(password) < MinRootPasswordLength {
		return fmt.Errorf("%w: the password must have at least %d characters", ErrInvalidValue, MinRootPasswordLength)
	}
	if strings.HasPrefix(password, "{") {
		// "{SCHEME}..." would be stored as a pre-hashed value.
		return fmt.Errorf("%w: the password must not start with {", ErrInvalidValue)
	}
	req := ldap.NewModifyRequest("cn=config", nil)
	req.Replace("nsslapd-rootpw", []string{password})
	return c.Modify(req)
}

// DNSASLMappings holds the SASL identity mappings.
const DNSASLMappings = "cn=mapping,cn=sasl,cn=config"

// SASLMapping maps a SASL identity to an entry: the regular expression's
// groups (\1, \2) are substituted into the base DN and filter templates.
type SASLMapping struct {
	Name     string `json:"name" doc:"cn"`
	Regex    string `json:"regex" doc:"nsSaslMapRegexString"`
	BaseDN   string `json:"baseDN" doc:"nsSaslMapBaseDNTemplate"`
	Filter   string `json:"filter" doc:"nsSaslMapFilterTemplate"`
	Priority int    `json:"priority,omitempty" doc:"nsSaslMapPriority: 1 (first) … 100; 0 means unset (100)" minimum:"0" maximum:"100"`
}

func saslMappingDN(name string) string {
	return "cn=" + ldap.EscapeDN(name) + "," + DNSASLMappings
}

// ListSASLMappings returns the mappings in the order the server tries them.
func ListSASLMappings(c *ldap.Conn) ([]SASLMapping, error) {
	res, err := c.Search(ldap.NewSearchRequest(DNSASLMappings, ldap.ScopeSingleLevel, ldap.NeverDerefAliases,
		0, 0, false, "(objectClass=nsSaslMapping)",
		[]string{"cn", "nsSaslMapRegexString", "nsSaslMapBaseDNTemplate", "nsSaslMapFilterTemplate", "nsSaslMapPriority"}, nil))
	if err != nil {
		return nil, err
	}
	out := make([]SASLMapping, 0, len(res.Entries))
	for _, e := range res.Entries {
		prio, _ := strconv.Atoi(e.GetAttributeValue("nsSaslMapPriority"))
		out = append(out, SASLMapping{
			Name:     e.GetAttributeValue("cn"),
			Regex:    e.GetAttributeValue("nsSaslMapRegexString"),
			BaseDN:   e.GetAttributeValue("nsSaslMapBaseDNTemplate"),
			Filter:   e.GetAttributeValue("nsSaslMapFilterTemplate"),
			Priority: prio,
		})
	}
	eff := func(p int) int {
		if p == 0 {
			return 100
		}
		return p
	}
	slices.SortStableFunc(out, func(a, b SASLMapping) int {
		if d := eff(a.Priority) - eff(b.Priority); d != 0 {
			return d
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

func (m *SASLMapping) validate() error {
	m.Name = strings.TrimSpace(m.Name)
	switch {
	case m.Name == "" || strings.ContainsAny(m.Name, ",=+\"\\<>;"):
		return fmt.Errorf("%w: mapping name %q", ErrInvalidName, m.Name)
	case strings.TrimSpace(m.Regex) == "":
		return fmt.Errorf("%w: the regular expression is empty", ErrInvalidValue)
	case strings.TrimSpace(m.BaseDN) == "":
		return fmt.Errorf("%w: the base DN template is empty", ErrInvalidValue)
	case strings.TrimSpace(m.Filter) == "":
		return fmt.Errorf("%w: the filter template is empty", ErrInvalidValue)
	case m.Priority < 0 || m.Priority > 100:
		return fmt.Errorf("%w: priority must be 1–100", ErrInvalidValue)
	}
	return nil
}

func (m *SASLMapping) attrs() map[string][]string {
	a := map[string][]string{
		"nsSaslMapRegexString":    {m.Regex},
		"nsSaslMapBaseDNTemplate": {m.BaseDN},
		"nsSaslMapFilterTemplate": {m.Filter},
	}
	if m.Priority > 0 {
		a["nsSaslMapPriority"] = []string{strconv.Itoa(m.Priority)}
	}
	return a
}

// CreateSASLMapping adds a mapping.
func CreateSASLMapping(c *ldap.Conn, m SASLMapping) error {
	if err := m.validate(); err != nil {
		return err
	}
	req := ldap.NewAddRequest(saslMappingDN(m.Name), nil)
	req.Attribute("objectClass", []string{"top", "nsSaslMapping"})
	req.Attribute("cn", []string{m.Name})
	for k, v := range m.attrs() {
		req.Attribute(k, v)
	}
	return c.Add(req)
}

// UpdateSASLMapping replaces the templates and priority of a mapping.
func UpdateSASLMapping(c *ldap.Conn, name string, m SASLMapping) error {
	m.Name = name
	if err := m.validate(); err != nil {
		return err
	}
	req := ldap.NewModifyRequest(saslMappingDN(name), nil)
	for k, v := range m.attrs() {
		req.Replace(k, v)
	}
	if m.Priority == 0 {
		req.Replace("nsSaslMapPriority", []string{})
	}
	err := c.Modify(req)
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: SASL mapping %q", ErrNotFound, name)
	}
	return err
}

// DeleteSASLMapping removes a mapping.
func DeleteSASLMapping(c *ldap.Conn, name string) error {
	err := c.Del(ldap.NewDelRequest(saslMappingDN(name), nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: SASL mapping %q", ErrNotFound, name)
	}
	return err
}
