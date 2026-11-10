// Package logs reads 389-ds log files from the shared volume: the tail of a
// file with an optional filter, and following a file as it grows (including
// rotation).
package logs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/files"
)

// Kinds of server logs and their cn=config attribute prefixes.
var Kinds = []string{"access", "errors", "audit", "auditfail", "security"}

var configAttr = map[string]string{
	"access": "nsslapd-accesslog", "errors": "nsslapd-errorlog", "audit": "nsslapd-auditlog",
	"auditfail": "nsslapd-auditfaillog", "security": "nsslapd-securitylog",
}

// ErrUnknownLog is returned for a log kind or file that does not exist.
var ErrUnknownLog = errors.New("unknown log")

// Log describes one server log and its files (current first, then rotated).
type Log struct {
	Kind     string       `json:"kind" enum:"access,errors,audit,auditfail,security"`
	Path     string       `json:"path"`
	Enabled  bool         `json:"enabled"`
	Buffered bool         `json:"buffered,omitempty" doc:"Lines are written in batches (nsslapd-accesslog-logbuffering)"`
	SameAs   string       `json:"sameAs,omitempty" doc:"Writes into the same file as another log"`
	Files    []files.Info `json:"files"`
}

// List reads the log configuration and lists the files of every log that lies
// on the shared volume rooted at root.
func List(c *ldap.Conn, root string) ([]Log, error) {
	attrs := []string{"nsslapd-accesslog-logbuffering"}
	for _, k := range Kinds {
		attrs = append(attrs, configAttr[k], configAttr[k]+"-logging-enabled")
	}
	res, err := c.Search(ldap.NewSearchRequest("cn=config", ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		"(objectClass=*)", attrs, nil))
	if err != nil {
		return nil, err
	}
	if len(res.Entries) == 0 {
		return nil, errors.New("cn=config not readable")
	}
	e := res.Entries[0]
	out := []Log{}
	seen := map[string]string{}
	for _, k := range Kinds {
		path := e.GetAttributeValue(configAttr[k])
		if path == "" {
			continue
		}
		l := Log{Kind: k, Path: path, Enabled: strings.EqualFold(e.GetAttributeValue(configAttr[k]+"-logging-enabled"), "on"), Files: []files.Info{}}
		if k == "access" {
			l.Buffered = strings.EqualFold(e.GetAttributeValue("nsslapd-accesslog-logbuffering"), "on")
		}
		if other, ok := seen[path]; ok {
			l.SameAs = other
		} else {
			seen[path] = k
			l.Files = logFiles(root, path)
		}
		out = append(out, l)
	}
	return out, nil
}

// logFiles lists the current file and its rotated siblings (base.YYYYMMDD-…).
func logFiles(root, path string) []files.Info {
	s, err := files.Open(root, filepath.Dir(path))
	if err != nil {
		return []files.Info{}
	}
	all, err := s.List(false)
	if err != nil {
		return []files.Info{}
	}
	base := filepath.Base(path)
	out := []files.Info{}
	for _, f := range all {
		if f.Name == base || (strings.HasPrefix(f.Name, base+".") && !strings.HasSuffix(f.Name, ".rotationinfo")) {
			out = append(out, f)
		}
	}
	slices.SortStableFunc(out, func(a, b files.Info) int {
		if a.Name == base {
			return -1
		}
		if b.Name == base {
			return 1
		}
		return b.Modified.Compare(a.Modified)
	})
	return out
}

// Resolve returns the path of file (empty = current) of the given log,
// accepting only files that List would show.
func Resolve(logs []Log, kind, file string) (string, error) {
	for _, l := range logs {
		if l.Kind != kind {
			continue
		}
		if l.SameAs != "" {
			return Resolve(logs, l.SameAs, file)
		}
		if file == "" {
			return l.Path, nil
		}
		for _, f := range l.Files {
			if f.Name == file {
				return filepath.Join(filepath.Dir(l.Path), f.Name), nil
			}
		}
		return "", fmt.Errorf("%w: %s/%s", ErrUnknownLog, kind, file)
	}
	return "", fmt.Errorf("%w: %s", ErrUnknownLog, kind)
}

// Filter matches log lines. Words are combined with AND; each is matched
// case-insensitively, except "conn=N" and "op=N" which must match whole tokens
// (conn=1 must not match conn=12).
type Filter struct{ terms []string }

// NewFilter parses a filter string.
func NewFilter(s string) Filter {
	return Filter{terms: strings.Fields(strings.ToLower(s))}
}

// Match reports whether the line passes the filter.
func (f Filter) Match(line string) bool {
	if len(f.terms) == 0 {
		return true
	}
	l := strings.ToLower(line)
	for _, t := range f.terms {
		if tokenRE.MatchString(t) {
			if !containsToken(l, t) {
				return false
			}
		} else if !strings.Contains(l, t) {
			return false
		}
	}
	return true
}

// tokenRE: "conn=12" or "op=3" must match whole tokens; a bare "op=" is a
// plain substring.
var tokenRE = regexp.MustCompile(`^(conn|op)=\d+$`)

func containsToken(line, tok string) bool {
	for i := 0; ; {
		j := strings.Index(line[i:], tok)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(tok)
		before := start == 0 || line[start-1] == ' '
		after := end == len(line) || line[end] == ' '
		if before && after {
			return true
		}
		i = end
	}
}

// Tail returns up to n matching lines from the end of the file, oldest first.
// It reads backwards in chunks and stops after scanning maxScan bytes, so a
// rare filter on a huge file stays bounded; truncated reports that.
func Tail(path string, n int, f Filter, maxScan int64) (lines []string, truncated bool, err error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = fh.Close() }()
	st, err := fh.Stat()
	if err != nil {
		return nil, false, err
	}
	const chunk = 64 * 1024
	pos := st.Size()
	var rest []byte // partial first line of the previously read chunk
	var scanned int64
	for pos > 0 && len(lines) < n {
		if scanned >= maxScan {
			truncated = true
			break
		}
		size := int64(chunk)
		if pos < size {
			size = pos
		}
		pos -= size
		buf := make([]byte, size, size+int64(len(rest)))
		if _, err := fh.ReadAt(buf, pos); err != nil && err != io.EOF {
			return nil, false, err
		}
		scanned += size
		buf = append(buf, rest...)
		parts := bytes.Split(buf, []byte{'\n'})
		rest = parts[0]
		for i := len(parts) - 1; i >= 1 && len(lines) < n; i-- {
			if line := string(parts[i]); line != "" && f.Match(line) {
				lines = append(lines, line)
			}
		}
	}
	if pos == 0 && len(lines) < n && len(rest) > 0 && f.Match(string(rest)) {
		lines = append(lines, string(rest))
	}
	slices.Reverse(lines)
	if lines == nil {
		lines = []string{}
	}
	return lines, truncated, nil
}

// Follow calls emit with batches of new matching lines appended to path,
// starting at the current end of the file. It notices rotation (the file
// shrinking or being replaced) and continues with the new file. After about
// 15 s without lines it calls emit(nil) as a keep-alive.
func Follow(ctx context.Context, path string, f Filter, interval time.Duration, emit func([]string) error) error {
	var (
		fh      *os.File
		offset  int64
		ino     uint64
		partial []byte
	)
	open := func(fromEnd bool) error {
		if fh != nil {
			_ = fh.Close()
		}
		var err error
		if fh, err = os.Open(path); err != nil {
			return err
		}
		st, err := fh.Stat()
		if err != nil {
			return err
		}
		ino = inode(st)
		offset = 0
		if fromEnd {
			offset = st.Size()
		}
		partial = nil
		return nil
	}
	if err := open(true); err != nil {
		return err
	}
	defer func() { _ = fh.Close() }()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	keepAlive := int(15 * time.Second / interval)
	idle := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		if idle++; idle >= keepAlive {
			idle = 0
			if err := emit(nil); err != nil {
				return err
			}
		}
		if st, err := os.Stat(path); err == nil && (inode(st) != ino || st.Size() < offset) {
			if err := open(false); err != nil {
				continue // being rotated: try again on the next tick
			}
		}
		st, err := fh.Stat()
		if err != nil || st.Size() <= offset {
			continue
		}
		buf := make([]byte, st.Size()-offset)
		n, err := fh.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			continue
		}
		offset += int64(n)
		data := append(partial, buf[:n]...)
		cut := bytes.LastIndexByte(data, '\n')
		if cut < 0 {
			partial = data
			continue
		}
		partial = append([]byte(nil), data[cut+1:]...)
		var batch []string
		for _, line := range strings.Split(string(data[:cut]), "\n") {
			if line != "" && f.Match(line) {
				batch = append(batch, line)
			}
		}
		if len(batch) > 0 {
			idle = 0
			if err := emit(batch); err != nil {
				return err
			}
		}
	}
}
