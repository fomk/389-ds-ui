package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/pages"
)

type pageListOutput struct{ Body []pages.PageSummary }
type pagePath struct {
	ID string `path:"id" doc:"Page id"`
}
type pageOutput struct{ Body *pages.PageState }
type pageUpdateInput struct {
	ID   string `path:"id"`
	Body struct {
		Values map[string][]string `json:"values" doc:"New values by attribute; an empty list returns the attribute to the server default"`
	}
}
type pageUpdateOutput struct{ Body *pages.PageUpdate }

func registerPages(api huma.API, d Deps) {
	tags := []string{"settings"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "listPages", Method: http.MethodGet, Path: apiPrefix + "/pages",
		Summary: "Settings pages that apply to this server", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*pageListOutput, error) {
			var list []pages.PageSummary
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = d.Pages.List(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &pageListOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "getPage", Method: http.MethodGet, Path: apiPrefix + "/pages/{id}",
		Summary: "A settings page with the current values", Tags: tags},
		func(ctx context.Context, in *pagePath) (*pageOutput, error) {
			var st *pages.PageState
			err := conn(ctx, func(c *ldap.Conn) (err error) { st, err = d.Pages.Get(c, in.ID); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &pageOutput{Body: st}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "updatePage", Method: http.MethodPatch, Path: apiPrefix + "/pages/{id}",
		Summary: "Change settings of a page", Description: "Only attributes whose values differ are written, in one modify.",
		Tags: tags},
		func(ctx context.Context, in *pageUpdateInput) (*pageUpdateOutput, error) {
			var res *pages.PageUpdate
			err := conn(ctx, func(c *ldap.Conn) (err error) { res, err = d.Pages.Update(c, in.ID, in.Body.Values); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			if d.Restart != nil {
				for _, a := range res.Restart {
					d.Restart.Require(a)
				}
			}
			return &pageUpdateOutput{Body: res}, nil
		})
}
