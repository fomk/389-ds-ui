// Package actor carries the signed-in user through a request: who acts (for
// the task history) and whether directory data is accessed on their behalf
// (Proxied Authorization, so that the server applies their ACIs).
package actor

import "context"

// Roles of ds-ui users.
const (
	RoleAdmin    = "admin"    // server administration
	RoleOperator = "operator" // read-only administration; data as allowed by ACIs
)

// Actor is the user a request runs for.
type Actor struct {
	DN   string `json:"dn"`
	Name string `json:"name"`
	Role string `json:"role" enum:"admin,operator"`
	// Proxy: directory data is read and written with Proxied Authorization
	// as DN. False for the root DN, which binds as itself anyway.
	Proxy bool `json:"-"`
}

type key struct{}

// With returns a context carrying a.
func With(ctx context.Context, a *Actor) context.Context { return context.WithValue(ctx, key{}, a) }

// From returns the actor of a request, or nil (authentication is off).
func From(ctx context.Context) *Actor {
	a, _ := ctx.Value(key{}).(*Actor)
	return a
}

// Name is the DN of the actor, or "" when authentication is off.
func Name(ctx context.Context) string {
	if a := From(ctx); a != nil {
		return a.DN
	}
	return ""
}
