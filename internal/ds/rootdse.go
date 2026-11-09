// Package ds contains knowledge about 389 Directory Server: where things live
// in cn=config and how to operate on them.
package ds

import (
	"errors"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

const (
	// DNLDBMConfig holds global database settings (nsslapd-backend-implement etc.).
	DNLDBMConfig = "cn=config,cn=ldbm database,cn=plugins,cn=config"

	attrBackendImplement = "nsslapd-backend-implement"
)

// ServerInfo is a summary of the server derived from the root DSE.
type ServerInfo struct {
	Vendor                  string   `json:"vendor"`
	VendorVersion           string   `json:"vendorVersion"`
	Version                 string   `json:"version" doc:"Parsed product version, e.g. 3.1.2"`
	Build                   string   `json:"build,omitempty"`
	NamingContexts          []string `json:"namingContexts"`
	DefaultNamingContext    string   `json:"defaultNamingContext,omitempty"`
	SupportedControls       []string `json:"supportedControls"`
	SupportedExtensions     []string `json:"supportedExtensions"`
	SupportedSASLMechanisms []string `json:"supportedSaslMechanisms"`
	DBImplementation        string   `json:"dbImplementation,omitempty" enum:"mdb,bdb," doc:"Database implementation (LMDB or Berkeley DB)"`
	BoundAs                 string   `json:"boundAs" doc:"Identity ds-ui is bound as"`
}

// GetServerInfo reads the root DSE and a few config attributes.
func GetServerInfo(conn *ldap.Conn) (*ServerInfo, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		"", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)",
		[]string{
			"vendorName", "vendorVersion", "namingContexts", "defaultNamingContext",
			"supportedControl", "supportedExtension", "supportedSASLMechanisms",
		},
		nil,
	))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, errors.New("root DSE is empty")
	}
	e := res.Entries[0]

	info := &ServerInfo{
		Vendor:                  e.GetAttributeValue("vendorName"),
		VendorVersion:           e.GetAttributeValue("vendorVersion"),
		NamingContexts:          nonNil(e.GetAttributeValues("namingContexts")),
		DefaultNamingContext:    e.GetAttributeValue("defaultNamingContext"),
		SupportedControls:       nonNil(e.GetAttributeValues("supportedControl")),
		SupportedExtensions:     nonNil(e.GetAttributeValues("supportedExtension")),
		SupportedSASLMechanisms: nonNil(e.GetAttributeValues("supportedSASLMechanisms")),
	}
	info.Version, info.Build = ParseVendorVersion(info.VendorVersion)

	// Not fatal: older servers or restricted identities may not expose it.
	if impl, err := readAttr(conn, DNLDBMConfig, attrBackendImplement); err == nil {
		info.DBImplementation = strings.ToLower(impl)
	}

	if who, err := conn.WhoAmI(nil); err == nil {
		info.BoundAs = strings.TrimPrefix(who.AuthzID, "dn:")
		info.BoundAs = strings.TrimSpace(info.BoundAs)
	}
	return info, nil
}

var vendorVersionRE = regexp.MustCompile(`/(\d+(?:\.\d+)+)(?:\s+(\S+))?`)

// ParseVendorVersion extracts "3.1.2" and "B2026.125.0709" from
// "389-Directory/3.1.2 B2026.125.0709".
func ParseVendorVersion(s string) (version, build string) {
	m := vendorVersionRE.FindStringSubmatch(s)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

func readAttr(conn *ldap.Conn, dn, attr string) (string, error) {
	res, err := conn.Search(ldap.NewSearchRequest(
		dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{attr}, nil,
	))
	if err != nil {
		return "", err
	}
	if len(res.Entries) == 0 {
		return "", errors.New("entry not found: " + dn)
	}
	return res.Entries[0].GetAttributeValue(attr), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
