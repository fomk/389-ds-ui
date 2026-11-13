package certsource

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fomk/389-ds-ui/internal/certs"
)

type fakeStore struct {
	certs   []certs.Certificate
	deleted []string
	fail    error
}

func (s *fakeStore) List(context.Context) ([]certs.Certificate, error) {
	return append([]certs.Certificate{}, s.certs...), nil
}
func (s *fakeStore) ImportReplacing(_ context.Context, nick, certPEM, _, _ string) (string, error) {
	if s.fail != nil {
		return "", s.fail
	}
	b, _ := pem.Decode([]byte(certPEM))
	c, err := certs.Describe(b.Bytes)
	if err != nil {
		return "", err
	}
	c.HasKey = true
	for i := range s.certs {
		if s.certs[i].Subject == c.Subject {
			c.Nickname = s.certs[i].Nickname
			s.certs[i] = *c
			return c.Nickname, nil
		}
	}
	c.Nickname = nick
	s.certs = append(s.certs, *c)
	return nick, nil
}
func (s *fakeStore) Delete(_ context.Context, nick string) error {
	s.deleted = append(s.deleted, nick)
	for i, c := range s.certs {
		if c.Nickname == nick {
			s.certs = append(s.certs[:i], s.certs[i+1:]...)
			break
		}
	}
	return nil
}

type fakeServer struct {
	nick      string
	lockedKey bool
}

func (s *fakeServer) ServerCert(context.Context) (string, error)      { return s.nick, nil }
func (s *fakeServer) SetServerCert(_ context.Context, n string) error { s.nick = n; return nil }
func (s *fakeServer) CheckServerKey(_ context.Context, cur, next string) error {
	if s.lockedKey && cur != next {
		return errors.New("key change refused")
	}
	return nil
}

// writePair writes a self-signed certificate and its key; it returns the
// certificate file, the key file and the SHA-256 fingerprint.
func writePair(t *testing.T, dir, cn string, notBefore, notAfter time.Time) (string, string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: notBefore, NotAfter: notAfter, DNSNames: []string{cn}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kd, _ := x509.MarshalPKCS8PrivateKey(key)
	cf, kf := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	_ = os.WriteFile(cf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600)
	_ = os.WriteFile(kf, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}), 0o600)
	d, _ := certs.Describe(der)
	return cf, kf, d.SHA256
}

func TestImportAndSwitch(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	cf, kf, sum := writePair(t, dir, "ldap.example.org", now.Add(-time.Hour), now.Add(90*24*time.Hour))
	store := &fakeStore{certs: []certs.Certificate{{Nickname: "Server-Cert", Subject: "CN=container", HasKey: true, SHA256: "AA"}}}
	server := &fakeServer{nick: "Server-Cert"}
	var imported, recorded []string
	restarts := 0
	w := New(Config{CertFile: cf, KeyFile: kf, AutoRestart: true}, store, server, Hooks{
		Imported: func(n string) { imported = append(imported, n) },
		Record: func(_ context.Context, target string, err error) {
			recorded = append(recorded, target+":"+map[bool]string{true: "ok", false: "failed"}[err == nil])
		},
		Restart: func(context.Context) error { restarts++; return nil },
	}, nil)
	check := func(step string) Status {
		t.Helper()
		st, err := w.Check(context.Background())
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		return st
	}

	st := check("first import")
	first := server.nick
	if !strings.HasPrefix(first, NicknamePrefix) || st.Nickname != first || !st.InUse || st.Source.SHA256 != sum || st.LastImport == nil {
		t.Fatalf("after import: server %q, status %+v", first, st)
	}
	if len(imported) != 1 || len(recorded) != 1 || restarts != 1 {
		t.Errorf("hooks: imported %v, recorded %v, restarts %d", imported, recorded, restarts)
	}

	// Unchanged files: nothing happens.
	if st = check("unchanged"); !st.InUse || len(recorded) != 1 || restarts != 1 {
		t.Errorf("second check: %+v %v", st, recorded)
	}

	// Renewal (same subject): the certificate is replaced under its nickname.
	_, _, renewed := writePair(t, dir, "ldap.example.org", now.Add(-time.Hour), now.Add(90*24*time.Hour))
	if st = check("renewal"); server.nick != first || st.Source.SHA256 != renewed || len(imported) != 2 || restarts != 2 || len(store.deleted) != 0 {
		t.Errorf("renewal: server %q (first %q), status %+v, imported %v, deleted %v", server.nick, first, st, imported, store.deleted)
	}

	// Another name: a new nickname; the previous one stays until the next
	// import, as the running server still presents it.
	w.now = func() time.Time { return now.Add(time.Minute) }
	writePair(t, dir, "ldap2.example.org", now.Add(-time.Hour), now.Add(90*24*time.Hour))
	check("new subject")
	second := server.nick
	if second == first || len(store.deleted) != 0 {
		t.Errorf("new subject: server %q, deleted %v", second, store.deleted)
	}
	w.now = func() time.Time { return now.Add(2 * time.Minute) }
	writePair(t, dir, "ldap3.example.org", now.Add(-time.Hour), now.Add(90*24*time.Hour))
	check("third subject")
	if len(store.deleted) != 1 || store.deleted[0] != first || server.nick == second {
		t.Errorf("third import: deleted %v, server %q", store.deleted, server.nick)
	}
}

func TestRejectedFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	store := &fakeStore{}
	server := &fakeServer{nick: "Server-Cert"}

	// Expired certificate.
	cf, kf, _ := writePair(t, dir, "old.example.org", now.Add(-48*time.Hour), now.Add(-time.Hour))
	w := New(Config{CertFile: cf, KeyFile: kf}, store, server, Hooks{}, nil)
	if st, err := w.Check(context.Background()); err == nil || !strings.Contains(st.Error, "expired") || server.nick != "Server-Cert" {
		t.Errorf("expired: %v %+v", err, st)
	}

	// Key of another certificate.
	other := t.TempDir()
	_, otherKey, _ := writePair(t, other, "x", now.Add(-time.Hour), now.Add(time.Hour))
	cf, _, _ = writePair(t, dir, "new.example.org", now.Add(-time.Hour), now.Add(time.Hour))
	w = New(Config{CertFile: cf, KeyFile: otherKey}, store, server, Hooks{}, nil)
	if _, err := w.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("foreign key: %v", err)
	}

	// Attribute encryption pins the server key: a certificate of another key
	// is refused and nothing is imported.
	cf, kf, _ = writePair(t, dir, "new.example.org", now.Add(-time.Hour), now.Add(time.Hour))
	pinned := &fakeStore{certs: []certs.Certificate{{Nickname: "Server-Cert", HasKey: true, SHA256: "AA", PublicKey: "KEY"}}}
	locked := &fakeServer{nick: "Server-Cert", lockedKey: true}
	w = New(Config{CertFile: cf, KeyFile: kf}, pinned, locked, Hooks{}, nil)
	if _, err := w.Check(context.Background()); err == nil || locked.nick != "Server-Cert" || len(pinned.certs) != 1 {
		t.Errorf("key change with encryption: %v, server %q, certs %d", err, locked.nick, len(pinned.certs))
	}

	// Missing file.
	w = New(Config{CertFile: filepath.Join(dir, "nope.pem")}, store, server, Hooks{}, nil)
	if _, err := w.Check(context.Background()); err == nil {
		t.Error("missing file accepted")
	}

	// Import failure is recorded and leaves the server certificate alone.
	cf, kf, _ = writePair(t, dir, "new.example.org", now.Add(-time.Hour), now.Add(time.Hour))
	store.fail = errors.New("pk12util failed")
	failed := 0
	w = New(Config{CertFile: cf, KeyFile: kf}, store, server, Hooks{Record: func(_ context.Context, _ string, err error) {
		if err != nil {
			failed++
		}
	}}, nil)
	if _, err := w.Check(context.Background()); err == nil || failed != 1 || server.nick != "Server-Cert" {
		t.Errorf("import failure: %v, recorded %d, server %q", err, failed, server.nick)
	}
}

func TestCombinedFileWithChain(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	cf, kf, _ := writePair(t, dir, "leaf.example.org", now.Add(-time.Hour), now.Add(time.Hour))
	leaf, _ := os.ReadFile(cf)
	key, _ := os.ReadFile(kf)
	ca, _, _ := writePair(t, t.TempDir(), "ca.example.org", now.Add(-time.Hour), now.Add(time.Hour))
	caPEM, _ := os.ReadFile(ca)
	combined := filepath.Join(dir, "all.pem")
	_ = os.WriteFile(combined, append(append(append([]byte{}, leaf...), caPEM...), key...), 0o600)
	f, err := ReadFiles(combined, "")
	if err != nil {
		t.Fatal(err)
	}
	if f.Leaf.Subject.CommonName != "leaf.example.org" || !strings.Contains(f.Chain, "BEGIN CERTIFICATE") || strings.Count(f.Chain, "BEGIN CERTIFICATE") != 1 {
		t.Errorf("combined file: leaf %s, chain %q", f.Leaf.Subject, f.Chain)
	}
}
