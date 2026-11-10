package ds

import (
	"slices"
	"sync"
)

// RestartRegistry remembers that settings changed which 389-ds only applies
// after a restart. It lives in memory: after a restart of ds-ui itself the
// banner is gone, which is acceptable for a hint.
type RestartRegistry struct {
	mu      sync.Mutex
	reasons map[string]struct{}
}

// Require records why a restart is needed (e.g. "nsslapd-port").
func (r *RestartRegistry) Require(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reasons == nil {
		r.reasons = map[string]struct{}{}
	}
	r.reasons[reason] = struct{}{}
}

// Clear forgets all reasons (after a successful restart).
func (r *RestartRegistry) Clear() {
	r.mu.Lock()
	r.reasons = nil
	r.mu.Unlock()
}

// Reasons returns the sorted list of pending reasons.
func (r *RestartRegistry) Reasons() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.reasons))
	for k := range r.reasons {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
