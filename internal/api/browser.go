package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

type schemaInput struct {
	IfNoneMatch string `header:"If-None-Match"`
}

type schemaOutput struct {
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	Body         *schema.Schema
}

type entryInput struct {
	DN string `query:"dn" required:"true" doc:"Distinguished name of the entry"`
}

type entryOutput struct {
	ETag string `header:"ETag"`
	Body *entries.Entry
}

type childrenInput struct {
	DN      string `query:"dn" required:"true" doc:"Parent entry"`
	Cursor  string `query:"cursor" doc:"nextCursor from the previous page"`
	Limit   int    `query:"limit" default:"200" minimum:"1" maximum:"1000"`
	Refresh bool   `query:"refresh" doc:"Bypass the short-lived children cache"`
}

type childrenOutput struct {
	Body *entries.ChildrenPage
}

type rootsInput struct {
	System bool `query:"system" doc:"Include server-internal trees (cn=config, cn=schema, cn=monitor)"`
}

type rootsOutput struct {
	Body []entries.Summary
}

func registerBrowser(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "getSchema",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/schema",
		Summary:     "Parsed LDAP schema with resolved inheritance",
		Tags:        []string{"schema"},
	}, func(ctx context.Context, in *schemaInput) (*schemaOutput, error) {
		s, version, err := d.Schemas.Get(ctx, d.LDAP)
		if err != nil {
			return nil, toHTTPError(err)
		}
		tag := `"` + version + `"`
		if in.IfNoneMatch == tag {
			return nil, huma.Status304NotModified()
		}
		return &schemaOutput{ETag: tag, CacheControl: "no-cache", Body: s}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getEntryRoots",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/entries/roots",
		Summary:     "Tree roots: naming contexts and optional system trees",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *rootsInput) (*rootsOutput, error) {
		roots, err := d.Entries.Roots(ctx, in.System)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &rootsOutput{Body: roots}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getEntry",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/entries",
		Summary:     "Read one entry with user and operational attributes",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *entryInput) (*entryOutput, error) {
		e, err := d.Entries.Get(ctx, in.DN)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &entryOutput{ETag: e.ETag, Body: e}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getEntryChildren",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/entries/children",
		Summary:     "Direct children of an entry, sorted by RDN, paged",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *childrenInput) (*childrenOutput, error) {
		page, err := d.Entries.Children(ctx, in.DN, in.Cursor, in.Limit, in.Refresh)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &childrenOutput{Body: page}, nil
	})
}
