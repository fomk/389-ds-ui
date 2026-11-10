package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type localPoliciesOutput struct{ Body []ds.LocalPolicy }
type localPolicyInput struct{ Body ds.NewLocalPolicy }
type localPolicyCreatedOutput struct{ Body *ds.LocalPolicyCreated }
type localPolicyDNInput struct {
	DN string `query:"dn" required:"true" doc:"The passwordPolicy entry"`
}

func registerPwPolicies(api huma.API, d Deps) {
	tags := []string{"password policies"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "listLocalPolicies", Method: http.MethodGet, Path: apiPrefix + "/pwpolicies",
		Summary: "Subtree and user password policies", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*localPoliciesOutput, error) {
			var list []ds.LocalPolicy
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListLocalPolicies(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &localPoliciesOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "createLocalPolicy", Method: http.MethodPost, Path: apiPrefix + "/pwpolicies",
		Summary: "Create a subtree or user password policy", Description: "Turns on nsslapd-pwpolicy-local if it is off.",
		Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *localPolicyInput) (*localPolicyCreatedOutput, error) {
			var out *ds.LocalPolicyCreated
			err := conn(ctx, func(c *ldap.Conn) (err error) { out, err = ds.CreateLocalPolicy(c, in.Body); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &localPolicyCreatedOutput{Body: out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteLocalPolicy", Method: http.MethodDelete, Path: apiPrefix + "/pwpolicies",
		Summary: "Delete a subtree or user password policy", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *localPolicyDNInput) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.DeleteLocalPolicy(c, in.DN) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})
}
