package health

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/ds"
)

// UI routes that fix what a check finds.
const (
	linkBinds       = "/settings?page=security-binds"
	linkPwPolicy    = "/policies"
	linkTLS         = "/security"
	linkDisk        = "/settings?page=disk-monitoring"
	linkMonitor     = "/monitor"
	linkDatabases   = "/databases"
	linkReplication = "/replication"
	linkPlugins     = "/plugins"
	linkLogs        = "/settings?page=log-"
)

var checks = []check{
	{"security-binds", checkBinds},
	{"security-password-schemes", checkPasswordSchemes},
	{"tls-settings", checkTLS},
	{"tls-certificates", checkCertificates},
	{"disk-space", checkDisk},
	{"database-backends", checkBackends},
	{"database-caches", checkCaches},
	{"replication", checkReplication},
	{"plugins-indexes", checkPluginIndexes},
	{"plugins-retro-changelog", checkRetroChangelog},
	{"logs", checkLogs},
}

func checkBinds(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	e, err := read(c, "cn=config", "nsslapd-allow-anonymous-access", "nsslapd-allow-unauthenticated-binds",
		"nsslapd-security", "nsslapd-require-secure-binds", "nsslapd-minssf")
	if err != nil {
		return nil, err
	}
	var out []Finding
	if strings.EqualFold(value(e, "nsslapd-allow-anonymous-access"), "on") {
		out = append(out, Finding{ID: "anonymous-access", Severity: Info, Area: "security", Link: linkBinds})
	}
	if strings.EqualFold(value(e, "nsslapd-allow-unauthenticated-binds"), "on") {
		out = append(out, Finding{ID: "unauthenticated-binds", Severity: Warning, Area: "security", Link: linkBinds})
	}
	minssf, _ := strconv.Atoi(value(e, "nsslapd-minssf"))
	if strings.EqualFold(value(e, "nsslapd-security"), "on") && !strings.EqualFold(value(e, "nsslapd-require-secure-binds"), "on") && minssf == 0 {
		out = append(out, Finding{ID: "plain-binds", Severity: Info, Area: "security", Link: linkBinds})
	}
	return out, nil
}

// Password storage schemes: plain or unsalted/fast hashes are errors; salted
// fast SHA hashes are a warning (PBKDF2 or Argon2 resist guessing).
var (
	weakSchemes = []string{"CLEAR", "CRYPT", "CRYPT-MD5", "CRYPT-SHA256", "CRYPT-SHA512", "MD5", "SMD5", "NS-MTA-MD5", "SHA", "SHA256", "SHA384", "SHA512"}
	fastSchemes = []string{"SSHA", "SSHA256", "SSHA384", "SSHA512"}
)

func schemeFinding(id, scheme, link string) *Finding {
	s := strings.ToUpper(strings.TrimSpace(scheme))
	switch {
	case s == "":
		return nil
	case slices.Contains(weakSchemes, s):
		return &Finding{ID: id + "-weak", Severity: Error, Area: "security", Params: map[string]string{"scheme": s}, Link: link}
	case slices.Contains(fastSchemes, s):
		return &Finding{ID: id + "-fast", Severity: Warning, Area: "security", Params: map[string]string{"scheme": s}, Link: link}
	}
	return nil
}

func checkPasswordSchemes(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	e, err := read(c, "cn=config", "passwordStorageScheme", "nsslapd-rootpwstoragescheme")
	if err != nil {
		return nil, err
	}
	var out []Finding
	if f := schemeFinding("password-scheme", value(e, "passwordStorageScheme"), linkPwPolicy); f != nil {
		out = append(out, *f)
	}
	if f := schemeFinding("rootpw-scheme", value(e, "nsslapd-rootpwstoragescheme"), linkPwPolicy); f != nil {
		out = append(out, *f)
	}
	// Local policies may choose their own scheme.
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		"(&(objectClass=passwordPolicy)(passwordStorageScheme=*))", []string{"passwordStorageScheme"}, nil))
	if err == nil {
		for _, p := range res.Entries {
			if f := schemeFinding("password-scheme", p.GetAttributeValue("passwordStorageScheme"), linkPwPolicy); f != nil {
				f.ID = "local-" + f.ID
				f.Params["policy"] = p.DN
				out = append(out, *f)
			}
		}
	}
	return out, nil
}

var tlsOrder = map[string]int{"SSL3": 0, "TLS1.0": 1, "TLS1.1": 2, "TLS1.2": 3, "TLS1.3": 4}

func checkTLS(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	cfg, err := read(c, "cn=config", "nsslapd-security")
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(value(cfg, "nsslapd-security"), "on") {
		return []Finding{{ID: "tls-off", Severity: Warning, Area: "tls", Link: linkTLS}}, nil
	}
	enc, err := read(c, "cn=encryption,cn=config", "sslVersionMin", "allowWeakCipher")
	if err != nil {
		return nil, err
	}
	var out []Finding
	if v := strings.ToUpper(value(enc, "sslVersionMin")); v != "" {
		if n, ok := tlsOrder[v]; ok && n < tlsOrder["TLS1.2"] {
			out = append(out, Finding{ID: "tls-old-versions", Severity: Warning, Area: "tls", Params: map[string]string{"version": v}, Link: linkTLS})
		}
	}
	if strings.EqualFold(value(enc, "allowWeakCipher"), "on") {
		out = append(out, Finding{ID: "tls-weak-ciphers", Severity: Warning, Area: "tls", Link: linkTLS})
	}
	return out, nil
}

func checkCertificates(ctx context.Context, c *ldap.Conn, env Env) ([]Finding, error) {
	if env.Certs == nil {
		return nil, nil // remote mode: the NSS database is not reachable
	}
	server, err := ds.ServerCertName(c)
	if err != nil {
		return nil, err
	}
	list, err := env.Certs.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, cert := range list {
		isServer := cert.Nickname == server
		trustedCA := cert.CA && strings.Contains(cert.Trust, "C")
		if !isServer && !trustedCA {
			continue // only the server certificate and trusted CAs matter
		}
		days := int(cert.NotAfter.Sub(env.Now).Hours() / 24)
		params := map[string]string{"name": cert.Nickname, "date": cert.NotAfter.UTC().Format(time.DateOnly), "days": strconv.Itoa(days)}
		kind := "ca-cert"
		if isServer {
			kind = "server-cert"
		}
		switch {
		case cert.NotAfter.Before(env.Now):
			out = append(out, Finding{ID: kind + "-expired", Severity: Error, Area: "tls", Params: params, Link: linkTLS})
		case days < 30:
			out = append(out, Finding{ID: kind + "-expiring", Severity: Warning, Area: "tls", Params: params, Link: linkTLS})
		}
		if isServer && cert.SelfSigned {
			out = append(out, Finding{ID: "server-cert-self-signed", Severity: Info, Area: "tls", Params: params, Link: linkTLS})
		}
	}
	if server != "" && !slices.ContainsFunc(list, func(x certs.Certificate) bool { return x.Nickname == server }) {
		out = append(out, Finding{ID: "server-cert-missing", Severity: Error, Area: "tls", Params: map[string]string{"name": server}, Link: linkTLS})
	}
	return out, nil
}

func checkDisk(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	m, err := ds.ReadMonitor(c)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, d := range m.Disks {
		params := map[string]string{"partition": d.Partition, "percent": strconv.Itoa(d.UsePct)}
		switch {
		case d.UsePct >= 95:
			out = append(out, Finding{ID: "disk-full", Severity: Error, Area: "disk", Params: params, Link: linkMonitor})
		case d.UsePct >= 85:
			out = append(out, Finding{ID: "disk-filling", Severity: Warning, Area: "disk", Params: params, Link: linkMonitor})
		}
	}
	cfg, err := read(c, "cn=config", "nsslapd-disk-monitoring")
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(value(cfg, "nsslapd-disk-monitoring"), "on") {
		out = append(out, Finding{ID: "disk-monitoring-off", Severity: Info, Area: "disk", Link: linkDisk})
	}
	return out, nil
}

func checkBackends(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	list, err := ds.ListBackends(c)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range list {
		params := map[string]string{"suffix": b.Suffix, "backend": b.Name}
		link := linkDatabases + "?be=" + url.QueryEscape(b.Name)
		if !b.RootExists {
			out = append(out, Finding{ID: "suffix-no-root", Severity: Warning, Area: "database", Params: params, Link: link})
		}
		if b.ReadOnly {
			out = append(out, Finding{ID: "backend-read-only", Severity: Info, Area: "database", Params: params, Link: link})
		}
		if b.State != "" && b.State != "backend" {
			params["state"] = b.State
			out = append(out, Finding{ID: "backend-not-serving", Severity: Info, Area: "database", Params: params, Link: link})
		}
	}
	return out, nil
}

// Entry caches that are full and evict while they serve less than 90% of
// lookups from memory are too small for the data.
func checkCaches(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	m, err := ds.ReadMonitor(c)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range m.Backends {
		ec := b.EntryCache
		if ec.Tries < 10000 || ec.HitRatio >= 90 || ec.MaxSize <= 0 || ec.Size*100 < ec.MaxSize*95 {
			continue
		}
		out = append(out, Finding{ID: "entry-cache-small", Severity: Warning, Area: "database",
			Params: map[string]string{"backend": b.Name, "ratio": strconv.Itoa(ec.HitRatio)}, Link: linkDatabases + "?be=" + url.QueryEscape(b.Name)})
	}
	return out, nil
}

func checkReplication(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	list, err := ds.ListBackends(c)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range list {
		r, err := ds.GetReplica(c, b.Suffix)
		if err != nil {
			return nil, err
		}
		if !r.Enabled {
			continue
		}
		link := linkReplication + "?suffix=" + url.QueryEscape(b.Suffix)
		agmts, err := ds.ListAgreements(c, b.Suffix)
		if err != nil {
			return nil, err
		}
		for _, a := range agmts {
			params := map[string]string{"suffix": b.Suffix, "agreement": a.Name, "status": a.Status.LastUpdate}
			switch {
			case !a.Enabled:
				out = append(out, Finding{ID: "agreement-disabled", Severity: Info, Area: "replication", Params: params, Link: link})
			case a.Status.State == "error":
				out = append(out, Finding{ID: "agreement-error", Severity: Error, Area: "replication", Params: params, Link: link})
			case a.Status.InitState == "error":
				params["status"] = a.Status.LastInit
				out = append(out, Finding{ID: "agreement-init-error", Severity: Error, Area: "replication", Params: params, Link: link})
			}
		}
		cl, err := ds.GetChangelog(c, b.Suffix)
		if err != nil {
			return nil, err
		}
		if cl.Exists && cl.MaxAge == "" && cl.MaxEntries == 0 {
			out = append(out, Finding{ID: "changelog-unbounded", Severity: Warning, Area: "replication", Params: map[string]string{"suffix": b.Suffix}, Link: link})
		}
		conflicts, err := ds.ListConflicts(c, b.Suffix)
		if err != nil {
			return nil, err
		}
		if len(conflicts) > 0 {
			out = append(out, Finding{ID: "replication-conflicts", Severity: Warning, Area: "replication",
				Params: map[string]string{"suffix": b.Suffix, "count": strconv.Itoa(len(conflicts))}, Link: link})
		}
	}
	return out, nil
}

// Plug-ins that search by attribute values on every change need equality
// indexes for those attributes in every backend, or each change scans the
// whole database.
func checkPluginIndexes(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	need := map[string]string{} // attribute → plug-in
	if p, err := read(c, "cn=referential integrity postoperation,"+ds.DNPlugins, "nsslapd-pluginEnabled", "referint-membership-attr"); err != nil {
		return nil, err
	} else if strings.EqualFold(value(p, "nsslapd-pluginEnabled"), "on") {
		for _, a := range p.GetAttributeValues("referint-membership-attr") {
			need[strings.ToLower(a)] = "Referential Integrity"
		}
	}
	if p, err := read(c, "cn=MemberOf Plugin,"+ds.DNPlugins, "nsslapd-pluginEnabled", "memberOfAttr", "memberOfGroupAttr"); err != nil {
		return nil, err
	} else if strings.EqualFold(value(p, "nsslapd-pluginEnabled"), "on") {
		attr := value(p, "memberOfAttr")
		if attr == "" {
			attr = "memberOf"
		}
		need[strings.ToLower(attr)] = "MemberOf"
		for _, a := range p.GetAttributeValues("memberOfGroupAttr") {
			need[strings.ToLower(a)] = "MemberOf"
		}
	}
	if len(need) == 0 {
		return nil, nil
	}
	backends, err := ds.ListBackends(c)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, b := range backends {
		idx, err := ds.ListIndexes(c, b.Name)
		if err != nil {
			return nil, err
		}
		have := map[string]bool{}
		for _, ix := range idx {
			if slices.Contains(ix.Types, "eq") {
				have[strings.ToLower(ix.Attribute)] = true
			}
		}
		attrs := make([]string, 0, len(need))
		for a := range need {
			attrs = append(attrs, a)
		}
		slices.Sort(attrs)
		for _, a := range attrs {
			if !have[a] {
				out = append(out, Finding{ID: "plugin-attr-not-indexed", Severity: Warning, Area: "plugins",
					Params: map[string]string{"attribute": a, "plugin": need[a], "backend": b.Name}, Link: linkDatabases + "?be=" + url.QueryEscape(b.Name)})
			}
		}
	}
	return out, nil
}

func checkRetroChangelog(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	p, err := read(c, "cn=Retro Changelog Plugin,"+ds.DNPlugins, "nsslapd-pluginEnabled", "nsslapd-changelogmaxage")
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(value(p, "nsslapd-pluginEnabled"), "on") && value(p, "nsslapd-changelogmaxage") == "" {
		return []Finding{{ID: "retro-changelog-unbounded", Severity: Warning, Area: "plugins", Link: linkPlugins + "?p=Retro%20Changelog%20Plugin"}}, nil
	}
	return nil, nil
}

func checkLogs(_ context.Context, c *ldap.Conn, _ Env) ([]Finding, error) {
	e, err := read(c, "cn=config", "nsslapd-accesslog-logging-enabled", "nsslapd-errorlog-level", "nsslapd-auditlog-logging-enabled")
	if err != nil {
		return nil, err
	}
	var out []Finding
	if strings.EqualFold(value(e, "nsslapd-accesslog-logging-enabled"), "off") {
		out = append(out, Finding{ID: "access-log-off", Severity: Info, Area: "logs", Link: linkLogs + "access"})
	}
	// 16384 is the default; other bits turn on debugging output that slows
	// the server down and fills the disk.
	if lvl, err := strconv.Atoi(value(e, "nsslapd-errorlog-level")); err == nil && lvl&^16384 != 0 {
		out = append(out, Finding{ID: "error-log-debug", Severity: Warning, Area: "logs", Params: map[string]string{"level": fmt.Sprint(lvl)}, Link: linkLogs + "errors"})
	}
	return out, nil
}
