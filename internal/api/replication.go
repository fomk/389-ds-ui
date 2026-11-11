package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type suffixQuery struct {
	Suffix string `query:"suffix" required:"true"`
}
type replicaOutput struct{ Body *ds.Replica }
type replicaEnableInput struct {
	Suffix string `query:"suffix" required:"true"`
	Body   ds.EnableReplica
}
type replicaPatchInput struct {
	Suffix string `query:"suffix" required:"true"`
	Body   ds.ReplicaPatch
}
type changelogOutput struct{ Body *ds.Changelog }
type changelogPatchInput struct {
	Suffix string `query:"suffix" required:"true"`
	Body   ds.ChangelogPatch
}
type replManagerInput struct {
	Body struct {
		DN       string `json:"dn,omitempty" doc:"Default: cn=replication manager,cn=config"`
		Password string `json:"password" minLength:"8"`
	}
}

// ReplManager reports the replication manager account.
type ReplManager struct {
	DN string `json:"dn"`
}
type replManagerOutput struct{ Body ReplManager }
type agreementsOutput struct{ Body []ds.Agreement }
type agreementOutput struct{ Body *ds.Agreement }
type agreementCreateInput struct {
	Suffix string `query:"suffix" required:"true"`
	Body   ds.AgreementInput
}
type agreementPath struct {
	Suffix string `query:"suffix" required:"true"`
	Name   string `path:"name"`
}
type agreementUpdateInput struct {
	Suffix string `query:"suffix" required:"true"`
	Name   string `path:"name"`
	Body   ds.AgreementInput
}
type ruvOutput struct{ Body *ds.RUV }
type cleanAllRUVInput struct {
	Suffix string `query:"suffix" required:"true"`
	Body   struct {
		ReplicaID int  `json:"replicaId"`
		Force     bool `json:"force,omitempty" doc:"Clean even if some replicas cannot be reached"`
	}
}
type conflictsOutput struct{ Body []ds.Conflict }
type conflictConvertInput struct {
	DN   string `query:"dn" required:"true"`
	Body struct {
		RDN string `json:"rdn" minLength:"1" doc:"New RDN, e.g. cn=jdoe-2"`
	}
}

func registerReplication(api huma.API, d Deps) {
	tags := []string{"replication"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }
	replica := func(ctx context.Context, fn func(*ldap.Conn) (*ds.Replica, error)) (*replicaOutput, error) {
		var r *ds.Replica
		err := conn(ctx, func(c *ldap.Conn) (err error) { r, err = fn(c); return })
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &replicaOutput{Body: r}, nil
	}
	agreement := func(ctx context.Context, fn func(*ldap.Conn) (*ds.Agreement, error)) (*agreementOutput, error) {
		var a *ds.Agreement
		err := conn(ctx, func(c *ldap.Conn) (err error) { a, err = fn(c); return })
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &agreementOutput{Body: a}, nil
	}
	empty := func(err error) (*noContent, error) {
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &noContent{}, nil
	}
	op := func(id, method, path, summary string, status int) huma.Operation {
		return huma.Operation{OperationID: id, Method: method, Path: apiPrefix + path, Summary: summary, Tags: tags, DefaultStatus: status}
	}

	huma.Register(api, op("getReplica", http.MethodGet, "/replication", "Replication configuration of a suffix", 0),
		func(ctx context.Context, in *suffixQuery) (*replicaOutput, error) {
			return replica(ctx, func(c *ldap.Conn) (*ds.Replica, error) { return ds.GetReplica(c, in.Suffix) })
		})
	huma.Register(api, op("enableReplication", http.MethodPost, "/replication", "Enable replication for a suffix", http.StatusCreated),
		func(ctx context.Context, in *replicaEnableInput) (*replicaOutput, error) {
			return replica(ctx, func(c *ldap.Conn) (*ds.Replica, error) { return ds.EnableReplication(c, in.Suffix, in.Body) })
		})
	huma.Register(api, op("updateReplica", http.MethodPatch, "/replication", "Change role, replica ID, bind DNs or purge delay", 0),
		func(ctx context.Context, in *replicaPatchInput) (*replicaOutput, error) {
			return replica(ctx, func(c *ldap.Conn) (*ds.Replica, error) { return ds.UpdateReplica(c, in.Suffix, in.Body) })
		})
	huma.Register(api, op("disableReplication", http.MethodDelete, "/replication", "Disable replication (deletes the agreements)", http.StatusNoContent),
		func(ctx context.Context, in *suffixQuery) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DisableReplication(c, in.Suffix) }))
		})

	huma.Register(api, op("getChangelog", http.MethodGet, "/replication/changelog", "Replication changelog of the suffix's backend", 0),
		func(ctx context.Context, in *suffixQuery) (*changelogOutput, error) {
			var cl *ds.Changelog
			err := conn(ctx, func(c *ldap.Conn) (err error) { cl, err = ds.GetChangelog(c, in.Suffix); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &changelogOutput{Body: cl}, nil
		})
	huma.Register(api, op("updateChangelog", http.MethodPatch, "/replication/changelog", "Change changelog limits", 0),
		func(ctx context.Context, in *changelogPatchInput) (*changelogOutput, error) {
			var cl *ds.Changelog
			err := conn(ctx, func(c *ldap.Conn) (err error) { cl, err = ds.UpdateChangelog(c, in.Suffix, in.Body); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &changelogOutput{Body: cl}, nil
		})

	huma.Register(api, op("setReplManager", http.MethodPost, "/replication/manager", "Create the replication manager account or set its password", 0),
		func(ctx context.Context, in *replManagerInput) (*replManagerOutput, error) {
			dn := in.Body.DN
			if dn == "" {
				dn = ds.DefaultReplManager
			}
			if err := conn(ctx, func(c *ldap.Conn) error { return ds.CreateReplManager(c, dn, in.Body.Password) }); err != nil {
				return nil, toHTTPError(err)
			}
			return &replManagerOutput{Body: ReplManager{DN: dn}}, nil
		})

	huma.Register(api, op("listAgreements", http.MethodGet, "/replication/agreements", "Agreements of a suffix with status", 0),
		func(ctx context.Context, in *suffixQuery) (*agreementsOutput, error) {
			var list []ds.Agreement
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListAgreements(c, in.Suffix); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &agreementsOutput{Body: list}, nil
		})
	huma.Register(api, op("createAgreement", http.MethodPost, "/replication/agreements", "Create an agreement", http.StatusCreated),
		func(ctx context.Context, in *agreementCreateInput) (*agreementOutput, error) {
			return agreement(ctx, func(c *ldap.Conn) (*ds.Agreement, error) { return ds.CreateAgreement(c, in.Suffix, in.Body) })
		})
	huma.Register(api, op("updateAgreement", http.MethodPatch, "/replication/agreements/{name}", "Change an agreement", 0),
		func(ctx context.Context, in *agreementUpdateInput) (*agreementOutput, error) {
			return agreement(ctx, func(c *ldap.Conn) (*ds.Agreement, error) { return ds.UpdateAgreement(c, in.Suffix, in.Name, in.Body) })
		})
	huma.Register(api, op("deleteAgreement", http.MethodDelete, "/replication/agreements/{name}", "Delete an agreement", http.StatusNoContent),
		func(ctx context.Context, in *agreementPath) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DeleteAgreement(c, in.Suffix, in.Name) }))
		})
	huma.Register(api, op("initAgreement", http.MethodPost, "/replication/agreements/{name}/init", "Initialize the consumer (total update)", 0),
		func(ctx context.Context, in *agreementPath) (*agreementOutput, error) {
			out, err := agreement(ctx, func(c *ldap.Conn) (*ds.Agreement, error) { return ds.InitAgreement(c, in.Suffix, in.Name) })
			if d.Tasks != nil {
				d.Tasks.Record(ctx, ds.OpInitReplica, in.Name, err)
			}
			return out, err
		})

	huma.Register(api, op("getRUV", http.MethodGet, "/replication/ruv", "Replica update vector of a suffix", 0),
		func(ctx context.Context, in *suffixQuery) (*ruvOutput, error) {
			var r *ds.RUV
			err := conn(ctx, func(c *ldap.Conn) (err error) { r, err = ds.GetRUV(c, in.Suffix); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &ruvOutput{Body: r}, nil
		})
	huma.Register(api, op("cleanAllRUV", http.MethodPost, "/replication/cleanallruv", "Remove a retired replica ID from the topology (task)", http.StatusAccepted),
		func(ctx context.Context, in *cleanAllRUVInput) (*taskOutput, error) {
			var attrs map[string][]string
			err := conn(ctx, func(c *ldap.Conn) (err error) {
				attrs, err = ds.CleanAllRUVAttrs(c, in.Suffix, in.Body.ReplicaID, in.Body.Force)
				return
			})
			if err != nil {
				return nil, toHTTPError(err)
			}
			t, err := d.Tasks.Start(ctx, ds.TaskCleanAllRUV, in.Suffix, attrs)
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &taskOutput{Status: http.StatusAccepted, Body: t}, nil
		})

	huma.Register(api, op("listConflicts", http.MethodGet, "/replication/conflicts", "Replication conflict and glue entries", 0),
		func(ctx context.Context, in *suffixQuery) (*conflictsOutput, error) {
			var list []ds.Conflict
			err := conn(ctx, func(c *ldap.Conn) (err error) { list, err = ds.ListConflicts(c, in.Suffix); return })
			if err != nil {
				return nil, toHTTPError(err)
			}
			return &conflictsOutput{Body: list}, nil
		})
	huma.Register(api, op("deleteConflict", http.MethodDelete, "/replication/conflicts", "Delete a conflict entry", http.StatusNoContent),
		func(ctx context.Context, in *dnQuery) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.DeleteConflict(c, in.DN) }))
		})
	huma.Register(api, op("convertConflict", http.MethodPost, "/replication/conflicts/convert", "Keep a conflict entry under a new RDN", http.StatusNoContent),
		func(ctx context.Context, in *conflictConvertInput) (*noContent, error) {
			return empty(conn(ctx, func(c *ldap.Conn) error { return ds.ConvertConflict(c, in.DN, in.Body.RDN) }))
		})
}
