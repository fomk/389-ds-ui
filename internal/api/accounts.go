package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

type accountDNInput struct {
	DN string `query:"dn" required:"true"`
}
type accountStatusOutput struct{ Body *ds.AccountStatus }

func registerAccounts(api huma.API, d Deps) {
	tags := []string{"accounts"}
	conn := func(ctx context.Context, fn func(*ldap.Conn) error) error { return d.LDAP.Do(ctx, fn) }
	status := func(ctx context.Context, dn string, change func(*ldap.Conn, string) error, op string) (*accountStatusOutput, error) {
		var st *ds.AccountStatus
		err := conn(ctx, func(c *ldap.Conn) error {
			if change != nil {
				if err := change(c, dn); err != nil {
					return err
				}
			}
			var err error
			st, err = ds.GetAccountStatus(c, dn, time.Now())
			return err
		})
		if op != "" && d.Tasks != nil {
			d.Tasks.Record(ctx, op, dn, err)
		}
		if err != nil {
			return nil, toHTTPError(err)
		}
		return &accountStatusOutput{Body: st}, nil
	}

	huma.Register(api, huma.Operation{OperationID: "getAccountStatus", Method: http.MethodGet, Path: apiPrefix + "/accounts/status",
		Summary: "Lock, lockout and password expiry state of an entry", Tags: tags},
		func(ctx context.Context, in *accountDNInput) (*accountStatusOutput, error) {
			return status(ctx, in.DN, nil, "")
		})
	huma.Register(api, huma.Operation{OperationID: "lockAccount", Method: http.MethodPost, Path: apiPrefix + "/accounts/lock",
		Summary: "Lock an account (nsAccountLock)", Tags: tags},
		func(ctx context.Context, in *accountDNInput) (*accountStatusOutput, error) {
			return status(ctx, in.DN, ds.LockAccount, ds.OpLockAccount)
		})
	huma.Register(api, huma.Operation{OperationID: "unlockAccount", Method: http.MethodPost, Path: apiPrefix + "/accounts/unlock",
		Summary: "Unlock an account and reset a lockout after failed binds", Tags: tags},
		func(ctx context.Context, in *accountDNInput) (*accountStatusOutput, error) {
			return status(ctx, in.DN, ds.UnlockAccount, ds.OpUnlockAccount)
		})
}
