package health

import "testing"

func TestSchemeFinding(t *testing.T) {
	cases := map[string]string{
		"CLEAR": "x-weak", "sha": "x-weak", "MD5": "x-weak", "CRYPT-SHA512": "x-weak",
		"SSHA512": "x-fast", "ssha": "x-fast",
		"PBKDF2-SHA512": "", "PBKDF2_SHA256": "", "ARGON2": "", "": "",
	}
	for scheme, want := range cases {
		f := schemeFinding("x", scheme, "")
		got := ""
		if f != nil {
			got = f.ID
		}
		if got != want {
			t.Errorf("%q: %q, want %q", scheme, got, want)
		}
	}
}
