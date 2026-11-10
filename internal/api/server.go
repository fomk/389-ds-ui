// Package api is the HTTP layer: routing, OpenAPI, request/response models.
// Handlers never talk LDAP directly; they go through internal/ds.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/monitor"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
	"github.com/fomk/389-ds-ui/internal/templates"
)

// LDAP is the subset of ldapx.Client the API needs.
type LDAP interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// Deps are the dependencies of the HTTP layer.
type Deps struct {
	LDAP           LDAP
	Schemas        *schema.Cache
	Entries        *entries.Service
	Templates      []templates.Template
	History        *monitor.History
	Restart        *ds.RestartRegistry
	Pages          *pages.Service
	Tasks          *ds.Tasks
	Events         *events.Bus
	Files          *providers.Files // nil when the server's files are not reachable
	Lifecycle      providers.Lifecycle
	Capabilities   providers.Capabilities
	Instance       string
	Version        string
	AllowedOrigins []string
	Static         fs.FS // embedded SPA; nil disables UI serving
	Logger         *slog.Logger
}

const apiPrefix = "/api/v1"

func init() {
	// Response models always use empty slices instead of nil, so arrays are
	// declared non-nullable; this keeps the generated TS types simple.
	huma.DefaultArrayNullable = false
}

// NewHandler builds the complete HTTP handler.
func NewHandler(d Deps) http.Handler {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, requestLogger(d.Logger), hostGuard(d.AllowedOrigins), csrfGuard(d.AllowedOrigins),
		middleware.Compress(5, "application/json", "application/problem+json"))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if err := ping(req.Context(), d.LDAP); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Server-sent events: plain handler, the compress middleware must not buffer it.
	r.With(middleware.NoCache).Get(apiPrefix+"/events", eventsHandler(d.Events))
	r.Put(apiPrefix+"/ldif-files/{name}", uploadLDIF(d))
	r.With(middleware.NoCache).Get(apiPrefix+"/logs/{kind}/stream", streamLog(d))

	register(humachi.New(r, apiConfig(d.Version)), d)

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if len(req.URL.Path) >= len("/api/") && req.URL.Path[:5] == "/api/" {
			writeProblem(w, http.StatusNotFound, "no such API endpoint")
			return
		}
		spaHandler(d.Static).ServeHTTP(w, req)
	})
	return r
}

// OpenAPI returns the OpenAPI document without needing live dependencies.
func OpenAPI(version string) ([]byte, error) {
	api := humachi.New(chi.NewRouter(), apiConfig(version))
	register(api, Deps{})
	return json.MarshalIndent(api.OpenAPI(), "", "  ")
}

func apiConfig(version string) huma.Config {
	if version == "" {
		version = "dev"
	}
	cfg := huma.DefaultConfig("389-ds-ui API", version)
	cfg.Info.Description = "Administration API for 389 Directory Server."
	cfg.OpenAPIPath = apiPrefix + "/openapi"
	cfg.DocsPath = apiPrefix + "/docs"
	cfg.SchemasPath = apiPrefix + "/schemas"
	// Keep "$schema" links out of responses; the TS client does not need them.
	cfg.CreateHooks = nil
	return cfg
}

func register(api huma.API, d Deps) {
	registerSystem(api, d)
	registerBrowser(api, d)
	registerBrowserWrites(api, d)
	registerTemplates(api, d)
	registerLDIF(api, d)
	registerMonitor(api, d)
	registerDatabases(api, d)
	registerBackups(api, d)
	registerLogs(api, d)
	registerPages(api, d)
	registerPlugins(api, d)
	registerSecurity(api, d)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
