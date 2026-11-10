package files

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidName(t *testing.T) {
	for name, ok := range map[string]bool{
		"export.ldif": true, "backup_2026-11-10": true,
		"../etc/passwd": false, "a/b": false, ".hidden": false, "": false, "a..b": false, "x y": false,
	} {
		if ValidName(name) != ok {
			t.Errorf("ValidName(%q) != %v", name, ok)
		}
	}
}

func TestOpenConfinesToRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := Open(root, filepath.Join(root, "ldif")); err != nil {
		t.Error(err)
	}
	if _, err := Open(root, "/etc"); !errors.Is(err, ErrOutsideRoot) {
		t.Errorf("outside root: %v", err)
	}
	if _, err := Open(root, root+"-other"); !errors.Is(err, ErrOutsideRoot) {
		t.Errorf("sibling with the same prefix: %v", err)
	}
}

func TestStoreLifecycle(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root, filepath.Join(root, "ldif"))
	if list, err := s.List(false); err != nil || len(list) != 0 {
		t.Fatalf("missing dir lists empty: %v %v", list, err)
	}
	if _, err := s.Create("a.ldif", strings.NewReader("dn: dc=x\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("a.ldif", strings.NewReader("x")); !errors.Is(err, ErrExists) {
		t.Errorf("overwrite: %v", err)
	}
	if _, err := s.Create("../evil", strings.NewReader("x")); !errors.Is(err, ErrInvalidName) {
		t.Errorf("traversal: %v", err)
	}
	f, in, err := s.OpenFile("a.ldif")
	if err != nil || in.Size != 9 {
		t.Fatalf("open: %v %+v", err, in)
	}
	_ = f.Close()
	if err := s.Remove("a.ldif"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat("a.ldif"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after remove: %v", err)
	}
}

func TestTarGz(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root, root)
	_ = os.MkdirAll(filepath.Join(root, "bk1", "userroot"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "bk1", "DBVERSION"), []byte("v"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "bk1", "userroot", "data.mdb"), []byte("data"), 0o644)
	if list, _ := s.List(true); len(list) != 1 || list[0].Size != 5 {
		t.Errorf("dir listing with size: %+v", list)
	}
	var buf bytes.Buffer
	if err := s.WriteTarGz(&buf, "bk1"); err != nil {
		t.Fatal(err)
	}
	gz, _ := gzip.NewReader(&buf)
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != "bk1/,bk1/DBVERSION,bk1/userroot/,bk1/userroot/data.mdb" {
		t.Errorf("archive = %v", names)
	}
}
