package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFiles(t *testing.T) {
	root := t.TempDir()
	if DetectFiles(root) != nil {
		t.Fatal("empty dir must not be detected")
	}
	for _, d := range []string{"config", "logs"} {
		if err := os.Mkdir(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := DetectFiles(root)
	if f == nil || f.Root != root {
		t.Fatalf("DetectFiles = %+v", f)
	}
	if DetectFiles("") != nil {
		t.Error("empty path must not be detected")
	}
}
