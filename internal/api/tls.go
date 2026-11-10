package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// CertList is the NSS database with the certificate the server uses.
type CertList struct {
	Certificates []certs.Certificate `json:"certificates"`
	ServerCert   string              `json:"serverCert" doc:"nsSSLPersonalitySSL in cn=RSA,cn=encryption,cn=config"`
}
type certListOutput struct{ Body CertList }
type certPath struct {
	Nickname string `path:"nickname"`
}
type certOutput struct{ Body *certs.Certificate }
type certPEMInput struct {
	Body struct {
		Nickname string `json:"nickname" minLength:"1"`
		PEM      string `json:"pem" minLength:"1" maxLength:"1048576"`
	}
}
type pkcs12Input struct {
	Body struct {
		Data     []byte `json:"data" doc:"The PKCS#12 file (base64 in JSON)" maxLength:"1048576"`
		Password string `json:"password"`
	}
}
type pemPairInput struct {
	Body struct {
		Nickname string `json:"nickname" minLength:"1"`
		Cert     string `json:"cert" minLength:"1" maxLength:"1048576"`
		Key      string `json:"key" minLength:"1" maxLength:"1048576"`
		Chain    string `json:"chain,omitempty" maxLength:"1048576"`
	}
}

// Imported lists the nicknames an import added.
type Imported struct {
	Nicknames []string `json:"nicknames"`
}
type importedOutput struct{ Body Imported }
type csrInput struct{ Body certs.CSRRequest }

// CSR is a PEM certificate request.
type CSR struct {
	PEM string `json:"pem"`
}
type csrOutput struct{ Body CSR }
type trustInput struct {
	Nickname string `path:"nickname"`
	Body     struct {
		Trust string `json:"trust" doc:"e.g. CT,, (trusted CA) or ,, (none)"`
	}
}
type serverCertInput struct {
	Body struct {
		Nickname string `json:"nickname" minLength:"1"`
	}
}
type presentedOutput struct{ Body *certs.Presented }

const dnRSA = "cn=RSA,cn=encryption,cn=config"

func serverCertName(c *ldap.Conn) (string, error) {
	res, err := c.Search(ldap.NewSearchRequest(dnRSA, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
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

func registerTLS(api huma.API, d Deps) {
	tags := []string{"tls"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }
	store := func() (*certs.Store, error) {
		if d.Certs == nil {
			return nil, toHTTPError(fmt.Errorf("%w: the NSS database of the server is not reachable from ds-ui (remote mode)", providers.ErrNotSupported))
		}
		return d.Certs, nil
	}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: apiPrefix + path, Summary: summary, Tags: tags, DefaultStatus: status}
	}
	empty := func(err error) (*noContent, error) {
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &noContent{}, nil
	}

	huma.Register(api, op("listCerts", http.MethodGet, "/security/certs", "Certificates of the server's NSS database", 0),
		func(ctx context.Context, _ *struct{}) (*certListOutput, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			var out CertList
			if out.Certificates, err = s.List(ctx); err != nil {
				return nil, toHTTPError(err)
			}
			for i := range out.Certificates {
				out.Certificates[i].PEM = ""
			}
			if err := conn(ctx, func(c *ldap.Conn) (err error) { out.ServerCert, err = serverCertName(c); return }); err != nil {
				return nil, toHTTPError(err)
			}
			return &certListOutput{Body: out}, nil
		})
	huma.Register(api, op("getCert", http.MethodGet, "/security/certs/{nickname}", "One certificate with its PEM", 0),
		func(ctx context.Context, in *certPath) (*certOutput, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			c, err := s.Get(ctx, in.Nickname)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &certOutput{Body: c}, nil
		})
	huma.Register(api, op("importCA", http.MethodPost, "/security/certs/ca", "Add a trusted CA certificate", http.StatusNoContent),
		func(ctx context.Context, in *certPEMInput) (*noContent, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			return empty(s.ImportCA(ctx, in.Body.Nickname, in.Body.PEM))
		})
	huma.Register(api, op("importIssued", http.MethodPost, "/security/certs/issued", "Add the certificate issued for a request made here", http.StatusNoContent),
		func(ctx context.Context, in *certPEMInput) (*noContent, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			return empty(s.ImportIssued(ctx, in.Body.Nickname, in.Body.PEM))
		})
	huma.Register(api, op("importPKCS12", http.MethodPost, "/security/certs/pkcs12", "Import a certificate with its key from PKCS#12", 0),
		func(ctx context.Context, in *pkcs12Input) (*importedOutput, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			added, err := s.ImportPKCS12(ctx, in.Body.Data, in.Body.Password)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &importedOutput{Body: Imported{Nicknames: nonNil(added)}}, nil
		})
	huma.Register(api, op("importPEMPair", http.MethodPost, "/security/certs/pem-pair", "Import a PEM certificate and private key", 0),
		func(ctx context.Context, in *pemPairInput) (*importedOutput, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			added, err := s.ImportPEMPair(ctx, in.Body.Nickname, in.Body.Cert, in.Body.Key, in.Body.Chain)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &importedOutput{Body: Imported{Nicknames: nonNil(added)}}, nil
		})
	huma.Register(api, op("createCSR", http.MethodPost, "/security/csr", "Generate a key and a certificate request", 0),
		func(ctx context.Context, in *csrInput) (*csrOutput, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			pem, err := s.CreateCSR(ctx, in.Body)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &csrOutput{Body: CSR{PEM: pem}}, nil
		})
	huma.Register(api, op("setCertTrust", http.MethodPatch, "/security/certs/{nickname}", "Change trust flags", http.StatusNoContent),
		func(ctx context.Context, in *trustInput) (*noContent, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			return empty(s.SetTrust(ctx, in.Nickname, in.Body.Trust))
		})
	huma.Register(api, op("deleteCert", http.MethodDelete, "/security/certs/{nickname}", "Delete a certificate (and its key)", http.StatusNoContent),
		func(ctx context.Context, in *certPath) (*noContent, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			var current string
			if err := conn(ctx, func(c *ldap.Conn) (err error) { current, err = serverCertName(c); return }); err != nil {
				return nil, toHTTPError(err)
			}
			if current == in.Nickname {
				return nil, toHTTPError(fmt.Errorf("%w: %s is the server certificate; choose another one first", certs.ErrInUse, in.Nickname))
			}
			return empty(s.Delete(ctx, in.Nickname))
		})
	huma.Register(api, op("setServerCert", http.MethodPut, "/security/server-cert", "Use a certificate with a key as the server certificate (after restart)", http.StatusNoContent),
		func(ctx context.Context, in *serverCertInput) (*noContent, error) {
			s, err := store()
			if err != nil {
				return nil, err
			}
			c, err := s.Get(ctx, in.Body.Nickname)
			if err != nil {
				return nil, toHTTPError(err)
			}
			if !c.HasKey {
				return nil, toHTTPError(fmt.Errorf("%w: %s has no private key in the database", certs.ErrInvalid, c.Nickname))
			}
			var before string
			err = conn(ctx, func(lc *ldap.Conn) error {
				var err error
				if before, err = serverCertName(lc); err != nil {
					return err
				}
				req := ldap.NewModifyRequest(dnRSA, nil)
				req.Replace("nsSSLPersonalitySSL", []string{c.Nickname})
				return lc.Modify(req)
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			if d.Restart != nil {
				d.Restart.Change("nsSSLPersonalitySSL", before, c.Nickname)
			}
			return &noContent{}, nil
		})
	huma.Register(api, op("presentedCert", http.MethodGet, "/security/presented", "Certificate the server presents on its LDAPS port (TLS handshake)", 0),
		func(ctx context.Context, _ *struct{}) (*presentedOutput, error) {
			var port string
			var enabled bool
			err := conn(ctx, func(c *ldap.Conn) error {
				res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
					"(objectClass=*)", []string{"nsslapd-securePort", "nsslapd-security"}, nil))
				if err != nil {
					return err
				}
				port = res.Entries[0].GetAttributeValue("nsslapd-securePort")
				enabled = strings.EqualFold(res.Entries[0].GetAttributeValue("nsslapd-security"), "on")
				return nil
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			if !enabled || port == "" || port == "0" {
				return nil, problem(http.StatusConflict, "LDAPS is off (nsslapd-security)")
			}
			p, err := certs.Handshake(ctx, net.JoinHostPort(d.TLSHost, port))
			if err != nil {
				return nil, problem(http.StatusBadGateway, err.Error())
			}
			return &presentedOutput{Body: p}, nil
		})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
