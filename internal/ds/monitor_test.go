package ds

import (
	"testing"
	"time"
)

func TestParseConnection(t *testing.T) {
	c, ok := ParseConnection("1:20261110065424Z:78:76:-:cn=Directory Manager:0:12:43:1800:ip=local")
	if !ok {
		t.Fatal("not parsed")
	}
	if c.FD != "1" || c.OpsInitiated != 78 || c.OpsCompleted != 76 || c.BindDN != "cn=Directory Manager" ||
		c.ID != "1800" || c.Address != "local" || !c.Opened.Equal(time.Date(2026, 11, 10, 6, 54, 24, 0, time.UTC)) {
		t.Errorf("%+v", c)
	}
	// A bind DN containing colons survives.
	c, _ = ParseConnection("7:20261110065424Z:1:1:r:cn=a:b,dc=x:0:0:0:42:ip=10.0.0.5")
	if c.BindDN != "cn=a:b,dc=x" || c.Address != "10.0.0.5" || c.ID != "42" {
		t.Errorf("%+v", c)
	}
	if _, ok := ParseConnection("garbage"); ok {
		t.Error("garbage must not parse")
	}
}

func TestParseDisk(t *testing.T) {
	d, ok := ParseDisk(`partition="/data" size="1081101176832" used="88126504960" available="992974671872" use%="8"`)
	if !ok || d.Partition != "/data" || d.Size != 1081101176832 || d.UsePct != 8 || d.Available != 992974671872 {
		t.Errorf("%+v", d)
	}
	if _, ok := ParseDisk("nonsense"); ok {
		t.Error("nonsense must not parse")
	}
}

func TestBackendNameFromMonitorDN(t *testing.T) {
	if n := backendNameFromMonitorDN("cn=monitor,cn=userroot,cn=ldbm database,cn=plugins,cn=config"); n != "userroot" {
		t.Errorf("name = %q", n)
	}
}
