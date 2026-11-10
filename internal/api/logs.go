package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/logs"
	"github.com/fomk/389-ds-ui/internal/providers"
)

type logsOutput struct{ Body []logs.Log }

type tailInput struct {
	Kind   string `path:"kind" enum:"access,errors,audit,auditfail,security"`
	File   string `query:"file" doc:"Rotated file name; empty = current file"`
	Lines  int    `query:"lines" default:"500" minimum:"1" maximum:"5000"`
	Filter string `query:"filter" doc:"Words that must all occur (case-insensitive); conn=N and op=N match whole tokens"`
}

type tailOutput struct {
	Body struct {
		Path      string   `json:"path"`
		Lines     []string `json:"lines"`
		Truncated bool     `json:"truncated" doc:"The scan limit was reached before enough matching lines were found"`
	}
}

// maxTailScan bounds how much of a log file one request reads backwards.
const maxTailScan = 64 << 20

func listLogs(ctx context.Context, d Deps) ([]logs.Log, error) {
	if d.Files == nil {
		return nil, providers.ErrNotSupported
	}
	var list []logs.Log
	err := d.LDAP.Do(ctx, func(c *ldap.Conn) (err error) { list, err = logs.List(c, d.Files.Root); return })
	return list, err
}

func registerLogs(api huma.API, d Deps) {
	tags := []string{"logs"}
	huma.Register(api, huma.Operation{OperationID: "listLogs", Method: http.MethodGet, Path: apiPrefix + "/logs",
		Summary: "Server logs and their files (needs file access)", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*logsOutput, error) {
			list, err := listLogs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &logsOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "tailLog", Method: http.MethodGet, Path: apiPrefix + "/logs/{kind}",
		Summary: "Last lines of a log file, optionally filtered", Tags: tags},
		func(ctx context.Context, in *tailInput) (*tailOutput, error) {
			list, err := listLogs(ctx, d)
			if err != nil {
				return nil, toHTTPError(err)
			}
			path, err := logs.Resolve(list, in.Kind, in.File)
			if err != nil {
				return nil, toHTTPError(err)
			}
			lines, trunc, err := logs.Tail(path, in.Lines, logs.NewFilter(in.Filter), maxTailScan)
			if err != nil {
				return nil, toHTTPError(err)
			}
			out := &tailOutput{}
			out.Body.Path, out.Body.Lines, out.Body.Truncated = path, lines, trunc
			return out, nil
		})
}

// streamLog follows the current file of a log as server-sent events:
// GET /api/v1/logs/{kind}/stream?filter=… → "event: lines" with a JSON array.
func streamLog(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := listLogs(r.Context(), d)
		var path string
		if err == nil {
			path, err = logs.Resolve(list, chi.URLParam(r, "kind"), "")
		}
		if err != nil {
			var p *Problem
			if errorsAs(toHTTPError(err), &p) {
				writeProblem(w, p.Status, p.Detail)
				return
			}
			writeProblem(w, http.StatusInternalServerError, err.Error())
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeProblem(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		_, _ = fmt.Fprint(w, "retry: 3000\n\n")
		flusher.Flush()

		ctx := r.Context()
		err = logs.Follow(ctx, path, logs.NewFilter(r.URL.Query().Get("filter")), 500*time.Millisecond, func(batch []string) error {
			if batch == nil { // keep-alive for proxies
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return err
				}
				flusher.Flush()
				return nil
			}
			b, _ := json.Marshal(batch)
			if _, err := fmt.Fprintf(w, "event: lines\ndata: %s\n\n", b); err != nil {
				return err
			}
			flusher.Flush()
			return nil
		})
		if err != nil {
			d.Logger.Debug("log stream ended", "path", path, "err", err)
		}
	}
}
