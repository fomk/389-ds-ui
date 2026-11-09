package ldapx

import (
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// SplitRDN splits a DN into its first RDN and the parent DN, keeping the
// original spelling (no normalisation). Escaped commas (\,) and commas inside
// hex escapes are respected. For a single-RDN DN the parent is "".
func SplitRDN(dn string) (rdn, parent string) {
	escaped := false
	for i := 0; i < len(dn); i++ {
		switch c := dn[i]; {
		case escaped:
			escaped = false
		case c == '\\':
			escaped = true
		case c == ',' || c == ';':
			return strings.TrimSpace(dn[:i]), strings.TrimSpace(dn[i+1:])
		}
	}
	return strings.TrimSpace(dn), ""
}

// NormDN returns a case-insensitive comparison key for a DN. Invalid DNs are
// lower-cased as-is so they still produce a stable key.
func NormDN(dn string) string {
	if p, err := ldap.ParseDN(dn); err == nil {
		return strings.ToLower(p.String())
	}
	return strings.ToLower(strings.TrimSpace(dn))
}

// ValidDN reports whether dn parses as an RFC 4514 distinguished name.
// The empty DN (root DSE) is valid.
func ValidDN(dn string) bool {
	_, err := ldap.ParseDN(dn)
	return err == nil
}
