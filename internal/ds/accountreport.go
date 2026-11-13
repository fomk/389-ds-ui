package ds

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// DefaultAccountFilter selects the entries a report covers by default.
const DefaultAccountFilter = "(|(objectClass=person)(objectClass=posixAccount))"

// MaxAccountReport caps a report; larger subtrees are reported truncated.
const MaxAccountReport = 5000

// AccountRow is one account of a report.
type AccountRow struct {
	AccountStatus
	Name string `json:"name" doc:"uid or cn"`
}

// AccountReport is the state of the accounts under a base.
type AccountReport struct {
	Base      string         `json:"base"`
	Filter    string         `json:"filter"`
	Accounts  []AccountRow   `json:"accounts"`
	Counts    map[string]int `json:"counts" doc:"Accounts per state"`
	Truncated bool           `json:"truncated" doc:"More accounts match than the report holds"`
}

// ReportAccounts reads the accounts under base matching filter (two
// searches: the states, and the locks stored in the entries themselves).
func ReportAccounts(c *ldap.Conn, base, filter string, limit int, now time.Time) (*AccountReport, error) {
	if filter == "" {
		filter = DefaultAccountFilter
	}
	if _, err := ldap.CompileFilter(filter); err != nil {
		return nil, fmt.Errorf("%w: filter: %v", ErrInvalidValue, err)
	}
	if _, err := ldap.ParseDN(base); err != nil || base == "" {
		return nil, fmt.Errorf("%w: base %q", ErrInvalidValue, base)
	}
	if limit <= 0 || limit > MaxAccountReport {
		limit = MaxAccountReport
	}
	attrs := append([]string{"uid", "cn"}, accountAttrs...)
	res, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, limit+1, 0, false,
		filter, attrs, nil))
	truncated := false
	switch {
	case ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) && res != nil:
		truncated = true
	case ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject):
		return nil, fmt.Errorf("%w: entry %s", ErrNotFound, base)
	case err != nil:
		return nil, err
	}
	entries := res.Entries
	if len(entries) > limit {
		entries, truncated = entries[:limit], true
	}

	// Locks stored in the entries; the others come from roles or CoS.
	realLocked := map[string]bool{}
	if slices.ContainsFunc(entries, func(e *ldap.Entry) bool { return strings.EqualFold(e.GetAttributeValue("nsAccountLock"), "true") }) {
		lr, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			"(&"+filter+"(nsAccountLock=true))", []string{"1.1"}, []ldap.Control{ldap.NewControlString(ControlRealAttrsOnly, true, "")}))
		if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
			return nil, err
		}
		if lr != nil {
			for _, e := range lr.Entries {
				realLocked[strings.ToLower(e.DN)] = true
			}
		}
	}

	rep := &AccountReport{Base: base, Filter: filter, Accounts: make([]AccountRow, 0, len(entries)), Counts: map[string]int{}, Truncated: truncated}
	for _, e := range entries {
		st := accountStatus(e, realLocked[strings.ToLower(e.DN)], now)
		name := e.GetAttributeValue("uid")
		if name == "" {
			name = e.GetAttributeValue("cn")
		}
		rep.Accounts = append(rep.Accounts, AccountRow{AccountStatus: *st, Name: name})
		for _, s := range st.States {
			rep.Counts[s]++
		}
	}
	return rep, nil
}
