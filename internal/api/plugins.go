package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

// PluginList is the plug-in list with the server's dynamic-plug-ins mode.
type PluginList struct {
	Dynamic bool        `json:"dynamic" doc:"nsslapd-dynamic-plugins: changes apply without a restart"`
	Plugins []ds.Plugin `json:"plugins"`
}

// PluginChange is the result of enabling or disabling a plug-in.
type PluginChange struct {
	Plugin  *ds.Plugin `json:"plugin"`
	Restart bool       `json:"restart" doc:"The change applies after a server restart"`
}

type pluginListOutput struct{ Body PluginList }
type pluginPath struct {
	Name string `path:"name" doc:"Plug-in cn"`
}
type pluginOutput struct{ Body *ds.Plugin }
type pluginPatchInput struct {
	Name string `path:"name"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}
type pluginPatchOutput struct{ Body PluginChange }

func registerPlugins(api huma.API, d Deps) {
	tags := []string{"plugins"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "listPlugins", Method: http.MethodGet, Path: apiPrefix + "/plugins",
		Summary: "Server plug-ins", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*pluginListOutput, error) {
			var out PluginList
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				if out.Plugins, err = ds.ListPlugins(c); err != nil {
					return err
				}
				out.Dynamic, err = ds.DynamicPlugins(c)
				return err
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &pluginListOutput{Body: out}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "getPlugin", Method: http.MethodGet, Path: apiPrefix + "/plugins/{name}",
		Summary: "One plug-in", Tags: tags},
		func(ctx context.Context, in *pluginPath) (*pluginOutput, error) {
			var p *ds.Plugin
			err := conn(ctx, func(c *ldap.Conn) (err error) { p, err = ds.GetPlugin(c, in.Name); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &pluginOutput{Body: p}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "updatePlugin", Method: http.MethodPatch, Path: apiPrefix + "/plugins/{name}",
		Summary: "Enable or disable a plug-in", Tags: tags},
		func(ctx context.Context, in *pluginPatchInput) (*pluginPatchOutput, error) {
			var out PluginChange
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				if out.Restart, err = ds.SetPluginEnabled(c, in.Name, in.Body.Enabled); err != nil {
					return err
				}
				out.Plugin, err = ds.GetPlugin(c, in.Name)
				return err
			})
			if d.Tasks != nil {
				op := ds.OpDisablePlugin
				if in.Body.Enabled {
					op = ds.OpEnablePlugin
				}
				d.Tasks.Record(ctx, op, in.Name, err)
			}
			if err != nil {
				return nil, toHTTPError(err)
			}
			if out.Restart && d.Restart != nil {
				d.Restart.Change(in.Name, strconv.FormatBool(!in.Body.Enabled), strconv.FormatBool(in.Body.Enabled))
			}
			return &pluginPatchOutput{Body: out}, nil
		})
}
