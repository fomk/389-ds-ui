package ds

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// DNPassThru is the LDAP Pass-Through Authentication plug-in. It is
// configured with plug-in arguments (nsslapd-pluginarg0, 1, …), one per
// remote server set:
//
//	ldap[s]://host1[:port] host2[:port]/subtree [maxconns,maxops,timeout,ldapver,connlifetime,starttls]
const DNPassThru = "cn=Pass Through Authentication," + DNPlugins

// PassThruServer is one argument: binds of DNs under Subtree are checked by
// the servers in Hosts.
type PassThruServer struct {
	Secure       bool     `json:"secure" doc:"ldaps://"`
	Hosts        []string `json:"hosts" minItems:"1" doc:"host or host:port; tried in order"`
	Subtree      string   `json:"subtree" minLength:"1" doc:"Bind DNs under it are passed through"`
	MaxConns     int      `json:"maxConns" minimum:"1" doc:"Connections to each server (default 3)"`
	MaxOps       int      `json:"maxOps" minimum:"1" doc:"Outstanding operations per connection (default 5)"`
	Timeout      int      `json:"timeout" minimum:"0" doc:"Seconds to wait for a server (default 300)"`
	LDAPVersion  int      `json:"ldapVersion" enum:"2,3" doc:"Protocol version (default 3)"`
	ConnLifetime int      `json:"connLifetime" minimum:"0" doc:"Seconds before a connection is reopened; 0: never (default 300)"`
	StartTLS     bool     `json:"startTLS" doc:"StartTLS on ldap:// connections"`
}

var (
	ptaOptionsRE = regexp.MustCompile(`^\d+,\d+,\d+,\d+,\d+(,\d+)?$`)
	ptaHostRE    = regexp.MustCompile(`^[A-Za-z0-9.\-]+(:\d{1,5})?$|^\[[0-9A-Fa-f:.]+\](:\d{1,5})?$`)
)

// ParsePassThru reads one plug-in argument.
func ParsePassThru(arg string) (PassThruServer, error) {
	s := PassThruServer{MaxConns: 3, MaxOps: 5, Timeout: 300, LDAPVersion: 3, ConnLifetime: 300}
	arg = strings.TrimSpace(arg)
	switch {
	case strings.HasPrefix(strings.ToLower(arg), "ldaps://"):
		s.Secure, arg = true, arg[len("ldaps://"):]
	case strings.HasPrefix(strings.ToLower(arg), "ldap://"):
		arg = arg[len("ldap://"):]
	default:
		return s, fmt.Errorf("%w: %q is not an ldap:// or ldaps:// URL", ErrInvalidValue, arg)
	}
	// The options are the last word when they look like numbers; the
	// subtree itself may contain spaces.
	if i := strings.LastIndexByte(arg, ' '); i > 0 && ptaOptionsRE.MatchString(arg[i+1:]) {
		n := strings.Split(arg[i+1:], ",")
		v := make([]int, len(n))
		for j := range n {
			v[j], _ = strconv.Atoi(n[j])
		}
		s.MaxConns, s.MaxOps, s.Timeout, s.LDAPVersion, s.ConnLifetime = v[0], v[1], v[2], v[3], v[4]
		if len(v) > 5 {
			s.StartTLS = v[5] != 0
		}
		arg = strings.TrimSpace(arg[:i])
	}
	slash := strings.IndexByte(arg, '/')
	if slash < 0 {
		return s, fmt.Errorf("%w: no subtree after the hosts", ErrInvalidValue)
	}
	s.Hosts = strings.Fields(arg[:slash])
	s.Subtree = strings.TrimSpace(arg[slash+1:])
	return s, s.validate()
}

func (s PassThruServer) validate() error {
	if len(s.Hosts) == 0 {
		return fmt.Errorf("%w: no host", ErrInvalidValue)
	}
	for _, h := range s.Hosts {
		if !ptaHostRE.MatchString(h) {
			return fmt.Errorf("%w: host %q", ErrInvalidValue, h)
		}
	}
	if _, err := ldap.ParseDN(s.Subtree); err != nil || s.Subtree == "" {
		return fmt.Errorf("%w: subtree %q", ErrInvalidValue, s.Subtree)
	}
	if s.StartTLS && s.Secure {
		return fmt.Errorf("%w: StartTLS applies to ldap://, not ldaps://", ErrInvalidValue)
	}
	if s.MaxConns < 1 || s.MaxOps < 1 || s.Timeout < 0 || s.ConnLifetime < 0 || (s.LDAPVersion != 2 && s.LDAPVersion != 3) {
		return fmt.Errorf("%w: connection options", ErrInvalidValue)
	}
	return nil
}

// String writes the plug-in argument.
func (s PassThruServer) String() string {
	scheme := "ldap://"
	if s.Secure {
		scheme = "ldaps://"
	}
	tls := 0
	if s.StartTLS {
		tls = 1
	}
	return fmt.Sprintf("%s%s/%s %d,%d,%d,%d,%d,%d", scheme, strings.Join(s.Hosts, " "), s.Subtree,
		s.MaxConns, s.MaxOps, s.Timeout, s.LDAPVersion, s.ConnLifetime, tls)
}

func pluginArgs(e *ldap.Entry) map[int]string {
	args := map[int]string{}
	for _, a := range e.Attributes {
		low := strings.ToLower(a.Name)
		if !strings.HasPrefix(low, "nsslapd-pluginarg") || len(a.Values) == 0 {
			continue
		}
		if n, err := strconv.Atoi(low[len("nsslapd-pluginarg"):]); err == nil {
			args[n] = a.Values[0]
		}
	}
	return args
}

// ListPassThru returns the configured servers in argument order. Arguments
// that cannot be read are reported, not dropped silently.
func ListPassThru(c *ldap.Conn) ([]PassThruServer, error) {
	e, err := readEntry(c, DNPassThru, "*")
	if err != nil {
		return nil, err
	}
	args := pluginArgs(e)
	keys := make([]int, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]PassThruServer, 0, len(keys))
	for _, k := range keys {
		s, err := ParsePassThru(args[k])
		if err != nil {
			return nil, fmt.Errorf("nsslapd-pluginarg%d: %w", k, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// SetPassThru replaces the configured servers. The plug-in reads its
// arguments when it starts.
func SetPassThru(c *ldap.Conn, servers []PassThruServer) error {
	for i, s := range servers {
		if err := s.validate(); err != nil {
			return fmt.Errorf("server %d: %w", i+1, err)
		}
	}
	e, err := readEntry(c, DNPassThru, "*")
	if err != nil {
		return err
	}
	old := pluginArgs(e)
	req := ldap.NewModifyRequest(DNPassThru, nil)
	for i, s := range servers {
		req.Replace("nsslapd-pluginarg"+strconv.Itoa(i), []string{s.String()})
	}
	for k := range old {
		if k >= len(servers) {
			req.Delete("nsslapd-pluginarg"+strconv.Itoa(k), nil)
		}
	}
	if len(req.Changes) == 0 {
		return nil
	}
	return c.Modify(req)
}
