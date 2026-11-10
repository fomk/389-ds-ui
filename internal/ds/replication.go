package ds

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ldapx"
)

// Replica roles.
const (
	RoleSupplier = "supplier"
	RoleHub      = "hub"
	RoleConsumer = "consumer"
)

// ErrNotReplicated is returned for operations on a suffix without a replica.
var ErrNotReplicated = errors.New("replication is not enabled for this suffix")

// consumerID is the replica ID of hubs and consumers.
const consumerID = 65535

// Replica is the replication configuration of a suffix.
type Replica struct {
	Suffix     string   `json:"suffix"`
	Backend    string   `json:"backend"`
	Enabled    bool     `json:"enabled"`
	Role       string   `json:"role,omitempty" enum:"supplier,hub,consumer"`
	ReplicaID  int      `json:"replicaId,omitempty"`
	BindDNs    []string `json:"bindDNs" doc:"nsDS5ReplicaBindDN: accounts suppliers bind as"`
	BindGroup  string   `json:"bindGroup,omitempty" doc:"nsDS5ReplicaBindDNGroup: members may bind as suppliers"`
	PurgeDelay int      `json:"purgeDelay,omitempty" doc:"nsds5ReplicaPurgeDelay (seconds)"`
	DN         string   `json:"dn,omitempty"`
}

// EnableReplica turns replication on for a suffix.
type EnableReplica struct {
	Role      string   `json:"role" enum:"supplier,hub,consumer"`
	ReplicaID int      `json:"replicaId,omitempty" doc:"1–65534, suppliers only"`
	BindDNs   []string `json:"bindDNs,omitempty"`
	BindGroup string   `json:"bindGroup,omitempty"`
}

// ReplicaPatch changes a replica; nil fields stay.
type ReplicaPatch struct {
	Role       *string  `json:"role,omitempty" enum:"supplier,hub,consumer" doc:"Promote or demote"`
	ReplicaID  *int     `json:"replicaId,omitempty"`
	BindDNs    []string `json:"bindDNs,omitempty"`
	BindGroup  *string  `json:"bindGroup,omitempty"`
	PurgeDelay *int     `json:"purgeDelay,omitempty"`
}

// mappingTree finds the mapping tree entry and backend of a suffix.
func mappingTree(c *ldap.Conn, suffix string) (dn, backend string, err error) {
	res, err := c.Search(ldap.NewSearchRequest(dnMappingTree, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsMappingTree)", []string{"cn", "nsslapd-backend"}, nil))
	if err != nil {
		return "", "", err
	}
	for _, e := range res.Entries {
		for _, cn := range e.GetAttributeValues("cn") {
			if ldapx.NormDN(cn) == ldapx.NormDN(suffix) {
				return e.DN, e.GetAttributeValue("nsslapd-backend"), nil
			}
		}
	}
	return "", "", fmt.Errorf("%w: suffix %s", ErrNotFound, suffix)
}

func replicaDN(mt string) string { return "cn=replica," + mt }

var replicaAttrs = []string{"nsDS5ReplicaRoot", "nsDS5ReplicaId", "nsDS5ReplicaType", "nsDS5Flags",
	"nsDS5ReplicaBindDN", "nsDS5ReplicaBindDNGroup", "nsds5ReplicaPurgeDelay"}

// GetReplica reads the replication configuration of a suffix.
func GetReplica(c *ldap.Conn, suffix string) (*Replica, error) {
	mt, be, err := mappingTree(c, suffix)
	if err != nil {
		return nil, err
	}
	r := &Replica{Suffix: suffix, Backend: be, BindDNs: []string{}}
	res, err := c.Search(ldap.NewSearchRequest(replicaDN(mt), ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=nsds5Replica)", replicaAttrs, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || err == nil && len(res.Entries) == 0 {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	e := res.Entries[0]
	r.Enabled, r.DN = true, e.DN
	r.ReplicaID, _ = strconv.Atoi(e.GetAttributeValue("nsDS5ReplicaId"))
	r.PurgeDelay, _ = strconv.Atoi(e.GetAttributeValue("nsds5ReplicaPurgeDelay"))
	r.BindDNs = append(r.BindDNs, e.GetAttributeValues("nsDS5ReplicaBindDN")...)
	r.BindGroup = e.GetAttributeValue("nsDS5ReplicaBindDNGroup")
	switch {
	case e.GetAttributeValue("nsDS5ReplicaType") == "3":
		r.Role = RoleSupplier
	case e.GetAttributeValue("nsDS5Flags") == "1":
		r.Role = RoleHub
	default:
		r.Role = RoleConsumer
	}
	return r, nil
}

// replicaRoleAttrs returns type, flags and ID for a role.
func replicaRoleAttrs(role string, id int) (typ, flags string, rid int, err error) {
	switch role {
	case RoleSupplier:
		if id < 1 || id > 65534 {
			return "", "", 0, fmt.Errorf("%w: a supplier needs a replica ID from 1 to 65534", ErrInvalidValue)
		}
		return "3", "1", id, nil
	case RoleHub:
		return "2", "1", consumerID, nil
	case RoleConsumer:
		return "2", "0", consumerID, nil
	}
	return "", "", 0, fmt.Errorf("%w: role %q", ErrInvalidValue, role)
}

func checkDNs(list []string) ([]string, error) {
	var out []string
	for _, d := range list {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if _, err := ldap.ParseDN(d); err != nil {
			return nil, fmt.Errorf("%w: %q is not a DN", ErrInvalidValue, d)
		}
		out = append(out, d)
	}
	return out, nil
}

// EnableReplication creates the replica entry of a suffix. Suppliers and
// hubs get a changelog from the server.
func EnableReplication(c *ldap.Conn, suffix string, e EnableReplica) (*Replica, error) {
	cur, err := GetReplica(c, suffix)
	if err != nil {
		return nil, err
	}
	if cur.Enabled {
		return nil, fmt.Errorf("%w: replication of %s", ErrExists, suffix)
	}
	typ, flags, rid, err := replicaRoleAttrs(e.Role, e.ReplicaID)
	if err != nil {
		return nil, err
	}
	binds, err := checkDNs(e.BindDNs)
	if err != nil {
		return nil, err
	}
	mt, _, err := mappingTree(c, suffix)
	if err != nil {
		return nil, err
	}
	attrs := map[string][]string{
		"objectClass":      {"top", "nsds5Replica", "extensibleObject"},
		"cn":               {"replica"},
		"nsDS5ReplicaRoot": {suffix},
		"nsDS5ReplicaId":   {strconv.Itoa(rid)},
		"nsDS5ReplicaType": {typ},
		"nsDS5Flags":       {flags},
	}
	if len(binds) > 0 {
		attrs["nsDS5ReplicaBindDN"] = binds
	}
	if g := strings.TrimSpace(e.BindGroup); g != "" {
		if _, err := ldap.ParseDN(g); err != nil {
			return nil, fmt.Errorf("%w: bind group %q", ErrInvalidValue, g)
		}
		attrs["nsDS5ReplicaBindDNGroup"] = []string{g}
	}
	if err := addEntry(c, replicaDN(mt), attrs); err != nil {
		return nil, err
	}
	return GetReplica(c, suffix)
}

// UpdateReplica changes role, ID, bind DNs/group or purge delay.
func UpdateReplica(c *ldap.Conn, suffix string, p ReplicaPatch) (*Replica, error) {
	cur, err := GetReplica(c, suffix)
	if err != nil {
		return nil, err
	}
	if !cur.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrNotReplicated, suffix)
	}
	req := ldap.NewModifyRequest(cur.DN, nil)
	if p.Role != nil || p.ReplicaID != nil {
		role, id := cur.Role, cur.ReplicaID
		if p.Role != nil {
			role = *p.Role
		}
		if p.ReplicaID != nil {
			id = *p.ReplicaID
		}
		typ, flags, rid, err := replicaRoleAttrs(role, id)
		if err != nil {
			return nil, err
		}
		req.Replace("nsDS5ReplicaType", []string{typ})
		req.Replace("nsDS5Flags", []string{flags})
		req.Replace("nsDS5ReplicaId", []string{strconv.Itoa(rid)})
	}
	if p.BindDNs != nil {
		binds, err := checkDNs(p.BindDNs)
		if err != nil {
			return nil, err
		}
		req.Replace("nsDS5ReplicaBindDN", binds)
	}
	if p.BindGroup != nil {
		req.Replace("nsDS5ReplicaBindDNGroup", nonEmpty(*p.BindGroup))
	}
	if p.PurgeDelay != nil {
		if *p.PurgeDelay < -1 {
			return nil, fmt.Errorf("%w: purge delay", ErrInvalidValue)
		}
		req.Replace("nsds5ReplicaPurgeDelay", []string{strconv.Itoa(*p.PurgeDelay)})
	}
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	return GetReplica(c, suffix)
}

// DisableReplication removes the agreements and the replica entry.
func DisableReplication(c *ldap.Conn, suffix string) error {
	cur, err := GetReplica(c, suffix)
	if err != nil {
		return err
	}
	if !cur.Enabled {
		return fmt.Errorf("%w: %s", ErrNotReplicated, suffix)
	}
	res, err := c.Search(ldap.NewSearchRequest(cur.DN, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=*)", []string{"1.1"}, nil))
	if err != nil {
		return err
	}
	for _, e := range res.Entries {
		if err := c.Del(ldap.NewDelRequest(e.DN, nil)); err != nil {
			return err
		}
	}
	return c.Del(ldap.NewDelRequest(cur.DN, nil))
}

// --- changelog -------------------------------------------------------------------

// Changelog is the replication changelog of a backend.
type Changelog struct {
	DN           string `json:"dn"`
	Exists       bool   `json:"exists"`
	MaxAge       string `json:"maxAge,omitempty" doc:"nsslapd-changelogmaxage, e.g. 7d"`
	MaxEntries   int    `json:"maxEntries,omitempty" doc:"nsslapd-changelogmaxentries; 0: unlimited"`
	TrimInterval int    `json:"trimInterval,omitempty" doc:"nsslapd-changelogtrim-interval (seconds)"`
}

// ChangelogPatch changes the changelog limits; nil fields stay, "" / 0 removes.
type ChangelogPatch struct {
	MaxAge       *string `json:"maxAge,omitempty"`
	MaxEntries   *int    `json:"maxEntries,omitempty"`
	TrimInterval *int    `json:"trimInterval,omitempty"`
}

var maxAgeRE = regexp.MustCompile(`^[0-9]+[smhdw]?$`)

// GetChangelog reads the changelog of the suffix's backend.
func GetChangelog(c *ldap.Conn, suffix string) (*Changelog, error) {
	_, be, err := mappingTree(c, suffix)
	if err != nil {
		return nil, err
	}
	cl := &Changelog{DN: "cn=changelog," + backendDN(be)}
	res, err := c.Search(ldap.NewSearchRequest(cl.DN, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"nsslapd-changelogmaxage", "nsslapd-changelogmaxentries", "nsslapd-changelogtrim-interval"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || err == nil && len(res.Entries) == 0 {
		return cl, nil
	}
	if err != nil {
		return nil, err
	}
	e := res.Entries[0]
	cl.Exists = true
	cl.MaxAge = e.GetAttributeValue("nsslapd-changelogmaxage")
	cl.MaxEntries, _ = strconv.Atoi(e.GetAttributeValue("nsslapd-changelogmaxentries"))
	cl.TrimInterval, _ = strconv.Atoi(e.GetAttributeValue("nsslapd-changelogtrim-interval"))
	return cl, nil
}

// UpdateChangelog changes the changelog limits.
func UpdateChangelog(c *ldap.Conn, suffix string, p ChangelogPatch) (*Changelog, error) {
	cl, err := GetChangelog(c, suffix)
	if err != nil {
		return nil, err
	}
	if !cl.Exists {
		return nil, fmt.Errorf("%w: changelog of %s (enable replication as supplier or hub)", ErrNotFound, suffix)
	}
	req := ldap.NewModifyRequest(cl.DN, nil)
	if p.MaxAge != nil {
		v := strings.TrimSpace(*p.MaxAge)
		if v != "" && !maxAgeRE.MatchString(v) {
			return nil, fmt.Errorf("%w: max age %q (number with s, m, h, d or w)", ErrInvalidValue, v)
		}
		req.Replace("nsslapd-changelogmaxage", nonEmpty(v))
	}
	if p.MaxEntries != nil {
		if *p.MaxEntries < 0 {
			return nil, fmt.Errorf("%w: max entries", ErrInvalidValue)
		}
		req.Replace("nsslapd-changelogmaxentries", []string{strconv.Itoa(*p.MaxEntries)})
	}
	if p.TrimInterval != nil {
		if *p.TrimInterval < 1 {
			return nil, fmt.Errorf("%w: trim interval", ErrInvalidValue)
		}
		req.Replace("nsslapd-changelogtrim-interval", []string{strconv.Itoa(*p.TrimInterval)})
	}
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	return GetChangelog(c, suffix)
}

// --- replication manager ---------------------------------------------------------

// DefaultReplManager is the usual account suppliers bind as.
const DefaultReplManager = "cn=replication manager,cn=config"

// CreateReplManager adds an account for suppliers to bind as. Its password
// does not expire.
func CreateReplManager(c *ldap.Conn, dn, password string) error {
	if dn = strings.TrimSpace(dn); dn == "" {
		dn = DefaultReplManager
	}
	d, err := ldap.ParseDN(dn)
	if err != nil || len(d.RDNs) < 2 || len(d.RDNs[0].Attributes) != 1 || !strings.EqualFold(d.RDNs[0].Attributes[0].Type, "cn") {
		return fmt.Errorf("%w: %q must be cn=<name>,<parent>", ErrInvalidValue, dn)
	}
	if len(password) < MinRootPasswordLength {
		return fmt.Errorf("%w: the password must have at least %d characters", ErrInvalidValue, MinRootPasswordLength)
	}
	ok, err := exists(c, dn)
	if err != nil {
		return err
	}
	if ok {
		req := ldap.NewModifyRequest(dn, nil)
		req.Replace("userPassword", []string{password})
		return c.Modify(req)
	}
	cn := d.RDNs[0].Attributes[0].Value
	return addEntry(c, dn, map[string][]string{
		"objectClass":            {"top", "person", "organizationalPerson", "inetOrgPerson"},
		"cn":                     {cn},
		"sn":                     {cn},
		"userPassword":           {password},
		"passwordExpirationTime": {"20380119031407Z"},
		"nsIdleTimeout":          {"0"},
	})
}

// --- agreements ------------------------------------------------------------------

// Agreement is a replication agreement: this supplier sends changes to a
// consumer.
type Agreement struct {
	Name        string          `json:"name"`
	DN          string          `json:"dn"`
	Host        string          `json:"host"`
	Port        int             `json:"port"`
	Transport   string          `json:"transport" enum:"LDAP,StartTLS,LDAPS" doc:"nsDS5ReplicaTransportInfo: LDAP, TLS (StartTLS), SSL (LDAPS)"`
	BindMethod  string          `json:"bindMethod" enum:"SIMPLE,SSLCLIENTAUTH,SASL/GSSAPI,SASL/DIGEST-MD5"`
	BindDN      string          `json:"bindDN,omitempty"`
	Enabled     bool            `json:"enabled"`
	Schedule    string          `json:"schedule,omitempty" doc:"nsDS5ReplicaUpdateSchedule, e.g. 0800-1700 12345; empty: always"`
	Excluded    []string        `json:"excluded" doc:"Fractional replication: attributes not sent"`
	Description string          `json:"description,omitempty"`
	Status      AgreementStatus `json:"status"`
}

// AgreementStatus is what the supplier reports about an agreement.
type AgreementStatus struct {
	State         string     `json:"state" enum:"ok,error,busy,never,unknown" doc:"From the last update status"`
	LastUpdate    string     `json:"lastUpdate,omitempty" doc:"nsds5replicaLastUpdateStatus"`
	LastUpdateEnd *time.Time `json:"lastUpdateEnd,omitempty"`
	InProgress    bool       `json:"inProgress"`
	InitState     string     `json:"initState" enum:"ok,error,busy,never,unknown"`
	LastInit      string     `json:"lastInit,omitempty" doc:"nsds5replicaLastInitStatus"`
	LastInitEnd   *time.Time `json:"lastInitEnd,omitempty"`
	ChangesSent   string     `json:"changesSent,omitempty" doc:"nsds5replicaChangesSentSinceStartup"`
	ReapActive    bool       `json:"reapActive,omitempty"`
}

// AgreementInput creates or changes an agreement.
type AgreementInput struct {
	Name        string   `json:"name,omitempty"`
	Host        string   `json:"host,omitempty"`
	Port        int      `json:"port,omitempty"`
	Transport   string   `json:"transport,omitempty" enum:"LDAP,StartTLS,LDAPS"`
	BindMethod  string   `json:"bindMethod,omitempty" enum:"SIMPLE,SSLCLIENTAUTH,SASL/GSSAPI,SASL/DIGEST-MD5"`
	BindDN      string   `json:"bindDN,omitempty"`
	Credentials string   `json:"credentials,omitempty" doc:"Password for SIMPLE/DIGEST-MD5; empty keeps the current one"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Schedule    *string  `json:"schedule,omitempty"`
	Excluded    []string `json:"excluded,omitempty"`
	Description *string  `json:"description,omitempty"`
}

var agreementAttrs = []string{"cn", "nsDS5ReplicaHost", "nsDS5ReplicaPort", "nsDS5ReplicaTransportInfo", "nsDS5ReplicaBindMethod",
	"nsDS5ReplicaBindDN", "nsds5ReplicaEnabled", "nsDS5ReplicaUpdateSchedule", "nsDS5ReplicatedAttributeList", "description",
	"nsds5replicaLastUpdateStatus", "nsds5replicaLastUpdateEnd", "nsds5replicaUpdateInProgress",
	"nsds5replicaLastInitStatus", "nsds5replicaLastInitEnd", "nsds5replicaChangesSentSinceStartup", "nsds5replicaReapActive"}

var transports = map[string]string{"LDAP": "LDAP", "StartTLS": "TLS", "LDAPS": "SSL"}

// statusState reads "Error (0) Replica acquired successfully: …".
func statusState(s string) string {
	switch {
	case s == "":
		return "never"
	case strings.HasPrefix(s, "Error (0)") || strings.HasPrefix(s, "0 "):
		if strings.Contains(strings.ToLower(s), "in progress") {
			return "busy"
		}
		return "ok"
	case strings.HasPrefix(s, "Error ("), strings.HasPrefix(s, "-"):
		return "error"
	}
	return "unknown"
}

const excludePrefix = "(objectclass=*) $ EXCLUDE"

func agreementFromEntry(e *ldap.Entry) Agreement {
	a := Agreement{Name: e.GetAttributeValue("cn"), DN: e.DN, Host: e.GetAttributeValue("nsDS5ReplicaHost"),
		BindMethod: e.GetAttributeValue("nsDS5ReplicaBindMethod"), BindDN: e.GetAttributeValue("nsDS5ReplicaBindDN"),
		Enabled:  !strings.EqualFold(e.GetAttributeValue("nsds5ReplicaEnabled"), "off"),
		Schedule: e.GetAttributeValue("nsDS5ReplicaUpdateSchedule"), Description: e.GetAttributeValue("description"),
		Excluded: []string{}}
	a.Port, _ = strconv.Atoi(e.GetAttributeValue("nsDS5ReplicaPort"))
	a.Transport = "LDAP"
	for k, v := range transports {
		if strings.EqualFold(e.GetAttributeValue("nsDS5ReplicaTransportInfo"), v) {
			a.Transport = k
		}
	}
	if a.BindMethod == "" {
		a.BindMethod = "SIMPLE"
	}
	if l := e.GetAttributeValue("nsDS5ReplicatedAttributeList"); len(l) > len(excludePrefix) && strings.EqualFold(l[:len(excludePrefix)], excludePrefix) {
		a.Excluded = strings.Fields(l[len(excludePrefix):])
	}
	st := &a.Status
	st.LastUpdate = e.GetAttributeValue("nsds5replicaLastUpdateStatus")
	st.State = statusState(st.LastUpdate)
	st.LastUpdateEnd = genTime(e.GetAttributeValue("nsds5replicaLastUpdateEnd"))
	st.InProgress = strings.EqualFold(e.GetAttributeValue("nsds5replicaUpdateInProgress"), "TRUE")
	st.LastInit = e.GetAttributeValue("nsds5replicaLastInitStatus")
	st.InitState = statusState(st.LastInit)
	st.LastInitEnd = genTime(e.GetAttributeValue("nsds5replicaLastInitEnd"))
	st.ChangesSent = e.GetAttributeValue("nsds5replicaChangesSentSinceStartup")
	st.ReapActive = e.GetAttributeValue("nsds5replicaReapActive") == "1"
	// The epoch means "never".
	if st.LastUpdateEnd != nil && st.LastUpdateEnd.Year() <= 1970 {
		st.LastUpdateEnd = nil
	}
	if st.LastInitEnd != nil && st.LastInitEnd.Year() <= 1970 {
		st.LastInitEnd = nil
	}
	return a
}

func replicaOf(c *ldap.Conn, suffix string) (*Replica, error) {
	r, err := GetReplica(c, suffix)
	if err != nil {
		return nil, err
	}
	if !r.Enabled {
		return nil, fmt.Errorf("%w: %s", ErrNotReplicated, suffix)
	}
	return r, nil
}

// ListAgreements returns the agreements of a suffix with their status.
func ListAgreements(c *ldap.Conn, suffix string) ([]Agreement, error) {
	r, err := replicaOf(c, suffix)
	if err != nil {
		return nil, err
	}
	res, err := c.Search(ldap.NewSearchRequest(r.DN, ldap.ScopeSingleLevel, ldap.NeverDerefAliases, 0, 0, false,
		"(objectClass=nsds5ReplicationAgreement)", agreementAttrs, nil))
	if err != nil {
		return nil, err
	}
	out := make([]Agreement, 0, len(res.Entries))
	for _, e := range res.Entries {
		out = append(out, agreementFromEntry(e))
	}
	slices.SortFunc(out, func(a, b Agreement) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

func getAgreement(c *ldap.Conn, suffix, name string) (*Agreement, error) {
	list, err := ListAgreements(c, suffix)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if strings.EqualFold(list[i].Name, name) {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("%w: agreement %s", ErrNotFound, name)
}

var (
	hostRE     = regexp.MustCompile(`^[A-Za-z0-9.:\[\]-]+$`)
	scheduleRE = regexp.MustCompile(`^([0-2][0-9][0-5][0-9]-[0-2][0-9][0-5][0-9]) ([0-6]{1,7})$`)
)

func agreementChanges(in AgreementInput, creating bool) (map[string][]string, error) {
	set := map[string][]string{}
	if in.Host != "" || creating {
		if !hostRE.MatchString(in.Host) {
			return nil, fmt.Errorf("%w: host %q", ErrInvalidValue, in.Host)
		}
		set["nsDS5ReplicaHost"] = []string{in.Host}
	}
	if in.Port != 0 || creating {
		if in.Port < 1 || in.Port > 65535 {
			return nil, fmt.Errorf("%w: port %d", ErrInvalidValue, in.Port)
		}
		set["nsDS5ReplicaPort"] = []string{strconv.Itoa(in.Port)}
	}
	if in.Transport != "" || creating {
		t := in.Transport
		if t == "" {
			t = "LDAP"
		}
		v, ok := transports[t]
		if !ok {
			return nil, fmt.Errorf("%w: transport %q", ErrInvalidValue, t)
		}
		set["nsDS5ReplicaTransportInfo"] = []string{v}
	}
	if in.BindMethod != "" || creating {
		m := in.BindMethod
		if m == "" {
			m = "SIMPLE"
		}
		if !slices.Contains([]string{"SIMPLE", "SSLCLIENTAUTH", "SASL/GSSAPI", "SASL/DIGEST-MD5"}, m) {
			return nil, fmt.Errorf("%w: bind method %q", ErrInvalidValue, m)
		}
		set["nsDS5ReplicaBindMethod"] = []string{m}
	}
	if in.BindDN != "" {
		if _, err := ldap.ParseDN(in.BindDN); err != nil {
			return nil, fmt.Errorf("%w: bind DN %q", ErrInvalidValue, in.BindDN)
		}
		set["nsDS5ReplicaBindDN"] = []string{in.BindDN}
	}
	if in.Credentials != "" {
		set["nsDS5ReplicaCredentials"] = []string{in.Credentials}
	}
	if in.Enabled != nil {
		set["nsds5ReplicaEnabled"] = []string{map[bool]string{true: "on", false: "off"}[*in.Enabled]}
	}
	if in.Schedule != nil {
		s := strings.TrimSpace(*in.Schedule)
		if s != "" && !scheduleRE.MatchString(s) {
			return nil, fmt.Errorf("%w: schedule %q (HHMM-HHMM days, days 0=Sunday … 6)", ErrInvalidValue, s)
		}
		set["nsDS5ReplicaUpdateSchedule"] = nonEmpty(s)
	}
	if in.Excluded != nil {
		var attrs []string
		for _, a := range in.Excluded {
			if a = strings.TrimSpace(a); a == "" {
				continue
			}
			if !attrRE.MatchString(a) {
				return nil, fmt.Errorf("%w: attribute %q", ErrInvalidValue, a)
			}
			attrs = append(attrs, a)
		}
		if len(attrs) > 0 {
			set["nsDS5ReplicatedAttributeList"] = []string{excludePrefix + " " + strings.Join(attrs, " ")}
		} else {
			set["nsDS5ReplicatedAttributeList"] = []string{}
		}
	}
	if in.Description != nil {
		set["description"] = nonEmpty(*in.Description)
	}
	return set, nil
}

// CreateAgreement adds an agreement to a supplier or hub replica.
func CreateAgreement(c *ldap.Conn, suffix string, in AgreementInput) (*Agreement, error) {
	r, err := replicaOf(c, suffix)
	if err != nil {
		return nil, err
	}
	if r.Role == RoleConsumer {
		return nil, fmt.Errorf("%w: a consumer does not send changes", ErrInvalidValue)
	}
	in.Name = strings.TrimSpace(in.Name)
	if !itemName(in.Name) {
		return nil, fmt.Errorf("%w: agreement name %q", ErrInvalidName, in.Name)
	}
	set, err := agreementChanges(in, true)
	if err != nil {
		return nil, err
	}
	if set["nsDS5ReplicaBindMethod"][0] == "SIMPLE" && (in.BindDN == "" || in.Credentials == "") {
		return nil, fmt.Errorf("%w: SIMPLE binds need a bind DN and a password", ErrInvalidValue)
	}
	attrs := map[string][]string{"objectClass": {"top", "nsds5ReplicationAgreement"}, "cn": {in.Name}, "nsDS5ReplicaRoot": {suffix}}
	for k, v := range set {
		if len(v) > 0 {
			attrs[k] = v
		}
	}
	if err := addEntry(c, "cn="+ldap.EscapeDN(in.Name)+","+r.DN, attrs); err != nil {
		return nil, err
	}
	return getAgreement(c, suffix, in.Name)
}

// UpdateAgreement changes an agreement.
func UpdateAgreement(c *ldap.Conn, suffix, name string, in AgreementInput) (*Agreement, error) {
	a, err := getAgreement(c, suffix, name)
	if err != nil {
		return nil, err
	}
	set, err := agreementChanges(in, false)
	if err != nil {
		return nil, err
	}
	req := ldap.NewModifyRequest(a.DN, nil)
	for k, v := range set {
		req.Replace(k, v)
	}
	if len(req.Changes) > 0 {
		if err := c.Modify(req); err != nil {
			return nil, err
		}
	}
	return getAgreement(c, suffix, name)
}

// DeleteAgreement removes an agreement.
func DeleteAgreement(c *ldap.Conn, suffix, name string) error {
	a, err := getAgreement(c, suffix, name)
	if err != nil {
		return err
	}
	return c.Del(ldap.NewDelRequest(a.DN, nil))
}

// InitAgreement starts a total update of the consumer from this supplier.
func InitAgreement(c *ldap.Conn, suffix, name string) (*Agreement, error) {
	a, err := getAgreement(c, suffix, name)
	if err != nil {
		return nil, err
	}
	req := ldap.NewModifyRequest(a.DN, nil)
	req.Replace("nsds5BeginReplicaRefresh", []string{"start"})
	if err := c.Modify(req); err != nil {
		return nil, err
	}
	return getAgreement(c, suffix, name)
}

// --- RUV -------------------------------------------------------------------------

// RUVElement is one replica in the replica update vector.
type RUVElement struct {
	ReplicaID int        `json:"replicaId"`
	URL       string     `json:"url,omitempty"`
	MinCSN    string     `json:"minCSN,omitempty"`
	MaxCSN    string     `json:"maxCSN,omitempty"`
	MaxTime   *time.Time `json:"maxTime,omitempty" doc:"Time of the latest change from this replica"`
}

// RUV is the replica update vector of a suffix on this server.
type RUV struct {
	Generation string       `json:"generation,omitempty"`
	Replicas   []RUVElement `json:"replicas"`
}

var ruvRE = regexp.MustCompile(`^\{replica (\d+)(?: ([^}]*))?\}(?:\s+(\S+))?(?:\s+(\S+))?`)

// csnTime is the timestamp in the first 8 hex digits of a CSN.
func csnTime(csn string) *time.Time {
	if len(csn) < 8 {
		return nil
	}
	n, err := strconv.ParseInt(csn[:8], 16, 64)
	if err != nil || n == 0 {
		return nil
	}
	t := time.Unix(n, 0).UTC()
	return &t
}

// ParseRUV reads nsds50ruv values.
func ParseRUV(values []string) RUV {
	ruv := RUV{Replicas: []RUVElement{}}
	for _, v := range values {
		if g, ok := strings.CutPrefix(v, "{replicageneration} "); ok {
			ruv.Generation = strings.TrimSpace(g)
			continue
		}
		m := ruvRE.FindStringSubmatch(v)
		if m == nil {
			continue
		}
		id, _ := strconv.Atoi(m[1])
		el := RUVElement{ReplicaID: id, URL: m[2], MinCSN: m[3], MaxCSN: m[4]}
		el.MaxTime = csnTime(el.MaxCSN)
		ruv.Replicas = append(ruv.Replicas, el)
	}
	slices.SortFunc(ruv.Replicas, func(a, b RUVElement) int { return a.ReplicaID - b.ReplicaID })
	return ruv
}

// GetRUV reads the RUV of a suffix from the replica entry, which holds the
// live vector; the copy in the RUV tombstone of the suffix is written to the
// database only from time to time.
func GetRUV(c *ldap.Conn, suffix string) (*RUV, error) {
	r, err := replicaOf(c, suffix)
	if err != nil {
		return nil, err
	}
	res, err := c.Search(ldap.NewSearchRequest(r.DN, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"nsds50ruv"}, nil))
	if err != nil {
		return nil, err
	}
	var values []string
	if len(res.Entries) > 0 {
		values = res.Entries[0].GetAttributeValues("nsds50ruv")
	}
	ruv := ParseRUV(values)
	return &ruv, nil
}

// CleanAllRUVAttrs builds the task entry that removes a retired replica ID
// from the RUVs of the whole topology.
func CleanAllRUVAttrs(c *ldap.Conn, suffix string, rid int, force bool) (map[string][]string, error) {
	r, err := replicaOf(c, suffix)
	if err != nil {
		return nil, err
	}
	if rid < 1 || rid > 65534 {
		return nil, fmt.Errorf("%w: replica ID %d", ErrInvalidValue, rid)
	}
	if r.Role == RoleSupplier && r.ReplicaID == rid {
		return nil, fmt.Errorf("%w: %d is this server's own replica ID", ErrInvalidValue, rid)
	}
	return map[string][]string{
		"replica-base-dn":        {suffix},
		"replica-id":             {strconv.Itoa(rid)},
		"replica-force-cleaning": {map[bool]string{true: "yes", false: "no"}[force]},
	}, nil
}

// --- conflicts --------------------------------------------------------------------

// Conflict is an entry the replication had to rename or hide because two
// suppliers made incompatible changes.
type Conflict struct {
	DN     string `json:"dn"`
	Reason string `json:"reason" doc:"nsds5ReplConflict"`
	Glue   bool   `json:"glue" doc:"A glue entry created for orphaned children"`
}

// ListConflicts finds conflict and glue entries below a suffix.
func ListConflicts(c *ldap.Conn, suffix string) ([]Conflict, error) {
	res, err := c.Search(ldap.NewSearchRequest(suffix, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 1000, 15, false,
		"(|(&(objectClass=ldapSubEntry)(nsds5ReplConflict=*))(nsds5ReplConflict=*)(&(objectClass=ldapSubEntry)(objectClass=glue))(objectClass=glue))",
		[]string{"nsds5ReplConflict", "objectClass"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return []Conflict{}, nil
	}
	if err != nil && !ldap.IsErrorWithCode(err, ldap.LDAPResultSizeLimitExceeded) {
		return nil, err
	}
	out := []Conflict{}
	if res != nil {
		for _, e := range res.Entries {
			out = append(out, Conflict{DN: e.DN, Reason: e.GetAttributeValue("nsds5ReplConflict"), Glue: hasClass(e, "glue")})
		}
	}
	return out, nil
}

// DeleteConflict removes a conflict entry.
func DeleteConflict(c *ldap.Conn, dn string) error {
	if err := isConflict(c, dn); err != nil {
		return err
	}
	return c.Del(ldap.NewDelRequest(dn, nil))
}

// ConvertConflict keeps a conflict entry as a regular entry under a new RDN.
func ConvertConflict(c *ldap.Conn, dn, newRDN string) error {
	if err := isConflict(c, dn); err != nil {
		return err
	}
	r, err := ldap.ParseDN(newRDN)
	if err != nil || len(r.RDNs) != 1 {
		return fmt.Errorf("%w: new RDN %q", ErrInvalidValue, newRDN)
	}
	if err := c.ModifyDN(ldap.NewModifyDNRequest(dn, newRDN, true, "")); err != nil {
		return err
	}
	parent, err := parentDN(dn)
	if err != nil {
		return err
	}
	req := ldap.NewModifyRequest(newRDN+","+parent, nil)
	req.Delete("nsds5ReplConflict", nil)
	return c.Modify(req)
}

func isConflict(c *ldap.Conn, dn string) error {
	res, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(|(nsds5ReplConflict=*)(&(objectClass=ldapSubEntry)(nsds5ReplConflict=*)))", []string{"1.1"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) || err == nil && len(res.Entries) == 0 {
		return fmt.Errorf("%w: conflict entry %s", ErrNotFound, dn)
	}
	return err
}
