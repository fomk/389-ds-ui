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

type modifyInput struct {
	DN      string `query:"dn" required:"true"`
	IfMatch string `header:"If-Match" doc:"ETag from the last read; the request fails with 412 if the entry changed since"`
	Body    struct {
		Changes []entries.Change `json:"changes" minItems:"1"`
	}
}

type createInput struct {
	Body entries.NewEntry
}

type createOutput struct {
	Status int
	ETag   string `header:"ETag"`
	Body   *entries.Entry
}

type deleteInput struct {
	DN      string `query:"dn" required:"true"`
	Subtree bool   `query:"subtree" doc:"Also delete all descendants (at most 10000 entries)"`
}

type deleteOutput struct {
	Body *entries.DeleteResult
}

type renameInput struct {
	Body entries.RenameRequest
}

type renameOutput struct {
	Body *entries.RenameResult
}

type passwordInput struct {
	Body entries.PasswordRequest
}

type passwordOutput struct {
	Body *entries.PasswordResult
}

func registerBrowserWrites(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "modifyEntry",
		Method:      http.MethodPatch,
		Path:        apiPrefix + "/entries",
		Summary:     "Modify attributes of an entry",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *modifyInput) (*entryOutput, error) {
		e, err := d.Entries.Modify(ctx, in.DN, in.IfMatch, in.Body.Changes)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &entryOutput{ETag: e.ETag, Body: e}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "createEntry",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/entries",
		Summary:       "Create an entry",
		Tags:          []string{"entries"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, in *createInput) (*createOutput, error) {
		e, err := d.Entries.Add(ctx, in.Body)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &createOutput{Status: http.StatusCreated, ETag: e.ETag, Body: e}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "deleteEntry",
		Method:      http.MethodDelete,
		Path:        apiPrefix + "/entries",
		Summary:     "Delete an entry, optionally with its subtree",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *deleteInput) (*deleteOutput, error) {
		res, err := d.Entries.Delete(ctx, in.DN, in.Subtree)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &deleteOutput{Body: res}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "renameEntry",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/entries/rename",
		Summary:     "Rename and/or move an entry (ModifyDN)",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *renameInput) (*renameOutput, error) {
		res, err := d.Entries.Rename(ctx, in.Body)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &renameOutput{Body: res}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "setPassword",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/entries/password",
		Summary:     "Set a password with the Password Modify extended operation",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *passwordInput) (*passwordOutput, error) {
		res, err := d.Entries.SetPassword(ctx, in.Body)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &passwordOutput{Body: res}, nil
	})
}
