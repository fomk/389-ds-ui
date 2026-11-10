package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type aciListInput struct {
	DN      string `query:"dn" required:"true"`
	Subtree bool   `query:"subtree" doc:"Include the entries below dn"`
}
type aciListOutput struct{ Body []ds.EntryACIs }
type aciDNInput struct {
	DN string `query:"dn" required:"true"`
}
type aciSetInput struct {
	DN   string `query:"dn" required:"true"`
	Body struct {
		ACIs []string `json:"acis" doc:"All ACIs of the entry; empty removes them"`
	}
}
type gerInput struct {
	Body struct {
		Target     string   `json:"target" minLength:"1"`
		Subject    string   `json:"subject" doc:"Bind DN to evaluate; empty for anonymous"`
		Attributes []string `json:"attributes,omitempty" doc:"Attributes the entry does not have yet"`
	}
}
type gerOutput struct{ Body *ds.EffectiveRights }

func registerACI(api huma.API, d Deps) {
	tags := []string{"access control"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "listACIs", Method: http.MethodGet, Path: apiPrefix + "/aci",
		Summary: "ACIs of an entry (and of the entries below it)", Tags: tags},
		func(ctx context.Context, in *aciListInput) (*aciListOutput, error) {
			var list []ds.EntryACIs
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListACIs(c, in.DN, in.Subtree); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &aciListOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listInheritedACIs", Method: http.MethodGet, Path: apiPrefix + "/aci/inherited",
		Summary: "ACIs of the ancestors of an entry, nearest first", Tags: tags},
		func(ctx context.Context, in *aciDNInput) (*aciListOutput, error) {
			var list []ds.EntryACIs
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.InheritedACIs(c, in.DN); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &aciListOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "setACIs", Method: http.MethodPut, Path: apiPrefix + "/aci",
		Summary: "Replace the ACIs of an entry (the server checks the syntax)", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *aciSetInput) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.SetACIs(c, in.DN, in.Body.ACIs) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "effectiveRights", Method: http.MethodPost, Path: apiPrefix + "/aci/effective-rights",
		Summary: "Rights a subject has on an entry (Get Effective Rights)", Tags: tags},
		func(ctx context.Context, in *gerInput) (*gerOutput, error) {
			var r *ds.EffectiveRights
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				r, err = ds.GetEffectiveRights(c, in.Body.Target, in.Body.Subject, in.Body.Attributes)
				return
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &gerOutput{Body: r}, nil
		})
}
