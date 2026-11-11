package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/monitor"
	"github.com/fomk/389-ds-ui/internal/providers"
)

type monitorOutput struct {
	Body *ds.Monitor
}

type historyOutput struct {
	Body struct {
		IntervalSeconds int              `json:"intervalSeconds"`
		Samples         []monitor.Sample `json:"samples"`
	}
}

type restartOutput struct {
	Status int
}

func registerMonitor(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "getMonitor",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/monitor",
		Summary:     "Server, database and connection statistics",
		Tags:        []string{"monitor"},
	}, func(ctx context.Context, _ *struct{}) (*monitorOutput, error) {
		var m *ds.Monitor
		err := d.LDAP.Do(ctx, func(c *ldap.Conn) (err error) {
			m, err = ds.ReadMonitor(c)
			return err
		})
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &monitorOutput{Body: m}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getMonitorHistory",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/monitor/history",
		Summary:     "Recent samples of cumulative counters (kept in memory)",
		Tags:        []string{"monitor"},
	}, func(context.Context, *struct{}) (*historyOutput, error) {
		out := &historyOutput{}
		out.Body.Samples = []monitor.Sample{}
		if d.History != nil {
			out.Body.IntervalSeconds = int(d.History.Interval().Seconds())
			out.Body.Samples = d.History.Samples()
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "restartServer",
		Method:        http.MethodPost,
		Path:          apiPrefix + "/system/restart",
		Summary:       "Restart the directory server (asynchronous; watch /system/state)",
		Tags:          []string{"system"},
		DefaultStatus: http.StatusAccepted,
	}, func(context.Context, *struct{}) (*restartOutput, error) {
		if d.Lifecycle == nil || d.Lifecycle.Name() == "none" {
			return nil, toHTTPError(providers.ErrNotSupported)
		}
		// Detached from the request: the restart must finish even if the
		// browser goes away. Docker waits up to 60 s for a clean shutdown.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			err := d.Lifecycle.Restart(ctx)
			if d.Tasks != nil {
				d.Tasks.Record(ctx, ds.OpRestart, "", err)
			}
			if err != nil {
				d.Logger.Error("server restart failed", "err", err)
				return
			}
			d.Logger.Info("directory server restarted")
			if d.Restart != nil {
				d.Restart.Clear()
			}
		}()
		return &restartOutput{Status: http.StatusAccepted}, nil
	})
}
