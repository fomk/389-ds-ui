package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/fomk/389-ds-ui/internal/health"
)

type healthOutput struct{ Body *health.Report }

func registerHealth(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{OperationID: "healthCheck", Method: http.MethodGet, Path: apiPrefix + "/health",
		Summary: "Check the server for problems (settings, certificates, disks, replication, indexes)", Tags: []string{"health"}},
		func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
			ctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			return &healthOutput{Body: health.Run(ctx, health.Env{LDAP: d.LDAP, Certs: d.Certs})}, nil
		})
}
