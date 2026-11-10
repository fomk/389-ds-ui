package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type rolesOutput struct{ Body []ds.Role }
type roleOutput struct{ Body *ds.Role }
type roleCreateInput struct{ Body ds.NewRole }
type dnQuery struct {
	DN string `query:"dn" required:"true"`
}
type rolePatchInput struct {
	DN   string `query:"dn" required:"true"`
	Body ds.RolePatch
}
type roleMembersInput struct {
	DN    string `query:"dn" required:"true"`
	Limit int    `query:"limit" default:"200" minimum:"1" maximum:"2000"`
}

// RoleMembers lists the entries that have a role.
type RoleMembers struct {
	Members   []string `json:"members"`
	Truncated bool     `json:"truncated"`
}
type roleMembersOutput struct{ Body RoleMembers }
type roleMemberInput struct {
	DN   string `query:"dn" required:"true" doc:"The managed role"`
	Body struct {
		Member string `json:"member" minLength:"1"`
		Add    bool   `json:"add" doc:"false removes the member"`
	}
}

type cosListOutput struct{ Body []ds.CoSDefinition }
type cosOutput struct{ Body *ds.CoSDefinition }
type cosCreateInput struct{ Body ds.NewCoS }
type templateInput struct {
	DN   string `query:"dn" required:"true" doc:"Template entry (created if missing)"`
	Body ds.TemplateValues
}

func registerRoles(api huma.API, d Deps) {
	tags := []string{"roles and CoS"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }
	empty := func(err error) (*noContent, error) {
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &noContent{}, nil
	}

	huma.Register(api, huma.Operation{OperationID: "listRoles", Method: http.MethodGet, Path: apiPrefix + "/roles",
		Summary: "Role definitions of all suffixes", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*rolesOutput, error) {
			var list []ds.Role
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListRoles(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &rolesOutput{Body: list}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "createRole", Method: http.MethodPost, Path: apiPrefix + "/roles",
		Summary: "Create a managed, filtered or nested role", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *roleCreateInput) (*roleOutput, error) {
			var r *ds.Role
			err := conn(ctx, func(c *ldap.Conn) (err error) { r, err = ds.CreateRole(c, in.Body); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &roleOutput{Body: r}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "updateRole", Method: http.MethodPatch, Path: apiPrefix + "/roles",
		Summary: "Change a role", Tags: tags},
		func(ctx context.Context, in *rolePatchInput) (*roleOutput, error) {
			var r *ds.Role
			err := conn(ctx, func(c *ldap.Conn) (err error) { r, err = ds.UpdateRole(c, in.DN, in.Body); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &roleOutput{Body: r}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "deleteRole", Method: http.MethodDelete, Path: apiPrefix + "/roles",
		Summary: "Delete a role", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *dnQuery) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DeleteRole(c, in.DN) }))
		})
	huma.Register(api, huma.Operation{OperationID: "listRoleMembers", Method: http.MethodGet, Path: apiPrefix + "/roles/members",
		Summary: "Entries that have the role (nsRole)", Tags: tags},
		func(ctx context.Context, in *roleMembersInput) (*roleMembersOutput, error) {
			var out RoleMembers
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				out.Members, out.Truncated, err = ds.RoleMembers(c, in.DN, in.Limit)
				return
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &roleMembersOutput{Body: out}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "setRoleMember", Method: http.MethodPost, Path: apiPrefix + "/roles/members",
		Summary: "Add or remove a member of a managed role", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *roleMemberInput) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.SetManagedMember(c, in.DN, in.Body.Member, in.Body.Add) }))
		})

	huma.Register(api, huma.Operation{OperationID: "listCoS", Method: http.MethodGet, Path: apiPrefix + "/cos",
		Summary: "Class of Service definitions with their templates", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*cosListOutput, error) {
			var list []ds.CoSDefinition
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListCoS(c); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &cosListOutput{Body: list}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "createCoS", Method: http.MethodPost, Path: apiPrefix + "/cos",
		Summary: "Create a pointer, indirect or classic CoS definition", Tags: tags, DefaultStatus: http.StatusCreated},
		func(ctx context.Context, in *cosCreateInput) (*cosOutput, error) {
			var r *ds.CoSDefinition
			err := conn(ctx, func(c *ldap.Conn) (err error) { r, err = ds.CreateCoS(c, in.Body); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &cosOutput{Body: r}, nil
		})
	huma.Register(api, huma.Operation{OperationID: "deleteCoS", Method: http.MethodDelete, Path: apiPrefix + "/cos",
		Summary: "Delete a CoS definition (templates stay)", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *dnQuery) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DeleteCoS(c, in.DN) }))
		})
	huma.Register(api, huma.Operation{OperationID: "setCoSTemplate", Method: http.MethodPut, Path: apiPrefix + "/cos/templates",
		Summary: "Create a CoS template or replace its values", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *templateInput) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.SetTemplate(c, in.DN, in.Body) }))
		})
	huma.Register(api, huma.Operation{OperationID: "deleteCoSTemplate", Method: http.MethodDelete, Path: apiPrefix + "/cos/templates",
		Summary: "Delete a CoS template", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *dnQuery) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DeleteTemplate(c, in.DN) }))
		})
}
