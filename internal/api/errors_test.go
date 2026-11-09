package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestToHTTPError(t *testing.T) {
	ldapErr := func(code uint16) error {
		e := ldap.NewError(code, errors.New("boom"))
		e.(*ldap.Error).MatchedDN = "dc=example,dc=com"
		return fmt.Errorf("op: %w", e)
	}
	cases := []struct {
		name     string
		err      error
		status   int
		ldapCode int
	}{
		{"noSuchObject", ldapErr(ldap.LDAPResultNoSuchObject), 404, 32},
		{"exists", ldapErr(ldap.LDAPResultEntryAlreadyExists), 409, 68},
		{"ocViolation", ldapErr(ldap.LDAPResultObjectClassViolation), 422, 65},
		{"access", ldapErr(ldap.LDAPResultInsufficientAccessRights), 403, 50},
		{"busy", ldapErr(ldap.LDAPResultBusy), 503, 51},
		{"other", ldapErr(ldap.LDAPResultOther), 500, 80},
		{"conn", &ldapx.ConnError{Op: "dial", Err: errors.New("refused")}, 503, 0},
		{"unsupported", providers.ErrNotSupported, http.StatusNotImplemented, 0},
		{"plain", errors.New("x"), 500, 0},
	}
	for _, tc := range cases {
		var p *Problem
		if !errors.As(toHTTPError(tc.err), &p) {
			t.Fatalf("%s: not a Problem", tc.name)
		}
		if p.Status != tc.status || p.LDAPCode != tc.ldapCode {
			t.Errorf("%s: status=%d ldapCode=%d, want %d/%d", tc.name, p.Status, p.LDAPCode, tc.status, tc.ldapCode)
		}
		if tc.ldapCode != 0 && p.LDAPMatchedDN != "dc=example,dc=com" {
			t.Errorf("%s: matchedDN = %q", tc.name, p.LDAPMatchedDN)
		}
	}
	if toHTTPError(nil) != nil {
		t.Error("nil must stay nil")
	}
}
