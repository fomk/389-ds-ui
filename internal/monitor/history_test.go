package monitor

import (
	"testing"
	"time"
)

func TestRingBuffer(t *testing.T) {
	h := NewHistory(nil, time.Second, 3, nil)
	if len(h.Samples()) != 0 {
		t.Fatal("new history must be empty")
	}
	for i := int64(1); i <= 5; i++ {
		h.Add(Sample{OpsCompleted: i})
	}
	got := h.Samples()
	if len(got) != 3 || got[0].OpsCompleted != 3 || got[2].OpsCompleted != 5 {
		t.Errorf("samples = %+v", got)
	}
}
