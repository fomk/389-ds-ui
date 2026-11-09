package ds

import "testing"

func TestParseVendorVersion(t *testing.T) {
	cases := []struct{ in, version, build string }{
		{"389-Directory/3.1.2 B2026.125.0709", "3.1.2", "B2026.125.0709"},
		{"389-Directory/2.3.1", "2.3.1", ""},
		{"389-Directory/1.4.4.17 B2021.272.0000", "1.4.4.17", "B2021.272.0000"},
		{"garbage", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		v, b := ParseVendorVersion(tc.in)
		if v != tc.version || b != tc.build {
			t.Errorf("ParseVendorVersion(%q) = %q, %q; want %q, %q", tc.in, v, b, tc.version, tc.build)
		}
	}
}
