// Package files gives ds-ui access to server directories on the shared
// /data volume (LDIF files, backups). Every name is validated and every path
// is confined to its directory.
package files

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	// ErrInvalidName rejects names with path separators, "..", etc.
	ErrInvalidName = errors.New("invalid file name")
	// ErrNotFound is returned for a missing file.
	ErrNotFound = errors.New("file not found")
	// ErrExists is returned when creating a file that already exists.
	ErrExists = errors.New("file already exists")
	// ErrOutsideRoot: the server is configured to use a directory that is not
	// on the volume shared with ds-ui.
	ErrOutsideRoot = errors.New("directory is not on the shared data volume")
)

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidName reports whether name is a plain, safe file name.
func ValidName(name string) bool { return nameRE.MatchString(name) && !strings.Contains(name, "..") }

// Info describes a file or a backup directory.
type Info struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Dir      bool      `json:"dir,omitempty"`
}

// Store is one directory.
type Store struct{ dir string }

// Open returns a store for dir, which must lie inside root.
func Open(root, dir string) (Store, error) {
	r, err := filepath.Abs(root)
	if err != nil {
		return Store{}, err
	}
	d, err := filepath.Abs(dir)
	if err != nil {
		return Store{}, err
	}
	if d != r && !strings.HasPrefix(d, r+string(filepath.Separator)) {
		return Store{}, fmt.Errorf("%w: %s", ErrOutsideRoot, dir)
	}
	return Store{dir: d}, nil
}

// Dir is the directory path (as the server sees it on the shared volume).
func (s Store) Dir() string { return s.dir }

// Path returns the full path of name after validating it.
func (s Store) Path(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return filepath.Join(s.dir, name), nil
}

// List returns entries (files, or directories when dirs is true), newest first.
func (s Store) List(dirs bool) ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Info{}
	for _, e := range entries {
		if e.IsDir() != dirs || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		in := Info{Name: e.Name(), Size: fi.Size(), Modified: fi.ModTime().UTC(), Dir: e.IsDir()}
		if e.IsDir() {
			in.Size = dirSize(filepath.Join(s.dir, e.Name()))
		}
		out = append(out, in)
	}
	slices.SortFunc(out, func(a, b Info) int { return b.Modified.Compare(a.Modified) })
	return out, nil
}

func dirSize(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// Stat returns information about name.
func (s Store) Stat(name string) (Info, error) {
	p, err := s.Path(name)
	if err != nil {
		return Info{}, err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Info{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return Info{}, err
	}
	return Info{Name: name, Size: fi.Size(), Modified: fi.ModTime().UTC(), Dir: fi.IsDir()}, nil
}

// OpenFile opens a regular file for reading.
func (s Store) OpenFile(name string) (*os.File, Info, error) {
	in, err := s.Stat(name)
	if err != nil {
		return nil, Info{}, err
	}
	if in.Dir {
		return nil, Info{}, fmt.Errorf("%w: %s is a directory", ErrInvalidName, name)
	}
	f, err := os.Open(filepath.Join(s.dir, name))
	return f, in, err
}

// Create writes r to a new file; an existing file is not overwritten. A
// partially written file is removed on error.
func (s Store) Create(name string, r io.Reader) (Info, error) {
	p, err := s.Path(name)
	if err != nil {
		return Info{}, err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Info{}, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errors.Is(err, fs.ErrExist) {
		return Info{}, fmt.Errorf("%w: %s", ErrExists, name)
	}
	if err != nil {
		return Info{}, err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(p)
		return Info{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(p)
		return Info{}, err
	}
	return s.Stat(name)
}

// Remove deletes a file or a backup directory.
func (s Store) Remove(name string) error {
	in, err := s.Stat(name)
	if err != nil {
		return err
	}
	p := filepath.Join(s.dir, in.Name)
	if in.Dir {
		return os.RemoveAll(p)
	}
	return os.Remove(p)
}

// WriteTarGz streams directory name as a .tar.gz archive.
func (s Store) WriteTarGz(w io.Writer, name string) error {
	in, err := s.Stat(name)
	if err != nil {
		return err
	}
	if !in.Dir {
		return fmt.Errorf("%w: %s is not a directory", ErrInvalidName, name)
	}
	base := filepath.Join(s.dir, name)
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() && !fi.IsDir() {
			return nil // skip sockets, symlinks
		}
		rel, _ := filepath.Rel(s.dir, p)
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if fi.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}
