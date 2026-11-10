//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// bindAs tries a simple bind and returns the LDAP result code (0 = success).
func bindAs(t *testing.T, dn, pw string) uint16 {
	t.Helper()
	conn, err := ldap.DialURL(ldapURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Bind(dn, pw); err != nil {
		var le *ldap.Error
		if errors.As(err, &le) {
			return le.ResultCode
		}
		t.Fatal(err)
	}
	return 0
}

func TestAccounts(t *testing.T) {
	c := newClient(t)
	h := api.NewHandler(api.Deps{LDAP: c, Lifecycle: providers.NoLifecycle{}})
	user := "uid=bjones,ou=People," + suffix
	q := "?dn=" + url.QueryEscape(user)
	status := func(method, path string) ds.AccountStatus {
		t.Helper()
		rec := serve(h, httptestRequest(method, path+q, ""))
		var st ds.AccountStatus
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &st) != nil {
			t.Fatalf("%s %s: %d %s", method, path, rec.Code, rec.Body)
		}
		return st
	}

	if st := status("GET", "/api/v1/accounts/status"); !slices.Equal(st.States, []string{ds.AccountActive}) || !st.HasPassword {
		t.Fatalf("initial: %+v", st)
	}
	if code := bindAs(t, user, "Secret123"); code != 0 {
		t.Fatalf("bind before lock: %d", code)
	}

	// Manual lock.
	if st := status("POST", "/api/v1/accounts/lock"); !slices.Contains(st.States, ds.AccountLocked) || st.LockedByRoleOrCoS {
		t.Errorf("after lock: %+v", st)
	}
	if code := bindAs(t, user, "Secret123"); code == 0 {
		t.Error("locked account could bind")
	}
	if st := status("POST", "/api/v1/accounts/unlock"); !slices.Equal(st.States, []string{ds.AccountActive}) {
		t.Errorf("after unlock: %+v", st)
	}
	if code := bindAs(t, user, "Secret123"); code != 0 {
		t.Errorf("bind after unlock: %d", code)
	}

	// Lockout after failed binds (global policy), then unlock resets it.
	setGlobal := func(vals map[string]string) {
		err := c.Do(context.Background(), func(conn *ldap.Conn) error {
			req := ldap.NewModifyRequest("cn=config", nil)
			for k, v := range vals {
				req.Replace(k, []string{v})
			}
			return conn.Modify(req)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	setGlobal(map[string]string{"passwordLockout": "on", "passwordMaxFailure": "2", "passwordUnlock": "off"})
	defer setGlobal(map[string]string{"passwordLockout": "off", "passwordMaxFailure": "3", "passwordUnlock": "on"})
	for range 3 {
		bindAs(t, user, "wrong")
	}
	st := status("GET", "/api/v1/accounts/status")
	if !slices.Contains(st.States, ds.AccountLockedOut) || st.LockedOutUntil != nil {
		t.Errorf("after failed binds: %+v", st)
	}
	if code := bindAs(t, user, "Secret123"); code == 0 {
		t.Error("locked-out account could bind")
	}
	if st := status("POST", "/api/v1/accounts/unlock"); !slices.Equal(st.States, []string{ds.AccountActive}) || st.FailedBinds != 0 {
		t.Errorf("after unlock: %+v", st)
	}
	if code := bindAs(t, user, "Secret123"); code != 0 {
		t.Errorf("bind after lockout reset: %d", code)
	}

	if rec := serve(h, httptestRequest("GET", "/api/v1/accounts/status?dn="+url.QueryEscape("uid=nobody,"+suffix), "")); rec.Code != 404 {
		t.Errorf("missing entry: %d", rec.Code)
	}
}
