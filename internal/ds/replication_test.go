package ds

import "testing"

func TestParseRUV(t *testing.T) {
	r := ParseRUV([]string{
		"{replicageneration} 6537a5c5000000010000",
		"{replica 2 ldap://ds2:3389} 6537a5d0000000020000 6537a700000300020000",
		"{replica 1 ldap://ds1:3389} 6537a5c6000000010000 6537a6ff000000010000",
		"{replica 3 ldap://ds3:3389}",
		"garbage",
	})
	if r.Generation != "6537a5c5000000010000" || len(r.Replicas) != 3 {
		t.Fatalf("%+v", r)
	}
	if r.Replicas[0].ReplicaID != 1 || r.Replicas[0].URL != "ldap://ds1:3389" || r.Replicas[0].MaxCSN != "6537a6ff000000010000" {
		t.Errorf("first = %+v", r.Replicas[0])
	}
	if r.Replicas[0].MaxTime == nil || r.Replicas[0].MaxTime.Unix() != 0x6537a6ff {
		t.Errorf("max time = %v", r.Replicas[0].MaxTime)
	}
	if r.Replicas[2].ReplicaID != 3 || r.Replicas[2].MaxCSN != "" || r.Replicas[2].MaxTime != nil {
		t.Errorf("empty element = %+v", r.Replicas[2])
	}
}

func TestStatusState(t *testing.T) {
	for in, want := range map[string]string{
		"": "never",
		"Error (0) Replica acquired successfully: Incremental update succeeded":            "ok",
		"Error (0) Total update in progress":                                               "busy",
		"Error (-1) Problem connecting to replica - LDAP error: Can't contact LDAP server": "error",
		"something else": "unknown",
	} {
		if got := statusState(in); got != want {
			t.Errorf("%q: %s, want %s", in, got, want)
		}
	}
}
