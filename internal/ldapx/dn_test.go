package ldapx

import "testing"

func TestSplitRDN(t *testing.T) {
	cases := []struct{ dn, rdn, parent string }{
		{"uid=jdoe,ou=People,dc=example,dc=com", "uid=jdoe", "ou=People,dc=example,dc=com"},
		{"cn=Doe\\, John,ou=People,dc=example", "cn=Doe\\, John", "ou=People,dc=example"},
		{"cn=a\\\\,ou=x", "cn=a\\\\", "ou=x"},
		{"dc=com", "dc=com", ""},
		{"cn=x+sn=y, ou=People", "cn=x+sn=y", "ou=People"},
		{"", "", ""},
	}
	for _, tc := range cases {
		rdn, parent := SplitRDN(tc.dn)
		if rdn != tc.rdn || parent != tc.parent {
			t.Errorf("SplitRDN(%q) = %q, %q; want %q, %q", tc.dn, rdn, parent, tc.rdn, tc.parent)
		}
	}
}

func TestNormDN(t *testing.T) {
	a := NormDN("UID=JDoe, OU=People,DC=Example,DC=com")
	b := NormDN("uid=jdoe,ou=people,dc=example,dc=com")
	if a != b {
		t.Errorf("%q != %q", a, b)
	}
}

func TestValidDN(t *testing.T) {
	for dn, want := range map[string]bool{
		"":                    true,
		"dc=example,dc=com":   true,
		"cn=Doe\\, John,dc=x": true,
		"not a dn":            false,
		"cn=x,,dc=example":    false,
	} {
		if got := ValidDN(dn); got != want {
			t.Errorf("ValidDN(%q) = %v, want %v", dn, got, want)
		}
	}
}
