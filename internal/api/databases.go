package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type backendsOutput struct{ Body []ds.Backend }
type backendOutput struct{ Body *ds.Backend }
type backendPath struct {
	Name string `path:"name" doc:"Backend name"`
}
type createBackendInput struct{ Body ds.NewBackend }

// Path parameters are declared on each input: huma ignores fields promoted
// from embedded unexported structs.
type patchBackendInput struct {
	Name string `path:"name"`
	Body ds.BackendPatch
}
type indexesOutput struct{ Body []ds.Index }
type indexInput struct {
	Name string `path:"name"`
	Body ds.Index
}
type indexPath struct {
	Name string `path:"name"`
	Attr string `path:"attr"`
}
type updateIndexInput struct {
	Name string `path:"name"`
	Attr string `path:"attr"`
	Body struct {
		Types         []string `json:"types" minItems:"1"`
		MatchingRules []string `json:"matchingRules,omitempty"`
	}
}
type reindexInput struct {
	Name string `path:"name"`
	Body struct {
		Attributes []string `json:"attributes,omitempty" doc:"Empty rebuilds every index of the backend"`
	}
}
type taskOutput struct {
	Status int
	Body   *ds.Task
}
type vlvOutput struct{ Body []ds.VLVIndex }
type tasksOutput struct{ Body []*ds.Task }
type taskPath struct {
	ID string `path:"id"`
}
type noContent struct{}

func registerDatabases(api huma.API, d Deps) {
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }
	tags := []string{"databases"}

	huma.Register(api, huma.Operation{OperationID: "listBackends", Method: http.MethodGet, Path: apiPrefix + "/backends",
		Summary: "Suffixes and their backends", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*backendsOutput, error) {
			var list []ds.Backend
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListBackends(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &backendsOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "createBackend", Method: http.MethodPost, Path: apiPrefix + "/backends",
		Summary: "Create a suffix (backend, mapping tree, optional root entry)", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *createBackendInput) (*backendOutput, error) {
			var b *ds.Backend
			err := conn(ctx, func(c *ldap.Conn) error {
				if err := ds.CreateBackend(c, in.Body); err != nil {
					return err
				}
				var err error
				b, err = ds.GetBackend(c, in.Body.Name)
				return err
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &backendOutput{Body: b}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "updateBackend", Method: http.MethodPatch, Path: apiPrefix + "/backends/{name}",
		Summary: "Change backend settings", Tags: tags},
		func(ctx context.Context, in *patchBackendInput) (*backendOutput, error) {
			var b *ds.Backend
			err := conn(ctx, func(c *ldap.Conn) error {
				if err := ds.UpdateBackend(c, in.Name, in.Body); err != nil {
					return err
				}
				var err error
				b, err = ds.GetBackend(c, in.Name)
				return err
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &backendOutput{Body: b}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteBackend", Method: http.MethodDelete, Path: apiPrefix + "/backends/{name}",
		Summary: "Delete a suffix and all its data", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *backendPath) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.DeleteBackend(c, in.Name) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listIndexes", Method: http.MethodGet, Path: apiPrefix + "/backends/{name}/indexes",
		Summary: "Attribute indexes of a backend", Tags: tags},
		func(ctx context.Context, in *backendPath) (*indexesOutput, error) {
			var list []ds.Index
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListIndexes(c, in.Name); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &indexesOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "createIndex", Method: http.MethodPost, Path: apiPrefix + "/backends/{name}/indexes",
		Summary: "Add an index (empty until reindexed)", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *indexInput) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.CreateIndex(c, in.Name, in.Body) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "updateIndex", Method: http.MethodPut, Path: apiPrefix + "/backends/{name}/indexes/{attr}",
		Summary: "Change index types and matching rules", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *updateIndexInput) (*noContent, error) {
			ix := ds.Index{Attribute: in.Attr, Types: in.Body.Types, MatchingRules: in.Body.MatchingRules}
			if ix.MatchingRules == nil {
				ix.MatchingRules = []string{}
			}
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.UpdateIndex(c, in.Name, ix) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "deleteIndex", Method: http.MethodDelete, Path: apiPrefix + "/backends/{name}/indexes/{attr}",
		Summary: "Delete an index", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *indexPath) (*noContent, error) {
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.DeleteIndex(c, in.Name, in.Attr) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &noContent{}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "reindex", Method: http.MethodPost, Path: apiPrefix + "/backends/{name}/reindex",
		Summary: "Start a reindex task", Tags: tags, DefaultStatus: http.StatusAccepted},
		func(ctx context.Context, in *reindexInput) (*taskOutput, error) {
			var attrs map[string][]string
			err := conn(ctx, func(c *ldap.Conn) (err error) { attrs, err = ds.ReindexAttrs(c, in.Name, in.Body.Attributes); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			t, err := d.Tasks.Start(ctx, ds.TaskIndex, in.Name, attrs)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &taskOutput{Status: http.StatusAccepted, Body: t}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listVLV", Method: http.MethodGet, Path: apiPrefix + "/backends/{name}/vlv",
		Summary: "VLV indexes of a backend", Tags: tags},
		func(ctx context.Context, in *backendPath) (*vlvOutput, error) {
			var list []ds.VLVIndex
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListVLV(c, in.Name); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &vlvOutput{Body: list}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "listTasks", Method: http.MethodGet, Path: apiPrefix + "/tasks",
		Summary: "Tasks started by this ds-ui, newest first", Tags: []string{"tasks"}},
		func(context.Context, *struct{}) (*tasksOutput, error) { return &tasksOutput{Body: d.Tasks.List()}, nil })

	huma.Register(api, huma.Operation{OperationID: "getTask", Method: http.MethodGet, Path: apiPrefix + "/tasks/{id}",
		Summary: "One task", Tags: []string{"tasks"}},
		func(_ context.Context, in *taskPath) (*taskOutput, error) {
			t, err := d.Tasks.Get(in.ID)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &taskOutput{Status: http.StatusOK, Body: t}, nil
		})
}
