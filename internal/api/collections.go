package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/pages"
)

type collectionsOutput struct{ Body []pages.CollectionSummary }
type collectionInput struct {
	ID     string `path:"id"`
	Parent string `query:"parent" doc:"Parent item, for nested collections"`
}
type collectionOutput struct{ Body *pages.CollectionState }
type itemCreateInput struct {
	ID     string `path:"id"`
	Parent string `query:"parent"`
	Body   struct {
		Name   string              `json:"name" minLength:"1"`
		Values map[string][]string `json:"values"`
	}
}
type itemPath struct {
	ID     string `path:"id"`
	Name   string `path:"name"`
	Parent string `query:"parent"`
}
type itemUpdateInput struct {
	ID     string `path:"id"`
	Name   string `path:"name"`
	Parent string `query:"parent"`
	Body   struct {
		Values map[string][]string `json:"values"`
	}
}
type itemOutput struct{ Body *pages.Item }
type pluginTasksOutput struct{ Body []ds.PluginTask }
type pluginTaskInput struct {
	ID   string `path:"id"`
	Body struct {
		Params map[string]string `json:"params"`
	}
}

func registerCollections(api huma.API, d Deps) {
	tags := []string{"settings"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }

	huma.Register(api, huma.Operation{OperationID: "listCollections", Method: http.MethodGet, Path: apiPrefix + "/collections",
		Summary: "Collections of similar entries (plug-in definitions)", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*collectionsOutput, error) {
			return &collectionsOutput{Body: d.Pages.Collections()}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "getCollection", Method: http.MethodGet, Path: apiPrefix + "/collections/{id}",
		Summary: "Items of a collection", Tags: tags},
		func(ctx context.Context, in *collectionInput) (*collectionOutput, error) {
			var st *pages.CollectionState
			err := conn(ctx, func(c *ldap.Conn) (err error) { st, err = d.Pages.ListItems(c, in.ID, in.Parent); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &collectionOutput{Body: st}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "createItem", Method: http.MethodPost, Path: apiPrefix + "/collections/{id}",
		Summary: "Add an item", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *itemCreateInput) (*itemOutput, error) {
			var it *pages.Item
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				it, err = d.Pages.CreateItem(c, in.ID, in.Parent, in.Body.Name, in.Body.Values)
				return
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &itemOutput{Body: it}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "updateItem", Method: http.MethodPatch, Path: apiPrefix + "/collections/{id}/items/{name}",
		Summary: "Change an item", Tags: tags},
		func(ctx context.Context, in *itemUpdateInput) (*itemOutput, error) {
			var it *pages.Item
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				it, err = d.Pages.UpdateItem(c, in.ID, in.Parent, in.Name, in.Body.Values)
				return
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &itemOutput{Body: it}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "deleteItem", Method: http.MethodDelete, Path: apiPrefix + "/collections/{id}/items/{name}",
		Summary: "Delete an item with everything below it", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *itemPath) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return d.Pages.DeleteItem(c, in.ID, in.Parent, in.Name) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listPluginTasks", Method: http.MethodGet, Path: apiPrefix + "/plugin-tasks",
		Summary: "Maintenance tasks of plug-ins (fixup, rebuild)", Tags: []string{"plugins"}},
		func(ctx context.Context, _ *struct{}) (*pluginTasksOutput, error) {
			return &pluginTasksOutput{Body: ds.PluginTasks}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "startPluginTask", Method: http.MethodPost, Path: apiPrefix + "/plugin-tasks/{id}",
		Summary: "Start a plug-in task", Tags: []string{"plugins"}, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *pluginTaskInput) (*taskOutput, error) {
			task, target, attrs, err := ds.PluginTaskAttrs(in.ID, in.Body.Params)
			if err != nil {
				return nil, toHTTPError(err)
			}
			t, err := d.Tasks.Start(ctx, task.Container, target, attrs)
			if ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
				// The task container exists only while the plug-in runs.
				return nil, problem(http.StatusConflict, "the task is not available: the plug-in "+task.Plugin+" is not running (enable it; without dynamic plug-ins, restart the server)")
			}
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &taskOutput{Status: http.StatusAccepted, Body: t}, nil
		})
}
