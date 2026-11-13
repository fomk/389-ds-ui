package ds

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// ControlRealAttrsOnly asks 389-ds not to compute virtual attributes (CoS,
// roles), so a value seen with it is stored in the entry itself.
const ControlRealAttrsOnly = "2.16.840.1.113730.3.4.17"

// Account states, most restrictive first.
const (
	AccountActive        = "active"
	AccountLocked        = "locked"          // nsAccountLock in the entry
	AccountLockedRole    = "locked-role"     // nsAccountLock computed (role or CoS)
	AccountLockedOut     = "locked-out"      // too many failed binds
	AccountExpired       = "expired"         // password expired, no grace logins left
	AccountMustChange    = "must-change"     // password reset by an administrator
	accountLockForeverAt = "19700101000000Z" // accountUnlockTime when only an admin can unlock
)

// AccountStatus describes what keeps an entry from binding.
type AccountStatus struct {
	DN     string   `json:"dn"`
	States []string `json:"states" doc:"active, or any of locked, locked-role, locked-out, expired, must-change"`
	// Effective password policy: a local one (pwdpolicysubentry) or empty for the global policy.
	Policy            string     `json:"policy,omitempty"`
	PasswordExpires   *time.Time `json:"passwordExpires,omitempty"`
	PasswordChanged   *time.Time `json:"passwordChanged,omitempty" doc:"pwdUpdateTime (if tracked)"`
	GraceLoginsUsed   int        `json:"graceLoginsUsed,omitempty"`
	FailedBinds       int        `json:"failedBinds,omitempty" doc:"passwordRetryCount"`
	LockedOutUntil    *time.Time `json:"lockedOutUntil,omitempty" doc:"Absent with locked-out: until an administrator unlocks"`
	LastLogin         *time.Time `json:"lastLogin,omitempty" doc:"lastLoginTime (Account Policy plug-in)"`
	HasPassword       bool       `json:"hasPassword"`
	LockedByRoleOrCoS bool       `json:"lockedByRoleOrCoS" doc:"Unlocking here is not possible; change the role or CoS"`
}

var accountAttrs = []string{"nsAccountLock", "pwdpolicysubentry", "passwordExpirationTime", "passwordGraceUserTime",
	"passwordRetryCount", "accountUnlockTime", "lastLoginTime", "pwdUpdateTime", "pwdReset", "userPassword"}

func genTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	for _, layout := range []string{"20060102150405Z", "20060102150405.000000Z", "20060102150405.0Z"} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t
		}
	}
	return nil
}

func readAccount(c *ldap.Conn, dn string, real bool) (*ldap.Entry, error) {
	var controls []ldap.Control
	if real {
		controls = []ldap.Control{ldap.NewControlString(ControlRealAttrsOnly, true, "")}
	}
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", accountAttrs, controls))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, dn)
	}
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 {
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, dn)
	}
	return res.Entries[0], nil
}

// GetAccountStatus reads the account state of an entry at time now.
func GetAccountStatus(c *ldap.Conn, dn string, now time.Time) (*AccountStatus, error) {
	e, err := readAccount(c, dn, false)
	if err != nil {
		return nil, err
	}
	realLock := false
	if strings.EqualFold(e.GetAttributeValue("nsAccountLock"), "true") {
		real, err := readAccount(c, dn, true)
		if err != nil {
			return nil, err
		}
		realLock = strings.EqualFold(real.GetAttributeValue("nsAccountLock"), "true")
	}
	return accountStatus(e, realLock, now), nil
}

// accountStatus computes the state from an entry read with accountAttrs;
// realLock tells whether nsAccountLock is stored in the entry itself (not
// computed by a role or CoS).
func accountStatus(e *ldap.Entry, realLock bool, now time.Time) *AccountStatus {
	st := &AccountStatus{
		DN:              e.DN,
		States:          []string{},
		Policy:          e.GetAttributeValue("pwdpolicysubentry"),
		PasswordExpires: genTime(e.GetAttributeValue("passwordExpirationTime")),
		PasswordChanged: genTime(e.GetAttributeValue("pwdUpdateTime")),
		LastLogin:       genTime(e.GetAttributeValue("lastLoginTime")),
		HasPassword:     e.GetAttributeValue("userPassword") != "",
	}
	st.GraceLoginsUsed, _ = strconv.Atoi(e.GetAttributeValue("passwordGraceUserTime"))
	st.FailedBinds, _ = strconv.Atoi(e.GetAttributeValue("passwordRetryCount"))

	if strings.EqualFold(e.GetAttributeValue("nsAccountLock"), "true") {
		if realLock {
			st.States = append(st.States, AccountLocked)
		} else {
			st.States = append(st.States, AccountLockedRole)
			st.LockedByRoleOrCoS = true
		}
	}
	if u := e.GetAttributeValue("accountUnlockTime"); u != "" {
		if u == accountLockForeverAt {
			st.States = append(st.States, AccountLockedOut)
		} else if t := genTime(u); t != nil && t.After(now) {
			st.States = append(st.States, AccountLockedOut)
			st.LockedOutUntil = t
		}
	}
	if st.PasswordExpires != nil && st.PasswordExpires.Before(now) {
		// 389-ds marks a reset password as expired at the epoch.
		if st.PasswordExpires.Year() <= 1970 {
			st.States = append(st.States, AccountMustChange)
		} else {
			st.States = append(st.States, AccountExpired)
		}
	}
	if len(st.States) == 0 {
		st.States = append(st.States, AccountActive)
	}
	return st
}

// LockAccount sets nsAccountLock in the entry.
func LockAccount(c *ldap.Conn, dn string) error {
	req := ldap.NewModifyRequest(dn, nil)
	req.Replace("nsAccountLock", []string{"true"})
	err := c.Modify(req)
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return fmt.Errorf("%w: entry %s", ErrNotFound, dn)
	}
	return err
}

// UnlockAccount removes nsAccountLock from the entry and resets a lockout
// after failed binds. A lock that comes from a role or CoS stays.
func UnlockAccount(c *ldap.Conn, dn string) error {
	real, err := readAccount(c, dn, true)
	if err != nil {
		return err
	}
	req := ldap.NewModifyRequest(dn, nil)
	if real.GetAttributeValue("nsAccountLock") != "" {
		req.Delete("nsAccountLock", nil)
	}
	if real.GetAttributeValue("accountUnlockTime") != "" {
		req.Delete("accountUnlockTime", nil)
	}
	if n := real.GetAttributeValue("passwordRetryCount"); n != "" && n != "0" {
		req.Replace("passwordRetryCount", []string{"0"})
	}
	if len(req.Changes) == 0 {
		return nil
	}
	return c.Modify(req)
}
