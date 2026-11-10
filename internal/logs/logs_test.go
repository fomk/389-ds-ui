package logs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/files"
)

func writeLines(t *testing.T, path string, from, to int) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for i := from; i < to; i++ {
		_, _ = fmt.Fprintf(f, "[t] conn=%d op=%d RESULT err=%d padding-%s\n", i%20, i, i%7, strings.Repeat("x", 40))
	}
}

func TestTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access")
	writeLines(t, path, 0, 20000) // ~1.7 MB: many 64 KB chunks
	lines, trunc, err := Tail(path, 5, Filter{}, 1<<30)
	if err != nil || trunc || len(lines) != 5 {
		t.Fatalf("%v %v %d", err, trunc, len(lines))
	}
	if !strings.Contains(lines[4], "op=19999 ") || !strings.Contains(lines[0], "op=19995 ") {
		t.Errorf("last lines, oldest first: %q … %q", lines[0], lines[4])
	}

	lines, _, _ = Tail(path, 1000, NewFilter("conn=1 err=3"), 1<<30)
	for _, l := range lines {
		if !strings.Contains(l, " conn=1 ") || !strings.Contains(l, "err=3") {
			t.Fatalf("filter mismatch: %q", l)
		}
	}
	if len(lines) == 0 {
		t.Fatal("filter found nothing")
	}

	// A filter that never matches stops after maxScan bytes.
	lines, trunc, _ = Tail(path, 10, NewFilter("no-such-text"), 200*1024)
	if len(lines) != 0 || !trunc {
		t.Errorf("bounded scan: %d lines, truncated=%v", len(lines), trunc)
	}

	// Whole small file including a first line without preceding newline.
	small := filepath.Join(t.TempDir(), "small")
	_ = os.WriteFile(small, []byte("one\ntwo\nthree"), 0o644)
	lines, _, _ = Tail(small, 10, Filter{}, 1<<30)
	if strings.Join(lines, ",") != "one,two,three" {
		t.Errorf("small file = %v", lines)
	}
}

func TestFilterTokens(t *testing.T) {
	f := NewFilter("CONN=1")
	if !f.Match("x conn=1 op=2") || f.Match("x conn=12 op=2") || !f.Match("conn=1") {
		t.Error("conn token matching")
	}
	if !NewFilter("result ERR=0").Match("conn=3 op=4 RESULT err=0 tag=101") {
		t.Error("case-insensitive AND")
	}
	if !NewFilter("op=").Match("conn=3 op=4") {
		t.Error("a bare op= is a substring")
	}
}

func TestFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "errors")
	writeLines(t, path, 0, 10) // existing content is not re-sent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var got []string
	done := make(chan error, 1)
	go func() {
		done <- Follow(ctx, path, NewFilter("op="), 20*time.Millisecond, func(b []string) error {
			if b == nil {
				return nil // keep-alive
			}
			mu.Lock()
			got = append(got, b...)
			mu.Unlock()
			return nil
		})
	}()
	time.Sleep(60 * time.Millisecond)
	writeLines(t, path, 10, 13)
	time.Sleep(100 * time.Millisecond)
	// Rotation: the file is renamed and a new one created.
	_ = os.Rename(path, path+".20261110-000000")
	writeLines(t, path, 100, 102)
	time.Sleep(150 * time.Millisecond)
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 || !strings.Contains(got[0], "op=10 ") || !strings.Contains(got[4], "op=101 ") {
		t.Errorf("followed lines = %q", got)
	}
}

func TestResolve(t *testing.T) {
	logs := []Log{
		{Kind: "audit", Path: "/data/logs/audit", Files: nil},
		{Kind: "auditfail", Path: "/data/logs/audit", SameAs: "audit"},
	}
	logs[0].Files = append(logs[0].Files, logs0Files()...)
	if p, err := Resolve(logs, "auditfail", ""); err != nil || p != "/data/logs/audit" {
		t.Errorf("sameAs: %s %v", p, err)
	}
	if p, err := Resolve(logs, "audit", "audit.20261110-000000"); err != nil || p != "/data/logs/audit.20261110-000000" {
		t.Errorf("rotated: %s %v", p, err)
	}
	if _, err := Resolve(logs, "audit", "../../etc/passwd"); err == nil {
		t.Error("unlisted file must be refused")
	}
	if _, err := Resolve(logs, "nope", ""); err == nil {
		t.Error("unknown kind must be refused")
	}
}

func logs0Files() []files.Info {
	return []files.Info{{Name: "audit"}, {Name: "audit.20261110-000000"}}
}
