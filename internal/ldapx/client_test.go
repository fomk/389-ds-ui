package ldapx

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-ldap/ldap/v3"
)

func TestNewAuthMethod(t *testing.T) {
	cases := []struct {
		url, bindDN string
		want        AuthMethod
	}{
		{"ldapi://%2Fdata%2Frun%2Fslapd-localhost.socket", "", AuthExternal},
		{"ldapi://%2Fdata%2Frun%2Fslapd-localhost.socket", "cn=Directory Manager", AuthSimple},
		{"ldap://ds:3389", "", AuthAnonymous},
		{"ldaps://ds:3636", "cn=Directory Manager", AuthSimple},
	}
	for _, tc := range cases {
		c, err := New(Options{URL: tc.url, BindDN: tc.bindDN, BindPassword: "x"})
		if err != nil {
			t.Fatalf("%s: %v", tc.url, err)
		}
		if c.Auth() != tc.want {
			t.Errorf("%s bind=%q: auth = %s, want %s", tc.url, tc.bindDN, c.Auth(), tc.want)
		}
	}
}

func TestNewRejectsUnknownScheme(t *testing.T) {
	if _, err := New(Options{URL: "http://ds"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRejectsMissingCA(t *testing.T) {
	if _, err := New(Options{URL: "ldaps://ds", CAFile: "/nonexistent/ca.pem"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestIsConnError(t *testing.T) {
	if !IsConnError(&ConnError{Op: "dial", Err: errors.New("refused")}) {
		t.Error("ConnError not detected")
	}
	if !IsConnError(fmt.Errorf("wrapped: %w", ldap.NewError(ldap.ErrorNetwork, errors.New("eof")))) {
		t.Error("network error not detected")
	}
	if IsConnError(ldap.NewError(ldap.LDAPResultNoSuchObject, errors.New("no such object"))) {
		t.Error("noSuchObject treated as connection error")
	}
}
