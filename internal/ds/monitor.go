package ds

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// Monitor is a snapshot of the server's cn=monitor tree.
type Monitor struct {
	Server      ServerMonitor    `json:"server"`
	Operations  map[string]int64 `json:"operations" doc:"Counters from cn=snmp,cn=monitor (lower-case names)"`
	Disks       []DiskUsage      `json:"disks"`
	NDNCache    CacheStats       `json:"ndnCache" doc:"Normalized DN cache (global)"`
	Backends    []BackendMonitor `json:"backends"`
	Connections []ConnectionInfo `json:"connections"`
	Collected   time.Time        `json:"collected"`
}

// ServerMonitor holds the main counters of cn=monitor.
type ServerMonitor struct {
	Version                 string    `json:"version"`
	StartTime               time.Time `json:"startTime"`
	CurrentTime             time.Time `json:"currentTime"`
	UptimeSeconds           int64     `json:"uptimeSeconds"`
	Threads                 int64     `json:"threads"`
	CurrentConnections      int64     `json:"currentConnections"`
	TotalConnections        int64     `json:"totalConnections"`
	ConnectionsAtMaxThreads int64     `json:"connectionsAtMaxThreads"`
	MaxThreadsPerConnHits   int64     `json:"maxThreadsPerConnHits"`
	ReadWaiters             int64     `json:"readWaiters"`
	OpsInitiated            int64     `json:"opsInitiated"`
	OpsCompleted            int64     `json:"opsCompleted"`
	EntriesSent             int64     `json:"entriesSent"`
	BytesSent               int64     `json:"bytesSent"`
	DTableSize              int64     `json:"dtableSize" doc:"File descriptor table size"`
}

// DiskUsage is one partition from cn=disk space,cn=monitor.
type DiskUsage struct {
	Partition string `json:"partition"`
	Size      int64  `json:"size"`
	Used      int64  `json:"used"`
	Available int64  `json:"available"`
	UsePct    int    `json:"usePct"`
}

// CacheStats describes one cache.
type CacheStats struct {
	HitRatio  int   `json:"hitRatio" doc:"Percent"`
	Hits      int64 `json:"hits"`
	Tries     int64 `json:"tries"`
	Size      int64 `json:"size" doc:"Bytes in use"`
	MaxSize   int64 `json:"maxSize" doc:"Bytes"`
	Count     int64 `json:"count"`
	MaxCount  int64 `json:"maxCount" doc:"-1 = unlimited"`
	Evictions int64 `json:"evictions,omitempty"`
}

// BackendMonitor holds the cache statistics of one backend.
type BackendMonitor struct {
	Name       string     `json:"name"`
	Suffix     string     `json:"suffix,omitempty"`
	ReadOnly   bool       `json:"readOnly"`
	EntryCache CacheStats `json:"entryCache"`
	DNCache    CacheStats `json:"dnCache"`
}

// ConnectionInfo is one line of the "connection" attribute of cn=monitor.
type ConnectionInfo struct {
	ID           string    `json:"id"`
	FD           string    `json:"fd"`
	Opened       time.Time `json:"opened"`
	OpsInitiated int64     `json:"opsInitiated"`
	OpsCompleted int64     `json:"opsCompleted"`
	BindDN       string    `json:"bindDn"`
	Address      string    `json:"address"`
}

func readEntry(c *ldap.Conn, dn string, attrs ...string) (*ldap.Entry, error) {
	if len(attrs) == 0 {
		attrs = []string{"*"}
	}
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", attrs, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, ldap.NewError(ldap.LDAPResultNoSuchObject, nil)
	}
	return res.Entries[0], nil
}

func num(e *ldap.Entry, attr string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(e.GetAttributeValue(attr)), 10, 64)
	return n
}

// ParseGeneralizedTime parses 389-ds timestamps such as 20261110065802Z.
func ParseGeneralizedTime(v string) time.Time {
	t, err := time.Parse("20060102150405Z", strings.TrimSpace(v))
	if err != nil {
		return time.Time{}
	}
	return t
}

// ReadMonitor collects a monitoring snapshot.
func ReadMonitor(c *ldap.Conn) (*Monitor, error) {
	m := &Monitor{Operations: map[string]int64{}, Disks: []DiskUsage{}, Backends: []BackendMonitor{}, Connections: []ConnectionInfo{}}

	e, err := readEntry(c, "cn=monitor")
	if err != nil {
		return nil, err
	}
	s := &m.Server
	s.Version = e.GetAttributeValue("version")
	s.StartTime = ParseGeneralizedTime(e.GetAttributeValue("starttime"))
	s.CurrentTime = ParseGeneralizedTime(e.GetAttributeValue("currenttime"))
	if !s.StartTime.IsZero() && !s.CurrentTime.IsZero() {
		s.UptimeSeconds = int64(s.CurrentTime.Sub(s.StartTime).Seconds())
	}
	s.Threads = num(e, "threads")
	s.CurrentConnections = num(e, "currentconnections")
	s.TotalConnections = num(e, "totalconnections")
	s.ConnectionsAtMaxThreads = num(e, "currentconnectionsatmaxthreads")
	s.MaxThreadsPerConnHits = num(e, "maxthreadsperconnhits")
	s.ReadWaiters = num(e, "readwaiters")
	s.OpsInitiated = num(e, "opsinitiated")
	s.OpsCompleted = num(e, "opscompleted")
	s.EntriesSent = num(e, "entriessent")
	s.BytesSent = num(e, "bytessent")
	s.DTableSize = num(e, "dtablesize")
	for _, v := range e.GetAttributeValues("connection") {
		if ci, ok := ParseConnection(v); ok {
			m.Connections = append(m.Connections, ci)
		}
	}
	backendDNs := e.GetAttributeValues("backendmonitordn")

	// The remaining parts are best effort: older servers may lack some of them.
	if snmp, err := readEntry(c, "cn=snmp,cn=monitor"); err == nil {
		for _, a := range snmp.Attributes {
			n := strings.ToLower(a.Name)
			if n == "objectclass" || n == "cn" {
				continue
			}
			if v, err := strconv.ParseInt(a.Values[0], 10, 64); err == nil {
				m.Operations[n] = v
			}
		}
	}
	if disk, err := readEntry(c, "cn=disk space,cn=monitor", "dsDisk"); err == nil {
		for _, v := range disk.GetAttributeValues("dsDisk") {
			if d, ok := ParseDisk(v); ok {
				m.Disks = append(m.Disks, d)
			}
		}
	}
	if ldbm, err := readEntry(c, "cn=monitor,"+DNLDBM); err == nil {
		m.NDNCache = CacheStats{
			HitRatio:  int(num(ldbm, "normalizeddncachehitratio")),
			Hits:      num(ldbm, "normalizeddncachehits"),
			Tries:     num(ldbm, "normalizeddncachetries"),
			Size:      num(ldbm, "currentnormalizeddncachesize"),
			MaxSize:   num(ldbm, "maxnormalizeddncachesize"),
			Count:     num(ldbm, "currentnormalizeddncachecount"),
			Evictions: num(ldbm, "normalizeddncacheevictions"),
		}
	}
	for _, dn := range backendDNs {
		be, err := readEntry(c, dn, "readonly", "entrycachehits", "entrycachetries", "entrycachehitratio",
			"currententrycachesize", "maxentrycachesize", "currententrycachecount", "maxentrycachecount",
			"dncachehits", "dncachetries", "dncachehitratio", "currentdncachesize", "maxdncachesize",
			"currentdncachecount", "maxdncachecount")
		if err != nil {
			continue
		}
		bm := BackendMonitor{
			Name:     backendNameFromMonitorDN(dn),
			ReadOnly: be.GetAttributeValue("readonly") == "1",
			EntryCache: CacheStats{
				HitRatio: int(num(be, "entrycachehitratio")), Hits: num(be, "entrycachehits"), Tries: num(be, "entrycachetries"),
				Size: num(be, "currententrycachesize"), MaxSize: num(be, "maxentrycachesize"),
				Count: num(be, "currententrycachecount"), MaxCount: num(be, "maxentrycachecount"),
			},
			DNCache: CacheStats{
				HitRatio: int(num(be, "dncachehitratio")), Hits: num(be, "dncachehits"), Tries: num(be, "dncachetries"),
				Size: num(be, "currentdncachesize"), MaxSize: num(be, "maxdncachesize"),
				Count: num(be, "currentdncachecount"), MaxCount: num(be, "maxdncachecount"),
			},
		}
		if cfg, err := readEntry(c, "cn="+ldap.EscapeDN(bm.Name)+","+DNLDBM, "nsslapd-suffix"); err == nil {
			bm.Suffix = cfg.GetAttributeValue("nsslapd-suffix")
		}
		m.Backends = append(m.Backends, bm)
	}
	m.Collected = time.Now().UTC()
	return m, nil
}

// DNLDBM is the parent of all backend instances.
const DNLDBM = "cn=ldbm database,cn=plugins,cn=config"

// "cn=monitor,cn=userroot,cn=ldbm database,..." → "userroot".
func backendNameFromMonitorDN(dn string) string {
	parts := strings.SplitN(dn, ",", 3)
	if len(parts) < 2 {
		return dn
	}
	return strings.TrimPrefix(strings.TrimPrefix(parts[1], "cn="), "CN=")
}

var diskRE = regexp.MustCompile(`(\w[\w%]*)="([^"]*)"`)

// ParseDisk parses dsDisk: partition="/data" size="…" used="…" available="…" use%="8".
func ParseDisk(v string) (DiskUsage, bool) {
	kv := map[string]string{}
	for _, m := range diskRE.FindAllStringSubmatch(v, -1) {
		kv[m[1]] = m[2]
	}
	if kv["partition"] == "" {
		return DiskUsage{}, false
	}
	p := func(k string) int64 { n, _ := strconv.ParseInt(kv[k], 10, 64); return n }
	pct, _ := strconv.Atoi(kv["use%"])
	return DiskUsage{Partition: kv["partition"], Size: p("size"), Used: p("used"), Available: p("available"), UsePct: pct}, true
}

// ParseConnection parses one "connection" value of cn=monitor:
// fd:opentime:opsinitiated:opscompleted:rw:binddn:…(3 counters)…:connid:ip=addr
// The bind DN may itself contain colons, so fields are taken from both ends.
func ParseConnection(v string) (ConnectionInfo, bool) {
	p := strings.Split(v, ":")
	if len(p) < 11 {
		return ConnectionInfo{}, false
	}
	n := len(p)
	init, _ := strconv.ParseInt(p[2], 10, 64)
	done, _ := strconv.ParseInt(p[3], 10, 64)
	return ConnectionInfo{
		FD:           p[0],
		Opened:       ParseGeneralizedTime(p[1]),
		OpsInitiated: init,
		OpsCompleted: done,
		BindDN:       strings.Join(p[5:n-5], ":"),
		ID:           p[n-2],
		Address:      strings.TrimPrefix(p[n-1], "ip="),
	}, true
}
