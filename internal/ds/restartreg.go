package ds

import (
	"slices"
	"sync"
)

// RestartRegistry remembers settings changed since the last restart that
// 389-ds only applies after a restart. For each one it keeps the value the
// server is running with, so changing a setting back clears the reason again.
// It lives in memory: after a restart of ds-ui itself the banner is gone,
// which is acceptable for a hint.
type RestartRegistry struct {
	mu sync.Mutex
	// reason → value the running server has (the value before the first change)
	running map[string]string
}

// unknownValue never equals a real value: reasons recorded with Require are
// only cleared by a restart.
const unknownValue = "\x00unknown"

// Require records why a restart is needed (e.g. "nsslapd-port") without
// knowing the running value.
func (r *RestartRegistry) Require(reason string) {
	r.Change(reason, unknownValue, "")
}

// Change records that reason changed from before to after. The first change
// since the restart fixes the running value; a change back to it removes the
// reason.
func (r *RestartRegistry) Change(reason, before, after string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	running, pending := r.running[reason]
	switch {
	case !pending && before != after:
		if r.running == nil {
			r.running = map[string]string{}
		}
		r.running[reason] = before
	case pending && after == running:
		delete(r.running, reason)
	}
}

// Clear forgets all reasons (after a successful restart).
func (r *RestartRegistry) Clear() {
	r.mu.Lock()
	r.running = nil
	r.mu.Unlock()
}

// Reasons returns the sorted list of pending reasons.
func (r *RestartRegistry) Reasons() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.running))
	for k := range r.running {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
