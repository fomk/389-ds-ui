// Package monitor keeps a short in-memory history of server counters for
// charts. Nothing is persisted: the history starts when ds-ui starts.
package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// Sample is one reading of the cumulative counters of cn=monitor.
type Sample struct {
	Time         time.Time `json:"t"`
	OpsCompleted int64     `json:"opsCompleted"`
	EntriesSent  int64     `json:"entriesSent"`
	BytesSent    int64     `json:"bytesSent"`
	Connections  int64     `json:"connections" doc:"Current connections (a gauge, not cumulative)"`
	Threads      int64     `json:"threads"`
}

// Doer runs a function with an LDAP connection.
type Doer interface {
	Do(ctx context.Context, fn func(*ldap.Conn) error) error
}

// History samples counters at a fixed interval into a ring buffer.
type History struct {
	interval time.Duration
	size     int
	ldap     Doer
	log      *slog.Logger

	mu      sync.Mutex
	samples []Sample
	next    int
	full    bool
}

// NewHistory keeps size samples taken every interval.
func NewHistory(l Doer, interval time.Duration, size int, log *slog.Logger) *History {
	if log == nil {
		log = slog.Default()
	}
	return &History{interval: interval, size: size, ldap: l, log: log, samples: make([]Sample, size)}
}

// Interval returns the sampling interval.
func (h *History) Interval() time.Duration { return h.interval }

// Run samples until ctx is cancelled.
func (h *History) Run(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	h.sample(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.sample(ctx)
		}
	}
}

func (h *History) sample(ctx context.Context) {
	var s Sample
	err := h.ldap.Do(ctx, func(c *ldap.Conn) error {
		res, err := c.Search(ldap.NewSearchRequest("cn=monitor", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
			"(objectClass=*)", []string{"opscompleted", "entriessent", "bytessent", "currentconnections", "threads"}, nil))
		if err != nil || len(res.Entries) == 0 {
			return err
		}
		e := res.Entries[0]
		s = Sample{Time: time.Now().UTC(),
			OpsCompleted: atoi(e.GetAttributeValue("opscompleted")), EntriesSent: atoi(e.GetAttributeValue("entriessent")),
			BytesSent: atoi(e.GetAttributeValue("bytessent")), Connections: atoi(e.GetAttributeValue("currentconnections")),
			Threads: atoi(e.GetAttributeValue("threads"))}
		return nil
	})
	if err != nil || s.Time.IsZero() {
		h.log.Debug("monitor sample skipped", "err", err)
		return
	}
	h.Add(s)
}

// Add appends a sample, overwriting the oldest one when full.
func (h *History) Add(s Sample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.samples[h.next] = s
	h.next = (h.next + 1) % h.size
	if h.next == 0 {
		h.full = true
	}
}

// Samples returns the samples oldest first.
func (h *History) Samples() []Sample {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.full {
		return append([]Sample(nil), h.samples[:h.next]...)
	}
	return append(append([]Sample(nil), h.samples[h.next:]...), h.samples[:h.next]...)
}

func atoi(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
