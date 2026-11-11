package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/ldapx"
)

// RootPasswordResult reports a root DN password change.
type RootPasswordResult struct {
	RootDN string `json:"rootDN"`
	// ds-ui binds as the root DN with a password (remote mode): it now uses
	// the new one until restarted, so its secret must be updated too.
	CredentialsUpdated bool `json:"credentialsUpdated" doc:"ds-ui binds with this password; update its secret before restarting it"`
}

type rootPasswordInput struct {
	Body struct {
		Password string `json:"password" minLength:"8"`
	}
}
type rootPasswordOutput struct{ Body RootPasswordResult }

type saslMappingsOutput struct{ Body []ds.SASLMapping }
type saslMappingInput struct{ Body ds.SASLMapping }
type saslMappingPath struct {
	Name string `path:"name"`
}
type saslMappingUpdateInput struct {
	Name string `path:"name"`
	Body ds.SASLMapping
}

// rebindable is implemented by ldapx.Client; tests may use plain LDAP stubs.
type rebindable interface {
	BindDN() string
	SetBindPassword(string)
}

func registerSecurity(api huma.API, d Deps) {
	tags := []string{"security"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "setRootPassword", Method: http.MethodPost, Path: apiPrefix + "/root-password",
		Summary: "Change the Directory Manager password", Tags: tags},
		func(ctx context.Context, in *rootPasswordInput) (*rootPasswordOutput, error) {
			var out RootPasswordResult
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				if out.RootDN, err = ds.RootDN(c); err != nil {
					return err
				}
				return ds.SetRootPassword(c, in.Body.Password)
			})
			if d.Tasks != nil {
				d.Tasks.Record(ctx, ds.OpRootPassword, out.RootDN, err)
			}
			if err != nil {
				return nil, toHTTPError(err)
			}
			if rb, ok := d.LDAP.(rebindable); ok && rb.BindDN() != "" && ldapx.NormDN(rb.BindDN()) == ldapx.NormDN(out.RootDN) {
				rb.SetBindPassword(in.Body.Password)
				out.CredentialsUpdated = true
			}
			return &rootPasswordOutput{Body: out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listSASLMappings", Method: http.MethodGet, Path: apiPrefix + "/sasl-mappings",
		Summary: "SASL identity mappings, in the order they are tried", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*saslMappingsOutput, error) {
			var list []ds.SASLMapping
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListSASLMappings(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &saslMappingsOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "createSASLMapping", Method: http.MethodPost, Path: apiPrefix + "/sasl-mappings",
		Summary: "Add a SASL mapping", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *saslMappingInput) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.CreateSASLMapping(c, in.Body) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "updateSASLMapping", Method: http.MethodPut, Path: apiPrefix + "/sasl-mappings/{name}",
		Summary: "Change a SASL mapping", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *saslMappingUpdateInput) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.UpdateSASLMapping(c, in.Name, in.Body) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteSASLMapping", Method: http.MethodDelete, Path: apiPrefix + "/sasl-mappings/{name}",
		Summary: "Delete a SASL mapping", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *saslMappingPath) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.DeleteSASLMapping(c, in.Name) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})
}
