package ds

import (
	"context"

	"github.com/go-ldap/ldap/v3"
)

// DNRSA holds the server certificate setting (nsSSLPersonalitySSL).
const DNRSA = "cn=RSA,cn=encryption,cn=config"

// ServerCertName returns the nickname of the server certificate, or "" when
// the encryption entry does not exist.
func ServerCertName(c *ldap.Conn) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest(DNRSA, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", []string{"nsSSLPersonalitySSL"}, nil))
	if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if len(res.Entries) == 0 {
		return "", nil
	}
	return res.Entries[0].GetAttributeValue("nsSSLPersonalitySSL"), nil
}

// SetServerCertName makes nick the server certificate; the server uses it
// after a restart.
func SetServerCertName(c *ldap.Conn, nick string) error {
	req := ldap.NewModifyRequest(DNRSA, nil)
	req.Replace("nsSSLPersonalitySSL", []string{nick})
	return c.Modify(req)
}

// ServerCert reads and sets the server certificate over LDAP.
type ServerCert struct{ LDAP Doer }

// ServerCert returns the nickname of the server certificate.
func (s ServerCert) ServerCert(ctx context.Context) (name string, err error) {
	err = s.LDAP.Do(ctx, func(c *ldap.Conn) error { name, err = ServerCertName(c); return err })
	return name, err
}

// SetServerCert makes nick the server certificate.
func (s ServerCert) SetServerCert(ctx context.Context, nick string) error {
	return s.LDAP.Do(ctx, func(c *ldap.Conn) error { return SetServerCertName(c, nick) })
}

// Restarter restarts the server process (a providers.Lifecycle).
type Restarter interface {
	Restart(ctx context.Context) error
}

// RestartServer restarts the server, records it in the history and, on
// success, forgets the pending restart reasons.
func RestartServer(ctx context.Context, lc Restarter, tasks *Tasks, reg *RestartRegistry) error {
	err := lc.Restart(ctx)
	tasks.Record(ctx, OpRestart, "", err)
	if err == nil && reg != nil {
		reg.Clear()
	}
	return err
}
