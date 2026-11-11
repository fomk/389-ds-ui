package ldapx

import (
	"context"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/actor"
)

// ControlProxiedAuthorization (RFC 4370): the request is evaluated as the
// given identity, with its access rights.
const ControlProxiedAuthorization = "2.16.840.1.113730.3.4.18"

// ProxyControls returns the controls that make a request run as the
// signed-in user, or nil. extra controls are kept.
func ProxyControls(ctx context.Context, extra ...ldap.Control) []ldap.Control {
	a := actor.From(ctx)
	if a == nil || !a.Proxy || a.DN == "" {
		return extra
	}
	return append(extra, ldap.NewControlString(ControlProxiedAuthorization, true, "dn:"+a.DN))
}

// Proxied reports whether requests in ctx run as another identity.
func Proxied(ctx context.Context) bool {
	a := actor.From(ctx)
	return a != nil && a.Proxy && a.DN != ""
}
