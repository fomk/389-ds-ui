package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/providers"
)

type SystemInfo struct {
	Instance  string        `json:"instance" doc:"389 instance name"`
	UIVersion string        `json:"uiVersion"`
	Server    ds.ServerInfo `json:"server"`
}

type systemInfoOutput struct{ Body SystemInfo }

type capabilitiesOutput struct{ Body providers.Capabilities }

type SystemState struct {
	LDAP            string `json:"ldap" enum:"ok,unreachable,error" doc:"Result of an LDAP round-trip"`
	LDAPError       string `json:"ldapError,omitempty"`
	Server          string `json:"server" enum:"running,stopped,restarting,unknown" doc:"Process state from the lifecycle provider"`
	RestartRequired bool   `json:"restartRequired" doc:"A setting changed that only applies after restart"`
}

type systemStateOutput struct{ Body SystemState }

func registerSystem(api huma.API, d Deps) {
	huma.Register(api, huma.Operation{
		OperationID: "getSystemInfo",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/system/info",
		Summary:     "Server and UI information",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*systemInfoOutput, error) {
		var info *ds.ServerInfo
		err := d.LDAP.Do(ctx, func(c *ldap.Conn) (err error) {
			info, err = ds.GetServerInfo(c)
			return err
		})
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &systemInfoOutput{Body: SystemInfo{Instance: d.Instance, UIVersion: d.Version, Server: *info}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getCapabilities",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/system/capabilities",
		Summary:     "Feature groups available in this deployment",
		Tags:        []string{"system"},
	}, func(context.Context, *struct{}) (*capabilitiesOutput, error) {
		return &capabilitiesOutput{Body: d.Capabilities}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "getSystemState",
		Method:      http.MethodGet,
		Path:        apiPrefix + "/system/state",
		Summary:     "Live server state",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*systemStateOutput, error) {
		st := SystemState{LDAP: "ok", Server: string(providers.StateUnknown)}
		if err := ping(ctx, d.LDAP); err != nil {
			st.LDAP, st.LDAPError = "error", err.Error()
			if ldapx.IsConnError(err) {
				st.LDAP = "unreachable"
			}
		}
		if d.Lifecycle != nil {
			if s, err := d.Lifecycle.Status(ctx); err == nil {
				st.Server = string(s)
			}
		}
		if st.Server == string(providers.StateUnknown) && st.LDAP == "ok" {
			// Without a lifecycle provider, a working LDAP connection is the best signal.
			st.Server = string(providers.StateRunning)
		}
		return &systemStateOutput{Body: st}, nil
	})
}

func ping(ctx context.Context, l LDAP) error {
	if l == nil {
		return errors.New("no LDAP client configured")
	}
	return l.Do(ctx, func(c *ldap.Conn) error {
		_, err := c.WhoAmI(nil)
		return err
	})
}
