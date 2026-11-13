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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/network"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/certs"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/pages"
	"github.com/fomk/389-ds-ui/internal/providers"
)

// containerRunner runs the certificate tools inside the DS container as the
// server's user, like the sidecar does.
type containerRunner struct{ ct testcontainers.Container }

var tmpSeq atomic.Int64

func (r containerRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	code, out, err := r.ct.Exec(ctx, append([]string{name}, args...), tcexec.Multiplexed(), tcexec.WithUser("dirsrv"))
	if err != nil {
		return nil, err
	}
	b, _ := io.ReadAll(out)
	if code != 0 {
		return b, fmt.Errorf("exit %d", code)
	}
	return b, nil
}

func (r containerRunner) WriteTemp(ctx context.Context, data []byte) (string, func(), error) {
	p := fmt.Sprintf("/tmp/dsui-itest-%d", tmpSeq.Add(1))
	// Copied as root: writable for dirsrv, as an output file may be.
	if err := r.ct.CopyToContainer(ctx, data, p, 0o666); err != nil {
		return "", nil, err
	}
	return p, func() { _, _, _ = r.ct.Exec(context.Background(), []string{"rm", "-f", p}) }, nil
}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  string
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ds-ui itest CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &testCA{cert: cert, key: key, pem: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

// sign issues a server certificate for a CSR.
func (ca *testCA) sign(t *testing.T, csrPEM string) (string, string) {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		t.Fatalf("CSR is not PEM: %q", csrPEM)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		t.Fatalf("CSR: %v", err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: csr.Subject, DNSNames: csr.DNSNames,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca.cert, csr.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), strings.ToUpper(hex.EncodeToString(sum[:]))
}

// selfSignedPair makes a certificate with its PEM private key.
func selfSignedPair(t *testing.T, cn string) (string, string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: cn}, DNSNames: []string{cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	kder, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kder}))
}

// TestReplaceServerCertificate replaces the server certificate and sets the
// TLS minimum only through the API, restarts the server and checks what it
// presents on LDAPS.
func TestReplaceServerCertificate(t *testing.T) {
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nw.Remove(context.Background()) })
	n := startNode(t, ctx, nw, "tlsds")
	if n == nil {
		t.FailNow()
	}
	defs, err := pages.Load()
	if err != nil {
		t.Fatal(err)
	}
	store := certs.New("/data/config", containerRunner{n.ct})
	bus := &events.Bus{}
	restart := &ds.RestartRegistry{}
	h := api.NewHandler(api.Deps{LDAP: n.c, Pages: pages.New(defs, nil), Certs: store, Restart: restart,
		Tasks: ds.NewTasks(n.c, bus, nil), Events: bus, Lifecycle: providers.NoLifecycle{}})
	do := func(method, path, body string) (int, string) {
		rec := serve(h, httptestRequest(method, path, body))
		return rec.Code, rec.Body.String()
	}

	// What the fresh container has.
	code, body := do("GET", "/api/v1/security/certs", "")
	var list api.CertList
	if code != 200 || json.Unmarshal([]byte(body), &list) != nil {
		t.Fatalf("list: %d %s", code, body)
	}
	if list.ServerCert != "Server-Cert" || !slices.ContainsFunc(list.Certificates, func(c certs.Certificate) bool { return c.Nickname == "Server-Cert" && c.HasKey }) {
		t.Fatalf("initial certs: %+v", list)
	}

	// Our CA, a CSR made in the NSS database, the issued certificate.
	ca := newTestCA(t)
	if code, body := do("POST", "/api/v1/security/certs/ca", js(map[string]string{"nickname": "itest CA", "pem": ca.pem})); code != 204 {
		t.Fatalf("import CA: %d %s", code, body)
	}
	code, body = do("POST", "/api/v1/security/csr", js(map[string]any{"subject": "CN=ldap.itest.example,O=ds-ui", "dnsNames": []string{"ldap.itest.example", "tlsds"}, "keyType": "ec256"}))
	var csr api.CSR
	if code != 200 || json.Unmarshal([]byte(body), &csr) != nil {
		t.Fatalf("CSR: %d %s", code, body)
	}
	issued, fingerprint := ca.sign(t, csr.PEM)
	if code, body := do("POST", "/api/v1/security/certs/issued", js(map[string]string{"nickname": "itest-server", "pem": issued})); code != 204 {
		t.Fatalf("import issued: %d %s", code, body)
	}
	code, body = do("GET", "/api/v1/security/certs/itest-server", "")
	var got certs.Certificate
	if code != 200 || json.Unmarshal([]byte(body), &got) != nil || !got.HasKey || got.SHA256 != fingerprint || !slices.Contains(got.DNSNames, "tlsds") {
		t.Fatalf("issued cert in the database: %d %s", code, body)
	}

	// A PEM certificate with its key, then trust and delete.
	pc, pk := selfSignedPair(t, "pair.itest.example")
	code, body = do("POST", "/api/v1/security/certs/pem-pair", js(map[string]string{"nickname": "itest pair", "cert": pc, "key": pk}))
	if code != 200 || !strings.Contains(body, "itest pair") {
		t.Errorf("PEM pair: %d %s", code, body)
	}
	if code, body := do("GET", "/api/v1/security/certs/itest%20pair", ""); code != 200 || !strings.Contains(body, `"hasKey":true`) {
		t.Errorf("PEM pair key: %d %s", code, body)
	}
	if code, body := do("PATCH", "/api/v1/security/certs/itest%20CA", `{"trust":"C,,"}`); code != 204 {
		t.Errorf("trust: %d %s", code, body)
	}
	if code, _ := do("PATCH", "/api/v1/security/certs/itest%20CA", `{"trust":"XYZ"}`); code != 422 {
		t.Errorf("bad trust: %d", code)
	}
	if code, body := do("DELETE", "/api/v1/security/certs/itest%20pair", ""); code != 204 {
		t.Errorf("delete pair: %d %s", code, body)
	}
	if code, _ := do("DELETE", "/api/v1/security/certs/Server-Cert", ""); code != 409 {
		t.Errorf("delete the server certificate: %d", code)
	}
	if code, _ := do("POST", "/api/v1/security/certs/ca", js(map[string]string{"nickname": "-bad", "pem": ca.pem})); code != 422 {
		t.Errorf("bad nickname: %d", code)
	}

	// Switch the server certificate and require TLS 1.3, then restart.
	if code, body := do("PUT", "/api/v1/security/server-cert", `{"nickname":"itest-server"}`); code != 204 {
		t.Fatalf("server cert: %d %s", code, body)
	}
	if code, body := do("PATCH", "/api/v1/pages/tls-protocol", `{"values":{"sslVersionMin":["TLS1.3"]}}`); code != 200 {
		t.Fatalf("TLS minimum: %d %s", code, body)
	}
	if r := restart.Reasons(); !slices.Contains(r, "nsSSLPersonalitySSL") || !slices.Contains(r, "sslVersionMin") {
		t.Errorf("restart reasons = %v", r)
	}
	stop := 30 * time.Second
	if err := n.ct.Stop(ctx, &stop); err != nil {
		t.Fatal(err)
	}
	if err := n.ct.Start(ctx); err != nil {
		t.Fatal(err)
	}
	ldaps, err := n.ct.PortEndpoint(ctx, "3636/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	var p *certs.Presented
	waitFor(t, "LDAPS after restart", 90*time.Second, func() bool {
		p, err = certs.Handshake(ctx, ldaps)
		return err == nil
	})
	if len(p.Chain) == 0 || p.Chain[0].SHA256 != fingerprint || p.Version != "TLS 1.3" {
		t.Errorf("presented: version %s, leaf %+v; want %s", p.Version, p.Chain, fingerprint)
	}
}
