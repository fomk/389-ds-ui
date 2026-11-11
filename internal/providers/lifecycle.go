// Package providers abstracts capabilities that depend on how ds-ui is
// deployed next to the directory server (process control, files, certificates).
package providers

import (
	"context"
	"errors"
)

// ServerState is the process-level state of the directory server.
type ServerState string

const (
	StateRunning    ServerState = "running"
	StateStopped    ServerState = "stopped"
	StateRestarting ServerState = "restarting"
	StateUnknown    ServerState = "unknown"
)

// ErrNotSupported is returned by providers that cannot perform an operation.
var ErrNotSupported = errors.New("operation not supported by this deployment")

// Lifecycle controls the directory server process.
type Lifecycle interface {
	// Name identifies the implementation ("docker", "none").
	Name() string
	Status(ctx context.Context) (ServerState, error)
	Restart(ctx context.Context) error
	Stop(ctx context.Context) error
	Start(ctx context.Context) error
}

// NoLifecycle is used when ds-ui has no control over the server process.
type NoLifecycle struct{}

func (NoLifecycle) Name() string { return "none" }
func (NoLifecycle) Status(context.Context) (ServerState, error) {
	return StateUnknown, ErrNotSupported
}
func (NoLifecycle) Restart(context.Context) error { return ErrNotSupported }
func (NoLifecycle) Stop(context.Context) error    { return ErrNotSupported }
func (NoLifecycle) Start(context.Context) error   { return ErrNotSupported }
