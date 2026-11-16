package topology

import (
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/ds"
)

func TestComputeLag(t *testing.T) {
	at := func(s int) *time.Time { v := time.Unix(1_800_000_000+int64(s), 0).UTC(); return &v }
	rep := &TopologyReport{Nodes: []TopologyNode{
		{Address: "a:389", Reachable: true, RUV: []ds.RUVElement{{ReplicaID: 1, MaxTime: at(100)}, {ReplicaID: 2, MaxTime: at(50)}}},
		{Address: "b:389", Reachable: true, RUV: []ds.RUVElement{{ReplicaID: 1, MaxTime: at(90)}, {ReplicaID: 2, MaxTime: at(60)}}},
		{Address: "c:389", Reachable: true, RUV: []ds.RUVElement{{ReplicaID: 1, MaxTime: at(100)}}},
		{Address: "d:389", Reachable: false},
	}}
	computeLag(rep)
	if len(rep.Suppliers) != 2 || rep.Suppliers[0] != 1 || rep.Suppliers[1] != 2 {
		t.Fatalf("suppliers %v", rep.Suppliers)
	}
	want := map[string][]int64{"a:389": {0, 10}, "b:389": {10, 0}, "c:389": {0, -1}}
	wantMax := map[string]int64{"a:389": 10, "b:389": 10, "c:389": -1, "d:389": 0}
	for _, n := range rep.Nodes {
		var got []int64
		for _, b := range n.SupplierLag {
			got = append(got, b.Seconds)
		}
		if w, ok := want[n.Address]; ok && (len(got) != len(w) || got[0] != w[0] || got[1] != w[1]) {
			t.Errorf("%s behind %v, want %v", n.Address, got, w)
		}
		if n.MaxBehind != wantMax[n.Address] {
			t.Errorf("%s max %d, want %d", n.Address, n.MaxBehind, wantMax[n.Address])
		}
	}
}
