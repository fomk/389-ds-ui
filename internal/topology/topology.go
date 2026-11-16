// Package topology builds a replication report across servers: starting at
// the local server it follows the replication agreements to the consumers,
// reads their replicas, agreements and RUVs, and works out how far each
// server is behind each supplier. Credentials for the other servers come
// with the request and are not kept.
package topology

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

// MaxNodes caps a report.
const MaxNodes = 20

// BindCredentials bind to another server.
type BindCredentials struct {
	BindDN   string `json:"bindDN"`
	Password string `json:"password"`
}

// HostCredentials are credentials for one server (host:port as in the
// agreements).
type HostCredentials struct {
	Address string `json:"address" doc:"host:port as the agreements name it"`
	BindCredentials
}

// TopologyRequest asks for a report.
type TopologyRequest struct {
	Suffix      string            `json:"suffix" minLength:"1"`
	Default     BindCredentials   `json:"default" doc:"Used for every server without its own credentials"`
	Hosts       []HostCredentials `json:"hosts,omitempty"`
	InsecureTLS bool              `json:"insecureTLS,omitempty" doc:"Do not verify the certificates of the other servers"`
}

// TopologyLink is an agreement of a node.
type TopologyLink struct {
	Name          string     `json:"name"`
	Target        string     `json:"target" doc:"host:port"`
	Transport     string     `json:"transport"`
	Enabled       bool       `json:"enabled"`
	State         string     `json:"state"`
	LastUpdate    string     `json:"lastUpdate,omitempty"`
	LastUpdateEnd *time.Time `json:"lastUpdateEnd,omitempty"`
}

// SupplierLag is how far a node is behind one supplier.
type SupplierLag struct {
	ReplicaID int `json:"replicaId"`
	// Seconds behind the newest change any server has from this supplier;
	// -1: the node has no change from it at all.
	Seconds int64 `json:"seconds"`
}

// TopologyNode is one server of the topology.
type TopologyNode struct {
	Address     string          `json:"address" doc:"host:port it was reached at; the local server: its own name"`
	Aliases     []string        `json:"aliases" doc:"Other addresses agreements use for it"`
	Self        bool            `json:"self"`
	Reachable   bool            `json:"reachable"`
	Error       string          `json:"error,omitempty"`
	ErrorKind   string          `json:"errorKind,omitempty" enum:"connect,tls,auth,not-replicated,other"`
	ReplicaID   int             `json:"replicaId,omitempty"`
	Role        string          `json:"role,omitempty"`
	Agreements  []TopologyLink  `json:"agreements"`
	RUV         []ds.RUVElement `json:"ruv"`
	SupplierLag []SupplierLag   `json:"behind" doc:"Per supplier"`
	MaxBehind   int64           `json:"maxBehind" doc:"Seconds; -1 when some supplier's changes are missing entirely"`
	id          string          // nsUniqueId of the replica entry
}

// TopologyReport is the topology of one suffix.
type TopologyReport struct {
	Suffix    string         `json:"suffix"`
	Nodes     []TopologyNode `json:"nodes"`
	Suppliers []int          `json:"suppliers" doc:"Replica IDs that originated changes"`
	Plain     []string       `json:"plain" doc:"Servers reached over plain LDAP: the password crossed the network unencrypted"`
	Truncated bool           `json:"truncated"`
	Started   time.Time      `json:"started"`
}

// Doer runs a function with the local LDAP connection.
type Doer interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// Dialer opens a connection to another server as an agreement describes it.
type Dialer func(ctx context.Context, host string, port int, transport string, insecure bool) (*ldap.Conn, error)

// DefaultDialer connects over LDAP, StartTLS or LDAPS, verifying
// certificates against the system roots and caFile (if set) unless insecure.
func DefaultDialer(caFile string) Dialer {
	return func(ctx context.Context, host string, port int, transport string, insecure bool) (*ldap.Conn, error) {
		cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure} //nolint:gosec // only on explicit request
		if caFile != "" && !insecure {
			pool, err := x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
			pem, err := os.ReadFile(caFile)
			if err != nil {
				return nil, err
			}
			pool.AppendCertsFromPEM(pem)
			cfg.RootCAs = pool
		}
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		nd := &net.Dialer{Timeout: 10 * time.Second}
		opts := []ldap.DialOpt{ldap.DialWithDialer(nd)}
		scheme := "ldap://"
		if transport == "LDAPS" {
			scheme = "ldaps://"
			opts = append(opts, ldap.DialWithTLSConfig(cfg))
		}
		c, err := ldap.DialURL(scheme+addr, opts...)
		if err != nil {
			return nil, err
		}
		c.SetTimeout(15 * time.Second)
		if transport == "StartTLS" {
			if err := c.StartTLS(cfg); err != nil {
				_ = c.Close()
				return nil, err
			}
		}
		return c, nil
	}
}

type target struct {
	addr, host, transport string
	port                  int
}

// Build makes the report for req.Suffix starting at the local server.
func Build(ctx context.Context, local Doer, req TopologyRequest, dial Dialer) (*TopologyReport, error) {
	if _, err := ldap.ParseDN(req.Suffix); err != nil || req.Suffix == "" {
		return nil, fmt.Errorf("%w: suffix %q", ds.ErrInvalidValue, req.Suffix)
	}
	rep := &TopologyReport{Suffix: req.Suffix, Nodes: []TopologyNode{}, Suppliers: []int{}, Plain: []string{}, Started: time.Now().UTC()}
	creds := map[string]BindCredentials{}
	for _, h := range req.Hosts {
		creds[strings.ToLower(h.Address)] = h.BindCredentials
	}

	var self TopologyNode
	err := local.Do(ctx, func(c *ldap.Conn) error {
		self = readNode(c, req.Suffix)
		addr, err := localAddress(c)
		self.Address = addr
		return err
	})
	if err != nil {
		return nil, err
	}
	if self.ErrorKind == "not-replicated" {
		return nil, fmt.Errorf("%w: %s", ds.ErrNotReplicated, req.Suffix)
	}
	self.Self, self.Reachable = true, self.Error == ""
	rep.Nodes = append(rep.Nodes, self)

	visited := map[string]bool{strings.ToLower(self.Address): true}
	queue := targets(self)
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		key := strings.ToLower(t.addr)
		if visited[key] {
			continue
		}
		visited[key] = true
		if len(rep.Nodes) >= MaxNodes {
			rep.Truncated = true
			break
		}
		n := TopologyNode{Address: t.addr, Aliases: []string{}, Agreements: []TopologyLink{}, RUV: []ds.RUVElement{}}
		cr, ok := creds[key]
		if !ok {
			cr = req.Default
		}
		c, err := dial(ctx, t.host, t.port, t.transport, req.InsecureTLS)
		if err != nil {
			n.Error, n.ErrorKind = err.Error(), connectKind(err)
			rep.Nodes = append(rep.Nodes, n)
			continue
		}
		if t.transport == "LDAP" {
			rep.Plain = append(rep.Plain, t.addr)
		}
		if err := c.Bind(cr.BindDN, cr.Password); err != nil {
			_ = c.Close()
			n.Error, n.ErrorKind = err.Error(), "auth"
			if !ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) && !ldap.IsErrorWithCode(err, ldap.LDAPResultInappropriateAuthentication) {
				n.ErrorKind = "other"
			}
			rep.Nodes = append(rep.Nodes, n)
			continue
		}
		read := readNode(c, req.Suffix)
		_ = c.Close()
		read.Address = t.addr
		// The same server under another name (e.g. the local one as its
		// network alias): one node with aliases.
		if i := slices.IndexFunc(rep.Nodes, func(x TopologyNode) bool { return x.id != "" && x.id == read.id }); i >= 0 {
			rep.Nodes[i].Aliases = append(rep.Nodes[i].Aliases, t.addr)
			continue
		}
		read.Reachable = read.Error == ""
		rep.Nodes = append(rep.Nodes, read)
		queue = append(queue, targets(read)...)
	}
	computeLag(rep)
	return rep, nil
}

func targets(n TopologyNode) []target {
	var out []target
	for _, a := range n.Agreements {
		host, port, err := net.SplitHostPort(a.Target)
		if err != nil {
			continue
		}
		p, _ := strconv.Atoi(port)
		out = append(out, target{addr: a.Target, host: host, port: p, transport: a.Transport})
	}
	return out
}

// readNode reads the replica, its identity, agreements and RUV of suffix.
func readNode(c *ldap.Conn, suffix string) TopologyNode {
	n := TopologyNode{Aliases: []string{}, Agreements: []TopologyLink{}, RUV: []ds.RUVElement{}, SupplierLag: []SupplierLag{}}
	r, err := ds.GetReplica(c, suffix)
	if err != nil {
		n.Error, n.ErrorKind = err.Error(), "other"
		return n
	}
	if !r.Enabled {
		n.Error, n.ErrorKind = "replication is not enabled for "+suffix, "not-replicated"
		return n
	}
	n.ReplicaID, n.Role = r.ReplicaID, r.Role
	if res, err := c.Search(ldap.NewSearchRequest(r.DN, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"nsUniqueId"}, nil)); err == nil && len(res.Entries) == 1 {
		n.id = res.Entries[0].GetAttributeValue("nsUniqueId")
	}
	agmts, err := ds.ListAgreements(c, suffix)
	if err != nil {
		n.Error, n.ErrorKind = err.Error(), "other"
		return n
	}
	for _, a := range agmts {
		n.Agreements = append(n.Agreements, TopologyLink{Name: a.Name, Target: net.JoinHostPort(a.Host, strconv.Itoa(a.Port)),
			Transport: a.Transport, Enabled: a.Enabled, State: a.Status.State, LastUpdate: a.Status.LastUpdate, LastUpdateEnd: a.Status.LastUpdateEnd})
	}
	ruv, err := ds.GetRUV(c, suffix)
	if err != nil {
		n.Error, n.ErrorKind = err.Error(), "other"
		return n
	}
	n.RUV = append(n.RUV, ruv.Replicas...)
	return n
}

// localAddress is the name and port the local server calls itself.
func localAddress(c *ldap.Conn) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"nsslapd-localhost", "nsslapd-port"}, nil))
	if err != nil {
		return "", err
	}
	if len(res.Entries) == 0 {
		return "", errors.New("cn=config not readable")
	}
	e := res.Entries[0]
	return net.JoinHostPort(e.GetAttributeValue("nsslapd-localhost"), e.GetAttributeValue("nsslapd-port")), nil
}

func connectKind(err error) string {
	var certErr *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &unknown) || errors.As(err, &hostErr) || strings.Contains(err.Error(), "tls:") {
		return "tls"
	}
	return "connect"
}

// computeLag compares, for every supplier, the time of its newest change on
// each node with the newest one any node has.
func computeLag(rep *TopologyReport) {
	newest := map[int]time.Time{}
	for _, n := range rep.Nodes {
		for _, e := range n.RUV {
			if e.MaxTime != nil && e.MaxTime.After(newest[e.ReplicaID]) {
				newest[e.ReplicaID] = *e.MaxTime
			}
		}
	}
	for rid := range newest {
		rep.Suppliers = append(rep.Suppliers, rid)
	}
	slices.Sort(rep.Suppliers)
	for i := range rep.Nodes {
		n := &rep.Nodes[i]
		n.SupplierLag = []SupplierLag{}
		if !n.Reachable {
			continue
		}
		for _, rid := range rep.Suppliers {
			b := SupplierLag{ReplicaID: rid, Seconds: -1}
			for _, e := range n.RUV {
				if e.ReplicaID == rid && e.MaxTime != nil {
					b.Seconds = int64(newest[rid].Sub(*e.MaxTime).Seconds())
				}
			}
			n.SupplierLag = append(n.SupplierLag, b)
			switch {
			case b.Seconds < 0:
				n.MaxBehind = -1
			case n.MaxBehind >= 0 && b.Seconds > n.MaxBehind:
				n.MaxBehind = b.Seconds
			}
		}
	}
}
