package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
)

type attributeTypeInput struct{ Body schema.AttributeTypeDef }
type attributeTypeUpdateInput struct {
	Name string `path:"name"`
	Body schema.AttributeTypeDef
}
type objectClassInput struct{ Body schema.ObjectClassDef }
type objectClassUpdateInput struct {
	Name string `path:"name"`
	Body schema.ObjectClassDef
}
type schemaNamePath struct {
	Name string `path:"name" doc:"Name or OID"`
}

// registerSchemaWrites adds the endpoints that change user-defined schema.
// Every change reads the current schema (not the cache) for its checks and
// invalidates the cache afterwards.
func registerSchemaWrites(api huma.API, d Deps) {
	tags := []string{"schema"}
	write := func(ctx context.Context, fn func(*ldap.Conn, *schema.Schema) error) (*noContent, error) {
		err := d.LDAP.Do(ctx, func(c *ldap.Conn) error {
			s, _, _, err := schema.Fetch(c)
			if err != nil {
				return err
			}
			return fn(c, s)
		})
		if d.Schemas != nil {
			d.Schemas.Invalidate()
		}
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &noContent{}, nil
	}

	huma.Register(api, huma.Operation{OperationID: "createAttributeType", Method: http.MethodPost, Path: apiPrefix + "/schema/attributetypes",
		Summary: "Add a user-defined attribute type", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *attributeTypeInput) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.AddAttributeType(c, s, in.Body) })
		})
	huma.Register(api, huma.Operation{OperationID: "updateAttributeType", Method: http.MethodPut, Path: apiPrefix + "/schema/attributetypes/{name}",
		Summary: "Change a user-defined attribute type (the OID stays)", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *attributeTypeUpdateInput) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.ReplaceAttributeType(c, s, in.Name, in.Body) })
		})
	huma.Register(api, huma.Operation{OperationID: "deleteAttributeType", Method: http.MethodDelete, Path: apiPrefix + "/schema/attributetypes/{name}",
		Summary: "Delete an unused user-defined attribute type", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *schemaNamePath) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.DeleteAttributeType(c, s, in.Name) })
		})

	huma.Register(api, huma.Operation{OperationID: "createObjectClass", Method: http.MethodPost, Path: apiPrefix + "/schema/objectclasses",
		Summary: "Add a user-defined object class", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *objectClassInput) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.AddObjectClass(c, s, in.Body) })
		})
	huma.Register(api, huma.Operation{OperationID: "updateObjectClass", Method: http.MethodPut, Path: apiPrefix + "/schema/objectclasses/{name}",
		Summary: "Change a user-defined object class (the OID stays)", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *objectClassUpdateInput) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.ReplaceObjectClass(c, s, in.Name, in.Body) })
		})
	huma.Register(api, huma.Operation{OperationID: "deleteObjectClass", Method: http.MethodDelete, Path: apiPrefix + "/schema/objectclasses/{name}",
		Summary: "Delete an unused user-defined object class", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *schemaNamePath) (*noContent, error) {
			return write(ctx, func(c *ldap.Conn, s *schema.Schema) error { return ds.DeleteObjectClass(c, s, in.Name) })
		})
}
