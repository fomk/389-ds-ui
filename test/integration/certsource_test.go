//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/certsource"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx"
)

// issue makes a server certificate with a new key, signed by the CA.
func (ca *testCA) issue(t *testing.T, cn string) (certPEM, keyPEM, sum string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn}, DNSNames: []string{cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalPKCS8PrivateKey(key)
	s := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder})), strings.ToUpper(hex.EncodeToString(s[:]))
}

// currentLDAP connects to the container's LDAP port as it is mapped now (the
// mapping changes when the container restarts).
type currentLDAP struct{ ct testcontainers.Container }

func (l currentLDAP) Do(ctx context.Context, fn func(*ldap.Conn) error) error {
	ep, err := l.ct.PortEndpoint(ctx, "3389/tcp", "ldap")
	if err != nil {
		return err
	}
	c, err := ldapx.New(ldapx.Options{URL: ep, BindDN: dmDN, BindPassword: dmPassword, Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return c.Do(ctx, fn)
}

// TestCertificateFromFiles follows certificate files the way certbot or
// cert-manager leave them (leaf + chain, separate key): the certificate is
// imported, becomes the server certificate and is presented after the restart.
func TestCertificateFromFiles(t *testing.T) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(context.Background()) })
	n := startNode(t, ctx, nw, "filesds")
	if n == nil {
		t.FailNow()
	}
	// The mapped port changes when the container restarts.
	presented := func() string {
		var p *certs.Presented
		waitFor(t, "LDAPS", 90*time.Second, func() bool {
			ldaps, err := n.ct.PortEndpoint(ctx, "3636/tcp", "")
			if err != nil {
				return false
			}
			p, err = certs.Handshake(ctx, ldaps)
			return err == nil
		})
		return p.Chain[0].SHA256
	}
	original := presented()

	ca := newTestCA(t)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	write := func(cn string) string {
		c, k, sum := ca.issue(t, cn)
		if err := os.WriteFile(certFile, []byte(c+ca.pem), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyFile, []byte(k), 0o600); err != nil {
			t.Fatal(err)
		}
		return sum
	}

	store := certs.New("/data/config", containerRunner{n.ct})
	tasks := ds.NewTasks(n.c, &events.Bus{}, nil)
	reg := &ds.RestartRegistry{}
	var beforeRestart string
	restart := func(ctx context.Context) error {
		// What the running server presents right after the switch: 389-ds
		// reads the NSS database only at startup.
		beforeRestart = presented()
		stop := 30 * time.Second
		if err := n.ct.Stop(ctx, &stop); err != nil {
			return err
		}
		return n.ct.Start(ctx)
	}
	w := certsource.New(certsource.Config{CertFile: certFile, KeyFile: keyFile, AutoRestart: true}, store, ds.ServerCert{LDAP: currentLDAP{n.ct}},
		certsource.Hooks{
			Imported: func(string) { reg.Require("nsSSLPersonalitySSL") },
			Record: func(ctx context.Context, target string, err error) {
				tasks.Record(ctx, ds.OpCertFromFiles, target, err)
			},
			Restart: restart,
		}, nil)

	first := write("filesds")
	st, err := w.Check(ctx)
	if err != nil {
		t.Fatalf("check: %v (%+v)", err, st)
	}
	if !st.InUse || !strings.HasPrefix(st.Nickname, certsource.NicknamePrefix) || st.Source.SHA256 != first {
		t.Fatalf("status: %+v", st)
	}
	if beforeRestart != original {
		t.Errorf("the running server switched certificates without a restart (presented %s, original %s)", beforeRestart, original)
	}
	if got := presented(); got != first {
		t.Errorf("after restart the server presents %s, want %s", got, first)
	}
	list := tasks.List()
	if len(list) == 0 || list[len(list)-1].Type != ds.OpCertFromFiles || list[len(list)-1].Status != ds.TaskSucceeded {
		t.Errorf("history: %+v", list)
	}

	// Unchanged files: no import, no restart.
	beforeRestart = ""
	if st, err = w.Check(ctx); err != nil || !st.InUse || beforeRestart != "" {
		t.Errorf("unchanged files: %v %+v restarted=%v", err, st, beforeRestart != "")
	}

	// Renewal: the same subject, so NSS would file the new certificate under
	// the old nickname; it replaces the old one there, nothing piles up.
	firstNick := st.Nickname
	second := write("filesds")
	if st, err = w.Check(ctx); err != nil || st.Nickname != firstNick || st.Source.SHA256 != second {
		t.Fatalf("renewal: %v %+v", err, st)
	}
	if got := presented(); got != second {
		t.Errorf("after renewal the server presents %s, want %s", got, second)
	}
	_, out, err := n.ct.Exec(ctx, []string{"sh", "-c", "certutil -L -d sql:/data/config; certutil -K -d sql:/data/config -f /data/config/pwdfile.txt"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(out)
	if c := strings.Count(string(b), firstNick); c != 2 { // one certificate, one key
		t.Errorf("%s appears %d times in the database:\n%s", firstNick, c, b)
	}
}
