//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/ds"
)

func TestReadMonitor(t *testing.T) {
	c := newClient(t)
	var m *ds.Monitor
	if err := c.Do(context.Background(), func(conn *ldap.Conn) (err error) {
		m, err = ds.ReadMonitor(conn)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s := m.Server
	if s.Version == "" || s.StartTime.IsZero() || s.UptimeSeconds < 0 || s.Threads <= 0 || s.OpsCompleted <= 0 {
		t.Errorf("server = %+v", s)
	}
	if len(m.Connections) == 0 || m.Connections[0].BindDN == "" {
		t.Errorf("connections = %+v", m.Connections)
	}
	if len(m.Backends) != 1 || m.Backends[0].Name != "userroot" || m.Backends[0].Suffix != suffix || m.Backends[0].EntryCache.MaxSize <= 0 {
		t.Errorf("backends = %+v", m.Backends)
	}
	if len(m.Disks) == 0 || m.Disks[0].Size <= 0 {
		t.Errorf("disks = %+v", m.Disks)
	}
	if m.Operations["searchops"] <= 0 {
		t.Errorf("snmp counters = %v", m.Operations)
	}
}
