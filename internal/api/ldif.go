package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/fomk/389-ds-ui/internal/entries"
)

type searchInput struct {
	Body entries.SearchRequest
}

type searchOutput struct {
	Body *entries.SearchResult
}

type exportInput struct {
	Base        string `query:"base" required:"true"`
	Scope       string `query:"scope" enum:"base,one,sub" default:"sub"`
	Filter      string `query:"filter" default:"(objectClass=*)"`
	Attrs       string `query:"attrs" doc:"Comma-separated attribute list; empty exports all user attributes (required for CSV)"`
	Operational bool   `query:"operational" doc:"Also export operational attributes (LDIF only)"`
	Format      string `query:"format" enum:"ldif,csv" default:"ldif"`
	Passwords   bool   `query:"passwords" doc:"Include password hashes (excluded by default)"`
}

type applyLDIFInput struct {
	Body struct {
		LDIF        string `json:"ldif" minLength:"1" maxLength:"10485760" doc:"Content or change records (add, modify, delete); URL values are not allowed"`
		StopOnError bool   `json:"stopOnError,omitempty"`
	}
}

type applyLDIFOutput struct {
	Body *entries.ApplySummary
}

type replaceLDIFInput struct {
	DN      string `query:"dn" required:"true"`
	IfMatch string `header:"If-Match"`
	Body    struct {
		LDIF string `json:"ldif" minLength:"1" maxLength:"10485760" doc:"One content record for the same DN"`
	}
}

func splitAttrs(s string) []string {
	var out []string
	for _, a := range strings.Split(s, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

func registerLDIF(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "search",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/search",
		Summary:     "Search entries (interactive, limited)",
		Tags:        []string{"search"},
	}, func(ctx context.Context, in *searchInput) (*searchOutput, error) {
		res, err := d.Entries.Search(ctx, in.Body)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &searchOutput{Body: res}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "exportEntries",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/export",
		Summary:     "Stream entries as LDIF or CSV",
		Tags:        []string{"search"},
		Responses: map[string]*huma.Response{
			"200": {Description: "LDIF or CSV stream", Content: map[string]*huma.MediaType{
				"text/ldif": {}, "text/csv": {},
			}},
		},
	}, func(ctx context.Context, in *exportInput) (*huma.StreamResponse, error) {
		req := entries.ExportRequest{Base: in.Base, Scope: in.Scope, Filter: in.Filter,
			Attrs: splitAttrs(in.Attrs), Operational: in.Operational, Passwords: in.Passwords}
		if err := entries.ValidateExport(req); err != nil {
			return nil, toHTTPError(err)
		}
		if in.Format == "csv" && len(req.Attrs) == 0 {
			return nil, huma.Error400BadRequest("CSV export needs the attrs parameter")
		}
		return &huma.StreamResponse{Body: func(hctx huma.Context) {
			ext, ctype := "ldif", "text/ldif; charset=utf-8"
			if in.Format == "csv" {
				ext, ctype = "csv", "text/csv; charset=utf-8"
			}
			hctx.SetHeader("Content-Type", ctype)
			hctx.SetHeader("Content-Disposition", fmt.Sprintf(`attachment; filename="export.%s"`, ext))
			hctx.SetHeader("Cache-Control", "no-store")
			w := hctx.BodyWriter()
			var err error
			if in.Format == "csv" {
				_, err = d.Entries.ExportCSV(hctx.Context(), w, req)
			} else {
				_, err = d.Entries.ExportLDIF(hctx.Context(), w, req)
			}
			if err != nil {
				// Headers are already sent: report the failure inside the stream
				// (an LDIF comment; for CSV a visibly broken last line).
				_, _ = fmt.Fprintf(w, "\n# ERROR: export incomplete: %v\n", err)
				d.Logger.Warn("export failed", "base", in.Base, "err", err)
			}
		}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "applyLDIF",
		Method:      http.MethodPost,
		Path:        apiPrefix + "/ldif/apply",
		Summary:     "Apply an LDIF fragment (add, modify, delete records)",
		Tags:        []string{"search"},
	}, func(ctx context.Context, in *applyLDIFInput) (*applyLDIFOutput, error) {
		res, err := d.Entries.ApplyLDIF(ctx, in.Body.LDIF, in.Body.StopOnError)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &applyLDIFOutput{Body: res}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "replaceEntryFromLDIF",
		Method:      http.MethodPut,
		Path:        apiPrefix + "/entries/ldif",
		Summary:     "Replace an entry's attributes with the given LDIF record",
		Tags:        []string{"entries"},
	}, func(ctx context.Context, in *replaceLDIFInput) (*entryOutput, error) {
		e, err := d.Entries.ReplaceFromLDIF(ctx, in.DN, in.IfMatch, in.Body.LDIF)
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &entryOutput{ETag: e.ETag, Body: e}, nil
	})
}
