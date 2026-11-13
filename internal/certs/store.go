// Package certs manages the NSS certificate database of 389-ds with the NSS
// tools (certutil, pk12util) and openssl. Commands run through a Runner:
// locally in the sidecar image, or inside a container in tests. Input
// reaches the tools through temporary files, never standard input.
package certs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	// ErrInvalid is returned for input the tools would reject.
	ErrInvalid = errors.New("invalid certificate input")
	// ErrNotFound is returned for an unknown nickname.
	ErrNotFound = errors.New("certificate not found")
	// ErrInUse protects the certificate the server is configured with.
	ErrInUse = errors.New("certificate is in use")
	// ErrTool reports a failing NSS or openssl command.
	ErrTool = errors.New("certificate tool failed")
)

// Runner executes the tools and provides temporary files they can read.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	// WriteTemp stores data in a file the tools can read and returns its
	// path and a function that removes it.
	WriteTemp(ctx context.Context, data []byte) (string, func(), error)
}

// LocalRunner runs the tools on this host.
type LocalRunner struct{}

// Run executes a command and returns its combined output.
func (LocalRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd.CombinedOutput()
}

// WriteTemp writes data to a private temporary file.
func (LocalRunner) WriteTemp(_ context.Context, data []byte) (string, func(), error) {
	dir, err := os.MkdirTemp("", "dsui-cert-")
	if err != nil {
		return "", nil, err
	}
	p := filepath.Join(dir, "data")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, err
	}
	return p, func() { _ = os.RemoveAll(dir) }, nil
}

// Store is an NSS database.
type Store struct {
	dir     string // NSS directory (cert9.db, key4.db)
	pinFile string // password file of the internal token
	r       Runner
}

// New returns a store for the NSS database in dir; the token password is in
// dir/pwdfile.txt as 389-ds keeps it.
func New(dir string, r Runner) *Store {
	return &Store{dir: dir, pinFile: filepath.Join(dir, "pwdfile.txt"), r: r}
}

func (s *Store) db() string { return "sql:" + s.dir }

func (s *Store) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, err := s.r.Run(ctx, name, args...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 600 {
			msg = msg[:600]
		}
		return out, fmt.Errorf("%w: %s: %s", ErrTool, name, msg)
	}
	return out, nil
}

// Certificate is a certificate in the database.
type Certificate struct {
	Nickname    string    `json:"nickname"`
	Trust       string    `json:"trust" doc:"NSS trust flags SSL,S/MIME,code: C trusted CA, T client CA, P trusted peer, u has a private key"`
	HasKey      bool      `json:"hasKey"`
	CA          bool      `json:"ca"`
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	SelfSigned  bool      `json:"selfSigned"`
	NotBefore   time.Time `json:"notBefore"`
	NotAfter    time.Time `json:"notAfter"`
	DNSNames    []string  `json:"dnsNames"`
	IPAddresses []string  `json:"ipAddresses"`
	Serial      string    `json:"serial"`
	KeyType     string    `json:"keyType"`
	SHA256      string    `json:"sha256" doc:"Fingerprint of the DER certificate"`
	PublicKey   string    `json:"publicKey" doc:"SHA-256 of the public key (SubjectPublicKeyInfo): equal for certificates of the same key"`
	PEM         string    `json:"pem,omitempty"`
}

var (
	listLine = regexp.MustCompile(`^(.+?)\s+([cCpPTu]*,[cCpPTu]*,[cCpPTu]*)\s*$`)
	keyLine  = regexp.MustCompile(`^<\s*\d+>\s+\S+\s+[0-9a-fA-F]+\s+(?:NSS Certificate DB:)?(.+)$`)
	trustRE  = regexp.MustCompile(`^[cCpPTu]*,[cCpPTu]*,[cCpPTu]*$`)
	nickRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._@()-]{0,63}$`)
)

func validNickname(n string) error {
	if !nickRE.MatchString(n) {
		return fmt.Errorf("%w: nickname %q (letters, digits, spaces, . _ @ ( ) -; up to 64)", ErrInvalid, n)
	}
	return nil
}

// Describe fills the X.509 details of a certificate from DER.
func Describe(der []byte) (*Certificate, error) {
	x, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	sum := sha256.Sum256(der)
	spki := sha256.Sum256(x.RawSubjectPublicKeyInfo)
	c := &Certificate{
		PublicKey: strings.ToUpper(hex.EncodeToString(spki[:])),
		Subject:   formatName(x.Subject), Issuer: formatName(x.Issuer), NotBefore: x.NotBefore, NotAfter: x.NotAfter,
		DNSNames: append([]string{}, x.DNSNames...), IPAddresses: []string{}, Serial: x.SerialNumber.Text(16),
		CA: x.IsCA, SHA256: strings.ToUpper(hex.EncodeToString(sum[:])),
		SelfSigned: bytesEqual(x.RawSubject, x.RawIssuer) && x.CheckSignature(x.SignatureAlgorithm, x.RawTBSCertificate, x.Signature) == nil,
		KeyType:    keyType(x),
		PEM:        string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
	for _, ip := range x.IPAddresses {
		c.IPAddresses = append(c.IPAddresses, ip.String())
	}
	return c, nil
}

// Short names for attribute types Go's pkix.Name.String prints as OIDs.
var nameOIDs = map[string]string{
	"2.5.4.3": "CN", "2.5.4.4": "SN", "2.5.4.5": "serialNumber", "2.5.4.6": "C", "2.5.4.7": "L", "2.5.4.8": "ST",
	"2.5.4.9": "street", "2.5.4.10": "O", "2.5.4.11": "OU", "2.5.4.12": "title", "2.5.4.42": "givenName",
	"2.5.4.43": "initials", "2.5.4.17": "postalCode", "0.9.2342.19200300.100.1.1": "UID",
	"0.9.2342.19200300.100.1.25": "DC", "1.2.840.113549.1.9.1": "emailAddress",
}

// formatName prints a distinguished name most specific first, with readable
// attribute names and values.
func formatName(n pkix.Name) string {
	parts := make([]string, 0, len(n.Names))
	for i := len(n.Names) - 1; i >= 0; i-- {
		a := n.Names[i]
		t, ok := nameOIDs[a.Type.String()]
		if !ok {
			t = a.Type.String()
		}
		parts = append(parts, t+"="+fmt.Sprint(a.Value))
	}
	if len(parts) == 0 {
		return n.String()
	}
	return strings.Join(parts, ",")
}

func keyType(x *x509.Certificate) string {
	switch x.PublicKeyAlgorithm {
	case x509.RSA:
		if k, ok := x.PublicKey.(interface{ Size() int }); ok {
			return fmt.Sprintf("RSA %d", k.Size()*8)
		}
		return "RSA"
	case x509.ECDSA:
		return "ECDSA"
	case x509.Ed25519:
		return "Ed25519"
	}
	return x.PublicKeyAlgorithm.String()
}

// nicknames lists nickname → trust.
func (s *Store) nicknames(ctx context.Context) ([][2]string, error) {
	out, err := s.run(ctx, "certutil", "-L", "-d", s.db())
	if err != nil {
		return nil, err
	}
	var list [][2]string
	for _, line := range strings.Split(string(out), "\n") {
		if m := listLine.FindStringSubmatch(strings.TrimRight(line, " ")); m != nil {
			list = append(list, [2]string{strings.TrimSpace(m[1]), m[2]})
		}
	}
	return list, nil
}

// keys lists the nicknames that have a private key.
func (s *Store) keys(ctx context.Context) (map[string]bool, error) {
	out, err := s.run(ctx, "certutil", "-K", "-d", s.db(), "-f", s.pinFile)
	if err != nil {
		// No keys at all is reported as an error by certutil.
		if strings.Contains(string(out), "no keys found") {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	keys := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if m := keyLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			keys[strings.TrimSpace(m[1])] = true
		}
	}
	return keys, nil
}

func (s *Store) pem(ctx context.Context, nick string) ([]byte, error) {
	out, err := s.run(ctx, "certutil", "-L", "-d", s.db(), "-n", nick, "-a")
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(out)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM for %s", ErrTool, nick)
	}
	return block.Bytes, nil
}

// List returns the certificates with their details.
func (s *Store) List(ctx context.Context) ([]Certificate, error) {
	nicks, err := s.nicknames(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Certificate, 0, len(nicks))
	for _, n := range nicks {
		der, err := s.pem(ctx, n[0])
		if err != nil {
			return nil, err
		}
		c, err := Describe(der)
		if err != nil {
			return nil, err
		}
		c.Nickname, c.Trust, c.HasKey = n[0], n[1], keys[n[0]]
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b Certificate) int {
		return strings.Compare(strings.ToLower(a.Nickname), strings.ToLower(b.Nickname))
	})
	return out, nil
}

// Get returns one certificate.
func (s *Store) Get(ctx context.Context, nick string) (*Certificate, error) {
	list, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Nickname == nick {
			return &list[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, nick)
}

func firstPEM(text string, typ string) ([]byte, error) {
	rest := []byte(text)
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			return nil, fmt.Errorf("%w: no %s block", ErrInvalid, typ)
		}
		if b.Type == typ {
			return b.Bytes, nil
		}
	}
}

func (s *Store) addCert(ctx context.Context, nick, trust, certPEM string) error {
	if err := validNickname(nick); err != nil {
		return err
	}
	der, err := firstPEM(certPEM, "CERTIFICATE")
	if err != nil {
		return err
	}
	if _, err := Describe(der); err != nil {
		return err
	}
	path, cleanup, err := s.r.WriteTemp(ctx, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	if err != nil {
		return err
	}
	defer cleanup()
	_, err = s.run(ctx, "certutil", "-A", "-d", s.db(), "-f", s.pinFile, "-n", nick, "-t", trust, "-a", "-i", path)
	return err
}

// ImportCA adds a trusted CA certificate (trust CT,, — server and client CA).
func (s *Store) ImportCA(ctx context.Context, nick, certPEM string) error {
	return s.addCert(ctx, nick, "CT,,", certPEM)
}

// ImportIssued adds a certificate issued for a key already in the database
// (the answer to a CSR made here).
func (s *Store) ImportIssued(ctx context.Context, nick, certPEM string) error {
	return s.addCert(ctx, nick, "u,u,u", certPEM)
}

// ImportPKCS12 imports a certificate with its key (and chain); it returns the
// nicknames that appeared.
func (s *Store) ImportPKCS12(ctx context.Context, data []byte, password string) ([]string, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty PKCS#12 file", ErrInvalid)
	}
	before, err := s.nicknames(ctx)
	if err != nil {
		return nil, err
	}
	p12, cleanP12, err := s.r.WriteTemp(ctx, data)
	if err != nil {
		return nil, err
	}
	defer cleanP12()
	pw, cleanPw, err := s.r.WriteTemp(ctx, []byte(password))
	if err != nil {
		return nil, err
	}
	defer cleanPw()
	if _, err := s.run(ctx, "pk12util", "-i", p12, "-d", s.db(), "-k", s.pinFile, "-w", pw); err != nil {
		return nil, err
	}
	after, err := s.nicknames(ctx)
	if err != nil {
		return nil, err
	}
	var added []string
	for _, n := range after {
		if !slices.ContainsFunc(before, func(b [2]string) bool { return b[0] == n[0] }) {
			added = append(added, n[0])
		}
	}
	return added, nil
}

// ImportPEMPair imports a certificate and its private key in PEM (with an
// optional CA chain) under a nickname, through a PKCS#12 bundle.
func (s *Store) ImportPEMPair(ctx context.Context, nick, certPEM, keyPEM, chainPEM string) ([]string, error) {
	if err := validNickname(nick); err != nil {
		return nil, err
	}
	if _, err := firstPEM(certPEM, "CERTIFICATE"); err != nil {
		return nil, err
	}
	if !strings.Contains(keyPEM, "PRIVATE KEY-----") {
		return nil, fmt.Errorf("%w: no PEM private key", ErrInvalid)
	}
	certPath, c1, err := s.r.WriteTemp(ctx, []byte(certPEM))
	if err != nil {
		return nil, err
	}
	defer c1()
	keyPath, c2, err := s.r.WriteTemp(ctx, []byte(keyPEM))
	if err != nil {
		return nil, err
	}
	defer c2()
	pass := make([]byte, 16)
	_, _ = rand.Read(pass)
	passText := hex.EncodeToString(pass)
	passPath, c3, err := s.r.WriteTemp(ctx, []byte(passText))
	if err != nil {
		return nil, err
	}
	defer c3()
	outPath, c4, err := s.r.WriteTemp(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer c4()
	args := []string{"pkcs12", "-export", "-in", certPath, "-inkey", keyPath, "-name", nick, "-out", outPath, "-passout", "file:" + passPath}
	if strings.TrimSpace(chainPEM) != "" {
		chainPath, c5, err := s.r.WriteTemp(ctx, []byte(chainPEM))
		if err != nil {
			return nil, err
		}
		defer c5()
		args = append(args, "-certfile", chainPath)
	}
	if _, err := s.run(ctx, "openssl", args...); err != nil {
		return nil, err
	}
	before, err := s.nicknames(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.run(ctx, "pk12util", "-i", outPath, "-d", s.db(), "-k", s.pinFile, "-w", passPath); err != nil {
		return nil, err
	}
	after, err := s.nicknames(ctx)
	if err != nil {
		return nil, err
	}
	var added []string
	for _, n := range after {
		if !slices.ContainsFunc(before, func(b [2]string) bool { return b[0] == n[0] }) {
			added = append(added, n[0])
		}
	}
	return added, nil
}

// ImportReplacing imports a certificate with its private key (and chain)
// for use as a server certificate and returns the nickname it got.
//
// NSS keeps one nickname per subject: a certificate with the subject of one
// already in the database joins that nickname whatever name is asked for, and
// 389-ds then picks the newest of them; certutil cannot delete an older one
// selectively. So a certificate with the same subject (a renewal) replaces
// the old one under its nickname: the old certificate and key are exported,
// removed, the new pair is imported, and on failure the old pair is put back.
// The running server keeps using the old pair it loaded until a restart.
func (s *Store) ImportReplacing(ctx context.Context, nick, certPEM, keyPEM, chainPEM string) (string, error) {
	der, err := firstPEM(certPEM, "CERTIFICATE")
	if err != nil {
		return "", err
	}
	desc, err := Describe(der)
	if err != nil {
		return "", err
	}
	list, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	same := ""
	for _, c := range list {
		if !c.CA && c.Subject == desc.Subject {
			same = c.Nickname
			break
		}
	}
	if same == "" {
		if _, err := s.ImportPEMPair(ctx, nick, certPEM, keyPEM, chainPEM); err != nil {
			return "", err
		}
		return nick, s.expect(ctx, nick, desc.SHA256)
	}

	// Keep the old pair to restore it if the import fails.
	passPath, c1, err := s.r.WriteTemp(ctx, []byte(randomPassword()))
	if err != nil {
		return "", err
	}
	defer c1()
	backup, c2, err := s.r.WriteTemp(ctx, nil)
	if err != nil {
		return "", err
	}
	defer c2()
	if _, err := s.run(ctx, "pk12util", "-o", backup, "-n", same, "-d", s.db(), "-k", s.pinFile, "-w", passPath); err != nil {
		return "", err
	}
	restore := func(cause error) error {
		_ = s.removeNickname(ctx, same)
		if _, err := s.run(ctx, "pk12util", "-i", backup, "-d", s.db(), "-k", s.pinFile, "-w", passPath); err != nil {
			return fmt.Errorf("%w; restoring the previous certificate %s also failed: %v", cause, same, err)
		}
		return cause
	}
	if err := s.removeNickname(ctx, same); err != nil {
		return "", restore(err)
	}
	if _, err := s.ImportPEMPair(ctx, same, certPEM, keyPEM, chainPEM); err != nil {
		return "", restore(err)
	}
	if err := s.expect(ctx, same, desc.SHA256); err != nil {
		return "", restore(err)
	}
	return same, nil
}

// expect checks that nick holds the certificate with the fingerprint.
func (s *Store) expect(ctx context.Context, nick, sha string) error {
	c, err := s.Get(ctx, nick)
	if err != nil {
		return err
	}
	if !c.HasKey || !strings.EqualFold(c.SHA256, sha) {
		return fmt.Errorf("%w: after the import %s holds another certificate (%s)", ErrTool, nick, c.SHA256)
	}
	return nil
}

// removeNickname deletes every certificate and private key under nick
// (certutil removes one certificate per call, the newest first).
func (s *Store) removeNickname(ctx context.Context, nick string) error {
	for range 16 {
		nicks, err := s.nicknames(ctx)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(nicks, func(n [2]string) bool { return n[0] == nick }) {
			break
		}
		keys, err := s.keys(ctx)
		if err != nil {
			return err
		}
		flag := "-D"
		if keys[nick] {
			flag = "-F"
		}
		if _, err := s.run(ctx, "certutil", flag, "-d", s.db(), "-f", s.pinFile, "-n", nick); err != nil {
			return err
		}
	}
	// Keys left without their certificate keep the label.
	ids, err := s.keyIDs(ctx, nick)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.run(ctx, "certutil", "-F", "-d", s.db(), "-f", s.pinFile, "-k", id); err != nil {
			return err
		}
	}
	return nil
}

var keyIDLine = regexp.MustCompile(`^<\s*\d+>\s+\S+\s+([0-9a-fA-F]+)\s+(?:NSS Certificate DB:)?(.+)$`)

// keyIDs returns the ids of the private keys labelled nick.
func (s *Store) keyIDs(ctx context.Context, nick string) ([]string, error) {
	out, err := s.run(ctx, "certutil", "-K", "-d", s.db(), "-f", s.pinFile)
	if err != nil {
		if strings.Contains(string(out), "no keys found") {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, line := range strings.Split(string(out), "\n") {
		if m := keyIDLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil && strings.TrimSpace(m[2]) == nick {
			ids = append(ids, m[1])
		}
	}
	return ids, nil
}

func randomPassword() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CSRRequest describes a key and certificate request made in the database.
type CSRRequest struct {
	Subject  string   `json:"subject" minLength:"3" doc:"e.g. CN=ldap.example.org,O=Example"`
	DNSNames []string `json:"dnsNames,omitempty"`
	KeyType  string   `json:"keyType,omitempty" enum:"rsa2048,rsa3072,rsa4096,ec256,ec384" doc:"Default rsa3072"`
}

var dnsRE = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]{0,62})(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}))*$`)

// CreateCSR generates a key in the database and returns a PEM certificate
// request for it. Import the issued certificate with ImportIssued.
func (s *Store) CreateCSR(ctx context.Context, req CSRRequest) (string, error) {
	subject := strings.TrimSpace(req.Subject)
	if !strings.Contains(subject, "=") || strings.ContainsAny(subject, "\n\"") {
		return "", fmt.Errorf("%w: subject %q", ErrInvalid, req.Subject)
	}
	var sans []string
	for _, d := range req.DNSNames {
		if d = strings.TrimSpace(d); d == "" {
			continue
		}
		if !dnsRE.MatchString(d) {
			return "", fmt.Errorf("%w: DNS name %q", ErrInvalid, d)
		}
		sans = append(sans, d)
	}
	noise := make([]byte, 2048)
	_, _ = rand.Read(noise)
	noisePath, clean, err := s.r.WriteTemp(ctx, noise)
	if err != nil {
		return "", err
	}
	defer clean()
	args := []string{"-R", "-d", s.db(), "-f", s.pinFile, "-z", noisePath, "-s", subject, "-a"}
	switch req.KeyType {
	case "", "rsa3072":
		args = append(args, "-k", "rsa", "-g", "3072")
	case "rsa2048":
		args = append(args, "-k", "rsa", "-g", "2048")
	case "rsa4096":
		args = append(args, "-k", "rsa", "-g", "4096")
	case "ec256":
		args = append(args, "-k", "ec", "-q", "nistp256")
	case "ec384":
		args = append(args, "-k", "ec", "-q", "nistp384")
	default:
		return "", fmt.Errorf("%w: key type %q", ErrInvalid, req.KeyType)
	}
	if len(sans) > 0 {
		args = append(args, "-8", strings.Join(sans, ","))
	}
	out, err := s.run(ctx, "certutil", args...)
	if err != nil {
		return "", err
	}
	text := string(out)
	i := strings.Index(text, "-----BEGIN")
	j := strings.LastIndex(text, "-----END")
	if i < 0 || j < 0 {
		return "", fmt.Errorf("%w: certutil printed no request", ErrTool)
	}
	end := strings.Index(text[j:], "\n")
	if end < 0 {
		end = len(text) - j
	}
	csr := strings.TrimSpace(text[i : j+end])
	// certutil writes "NEW CERTIFICATE REQUEST"; most CAs also take that.
	csr = strings.ReplaceAll(csr, "NEW CERTIFICATE REQUEST", "CERTIFICATE REQUEST")
	return csr + "\n", nil
}

// SetTrust changes the trust flags of a certificate.
func (s *Store) SetTrust(ctx context.Context, nick, trust string) error {
	if !trustRE.MatchString(trust) {
		return fmt.Errorf("%w: trust %q (e.g. CT,, or ,,)", ErrInvalid, trust)
	}
	if _, err := s.Get(ctx, nick); err != nil {
		return err
	}
	_, err := s.run(ctx, "certutil", "-M", "-d", s.db(), "-f", s.pinFile, "-n", nick, "-t", trust)
	return err
}

// Delete removes a certificate (and its private key, if any).
func (s *Store) Delete(ctx context.Context, nick string) error {
	c, err := s.Get(ctx, nick)
	if err != nil {
		return err
	}
	if c.HasKey {
		_, err = s.run(ctx, "certutil", "-F", "-d", s.db(), "-f", s.pinFile, "-n", nick)
	} else {
		_, err = s.run(ctx, "certutil", "-D", "-d", s.db(), "-f", s.pinFile, "-n", nick)
	}
	return err
}

func bytesEqual(a, b []byte) bool { return string(a) == string(b) }
