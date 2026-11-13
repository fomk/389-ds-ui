// Package health checks a running server for problems an administrator
// should know about: weak settings, expiring certificates, full disks,
// replication errors, missing indexes. It reads the server over LDAP (and
// the NSS database when ds-ui has it) and never changes anything.
package health

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/certs"
)

// Severities, most serious first.
const (
	Error   = "error"
	Warning = "warning"
	Info    = "info"
)

// Finding is one problem. The UI words it from ID and Params
// (health.<id>.title / .detail / .fix).
type Finding struct {
	ID       string            `json:"id" doc:"What was found, e.g. tls-cert-expiring"`
	Severity string            `json:"severity" enum:"error,warning,info"`
	Area     string            `json:"area" enum:"security,tls,disk,database,replication,plugins,logs"`
	Params   map[string]string `json:"params,omitempty" doc:"Values for the text (names, dates, numbers)"`
	Link     string            `json:"link,omitempty" doc:"UI route where it is fixed"`
}

// CheckError is a check that could not run.
type CheckError struct {
	Check   string `json:"check"`
	Message string `json:"message"`
}

// Report is the result of one run.
type Report struct {
	Findings []Finding    `json:"findings"`
	Checks   int          `json:"checks" doc:"Checks that ran"`
	Failed   []CheckError `json:"failed"`
	Started  time.Time    `json:"started"`
	Duration float64      `json:"duration" doc:"Seconds"`
}

// Doer runs a function with an LDAP connection.
type Doer interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// Env is what the checks may read.
type Env struct {
	LDAP  Doer
	Certs *certs.Store // nil when the NSS database is not reachable
	Now   time.Time
}

type check struct {
	id  string
	run func(ctx context.Context, c *ldap.Conn, env Env) ([]Finding, error)
}

var severityRank = map[string]int{Error: 0, Warning: 1, Info: 2}

// Run executes every check. A failing check is reported, the others go on.
func Run(ctx context.Context, env Env) *Report {
	if env.Now.IsZero() {
		env.Now = time.Now()
	}
	rep := &Report{Findings: []Finding{}, Failed: []CheckError{}, Started: env.Now.UTC()}
	start := time.Now()
	for _, ch := range checks {
		var found []Finding
		err := env.LDAP.Do(ctx, func(c *ldap.Conn) (err error) {
			found, err = ch.run(ctx, c, env)
			return err
		})
		if err != nil {
			rep.Failed = append(rep.Failed, CheckError{Check: ch.id, Message: err.Error()})
			continue
		}
		rep.Checks++
		rep.Findings = append(rep.Findings, found...)
	}
	slices.SortStableFunc(rep.Findings, func(a, b Finding) int { return severityRank[a.Severity] - severityRank[b.Severity] })
	rep.Duration = time.Since(start).Seconds()
	return rep
}

// read returns one entry, or nil when it does not exist.
func read(c *ldap.Conn, dn string, attrs ...string) (*ldap.Entry, error) {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", attrs, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dn, err)
	}
	if len(res.Entries) == 0 {
		return nil, nil
	}
	return res.Entries[0], nil
}

func value(e *ldap.Entry, attr string) string {
	if e == nil {
		return ""
	}
	return e.GetAttributeValue(attr)
}
