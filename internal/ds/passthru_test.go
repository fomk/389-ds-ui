package ds

import "testing"

func TestParsePassThru(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ldap://ldap.example.com/o=example", "ldap://ldap.example.com/o=example 3,5,300,3,300,0"},
		{"ldaps://a.example:636 b.example:636/ou=People,dc=example,dc=com 10,7,60,3,600",
			"ldaps://a.example:636 b.example:636/ou=People,dc=example,dc=com 10,7,60,3,600,0"},
		{"ldap://x.example/o=Example Org 3,5,300,3,300,1", "ldap://x.example/o=Example Org 3,5,300,3,300,1"},
	}
	for _, c := range cases {
		s, err := ParsePassThru(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got := s.String(); got != c.want {
			t.Errorf("%q → %q, want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"http://x/o=a", "ldap://x", "ldap:///o=a", "ldaps://x/o=a 3,5,300,3,300,1", "ldap://bad host!/o=a"} {
		if _, err := ParsePassThru(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
