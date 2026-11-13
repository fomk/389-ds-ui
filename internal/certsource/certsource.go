// Package certsource keeps the server certificate in step with PEM files
// maintained by an external tool (certbot, lego, cert-manager, a deploy
// script). It checks the files periodically; when they hold a certificate the
// server is not configured with, it imports it into the NSS database under a
// new nickname, makes it the server certificate and, if allowed, restarts the
// server. Obtaining and renewing certificates stays with the external tool.
package certsource

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fomk/389-ds-ui/internal/certs"
)

// NicknamePrefix marks certificates this package imported; older ones are
// removed once they are no longer in use.
const NicknamePrefix = "Server-Cert-auto-"

// Config describes where the files are and what to do with them.
type Config struct {
	CertFile    string        // leaf certificate first, then the chain
	KeyFile     string        // private key; empty: in CertFile
	Interval    time.Duration // how often the files are checked
	AutoRestart bool          // restart the server after an import
}

// Store is the part of the NSS database the watcher needs.
type Store interface {
	List(ctx context.Context) ([]certs.Certificate, error)
	// ImportReplacing imports the pair; a certificate with the same subject
	// is replaced under its nickname. It returns the nickname used.
	ImportReplacing(ctx context.Context, nick, certPEM, keyPEM, chainPEM string) (string, error)
	Delete(ctx context.Context, nick string) error
}

// Server reads and sets the server certificate (nsSSLPersonalitySSL).
type Server interface {
	ServerCert(ctx context.Context) (string, error)
	SetServerCert(ctx context.Context, nick string) error
}

// Hooks connect the watcher to the rest of ds-ui; nil functions are skipped.
type Hooks struct {
	// Imported is called after a new server certificate is in place; the
	// server presents it after a restart.
	Imported func(nick string)
	// Record adds an import to the task history.
	Record func(ctx context.Context, target string, err error)
	// Restart restarts the server (only with AutoRestart).
	Restart func(ctx context.Context) error
}

// Status is the state shown in the UI.
type Status struct {
	Enabled     bool               `json:"enabled"`
	CertFile    string             `json:"certFile,omitempty"`
	KeyFile     string             `json:"keyFile,omitempty"`
	Interval    string             `json:"interval,omitempty" doc:"Check interval, e.g. 10m0s"`
	AutoRestart bool               `json:"autoRestart"`
	LastCheck   *time.Time         `json:"lastCheck,omitempty"`
	LastImport  *time.Time         `json:"lastImport,omitempty"`
	Error       string             `json:"error,omitempty" doc:"Why the last check failed"`
	Source      *certs.Certificate `json:"source,omitempty" doc:"Certificate in the files"`
	Nickname    string             `json:"nickname,omitempty" doc:"Nickname of that certificate in the database"`
	InUse       bool               `json:"inUse" doc:"The server is configured with the certificate from the files"`
}

// Watcher checks the files and imports new certificates.
type Watcher struct {
	cfg    Config
	store  Store
	server Server
	hooks  Hooks
	log    *slog.Logger
	now    func() time.Time

	run    sync.Mutex // one check at a time
	mu     sync.Mutex
	status Status
}

// New creates a watcher; Run starts the periodic checks.
func New(cfg Config, store Store, server Server, hooks Hooks, log *slog.Logger) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Minute
	}
	return &Watcher{cfg: cfg, store: store, server: server, hooks: hooks, log: log, now: time.Now,
		status: Status{Enabled: true, CertFile: cfg.CertFile, KeyFile: cfg.KeyFile, Interval: cfg.Interval.String(), AutoRestart: cfg.AutoRestart}}
}

// Run checks the files now and then every interval until ctx ends.
func (w *Watcher) Run(ctx context.Context) {
	t := time.NewTicker(w.cfg.Interval)
	defer t.Stop()
	for {
		if _, err := w.Check(ctx); err != nil && ctx.Err() == nil {
			w.log.Warn("certificate files", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Status returns the result of the last check.
func (w *Watcher) Status() Status {
	if w == nil {
		return Status{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.status
	return s
}

// Check reads the files once and imports their certificate if the server
// does not use it yet.
func (w *Watcher) Check(ctx context.Context) (Status, error) {
	w.run.Lock()
	defer w.run.Unlock()
	now := w.now().UTC()
	src, imported, err := w.check(ctx, now)
	w.mu.Lock()
	w.status.LastCheck = &now
	w.status.Error = ""
	if err != nil {
		w.status.Error = err.Error()
	}
	// Without readable files there is nothing to describe.
	w.status.Source, w.status.Nickname, w.status.InUse = nil, "", false
	if src != nil {
		w.status.Source = src.cert
		w.status.Nickname = src.nick
		w.status.InUse = src.inUse
	}
	if imported {
		w.status.LastImport = &now
	}
	s := w.status
	w.mu.Unlock()
	return s, err
}

type source struct {
	cert  *certs.Certificate
	nick  string
	inUse bool
}

// Files is the parsed content of the certificate and key files.
type Files struct {
	Leaf    *x509.Certificate
	LeafPEM string
	Chain   string // the remaining certificates
	Key     string
}

// ReadFiles reads and validates the certificate and key files: the key must
// belong to the first certificate.
func ReadFiles(certFile, keyFile string) (*Files, error) {
	certData, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}
	keyData := certData
	if keyFile != "" && keyFile != certFile {
		if keyData, err = os.ReadFile(keyFile); err != nil {
			return nil, fmt.Errorf("read private key: %w", err)
		}
	}
	var blocks []*pem.Block
	for rest := certData; ; {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			break
		}
		if b.Type == "CERTIFICATE" {
			blocks = append(blocks, b)
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%s: no PEM certificate", certFile)
	}
	var key *pem.Block
	for rest := keyData; ; {
		var b *pem.Block
		if b, rest = pem.Decode(rest); b == nil {
			break
		}
		if strings.HasSuffix(b.Type, "PRIVATE KEY") {
			key = b
			break
		}
	}
	if key == nil {
		return nil, errors.New("no PEM private key")
	}
	if strings.Contains(key.Type, "ENCRYPTED") || key.Headers["Proc-Type"] != "" {
		return nil, errors.New("the private key is encrypted; provide it without a passphrase")
	}
	f := &Files{LeafPEM: string(pem.EncodeToMemory(blocks[0])), Key: string(pem.EncodeToMemory(key))}
	if _, err := tls.X509KeyPair([]byte(f.LeafPEM), []byte(f.Key)); err != nil {
		return nil, errors.New("the private key does not match the certificate")
	}
	if f.Leaf, err = x509.ParseCertificate(blocks[0].Bytes); err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	for _, b := range blocks[1:] {
		f.Chain += string(pem.EncodeToMemory(b))
	}
	return f, nil
}

func (w *Watcher) check(ctx context.Context, now time.Time) (*source, bool, error) {
	f, err := ReadFiles(w.cfg.CertFile, w.cfg.KeyFile)
	if err != nil {
		return nil, false, err
	}
	desc, err := certs.Describe(f.Leaf.Raw)
	if err != nil {
		return nil, false, err
	}
	desc.PEM = ""
	src := &source{cert: desc}
	sum := desc.SHA256

	current, err := w.server.ServerCert(ctx)
	if err != nil {
		return src, false, fmt.Errorf("read the server certificate setting: %w", err)
	}
	list, err := w.store.List(ctx)
	if err != nil {
		return src, false, err
	}
	for _, c := range list {
		if c.HasKey && strings.EqualFold(c.SHA256, sum) {
			if c.Nickname == current {
				src.nick, src.inUse = c.Nickname, true
				return src, false, nil
			}
			if src.nick == "" || strings.HasPrefix(c.Nickname, NicknamePrefix) {
				src.nick = c.Nickname
			}
		}
	}
	if now.Before(f.Leaf.NotBefore) {
		return src, false, fmt.Errorf("the certificate in the files is valid only from %s", f.Leaf.NotBefore.UTC().Format(time.RFC3339))
	}
	if now.After(f.Leaf.NotAfter) {
		return src, false, fmt.Errorf("the certificate in the files expired on %s", f.Leaf.NotAfter.UTC().Format(time.RFC3339))
	}

	// Import (unless the same certificate is already in the database under
	// another name) and switch to it.
	nick := src.nick
	if nick == "" {
		if nick, err = w.store.ImportReplacing(ctx, NicknamePrefix+now.Format("20060102-150405"), f.LeafPEM, f.Key, f.Chain); err != nil {
			w.record(ctx, desc.Subject, err)
			return src, false, err
		}
	}
	if nick != current {
		if err := w.server.SetServerCert(ctx, nick); err != nil {
			w.record(ctx, nick, err)
			return src, false, err
		}
	}
	src.nick, src.inUse = nick, true
	if w.hooks.Imported != nil {
		w.hooks.Imported(nick)
	}
	w.record(ctx, nick, nil)
	w.log.Info("server certificate imported from files", "nickname", nick, "subject", desc.Subject, "notAfter", desc.NotAfter)

	// Earlier automatic imports are no longer needed. The previous server
	// certificate stays: the running server presents it until the restart.
	for _, c := range list {
		if strings.HasPrefix(c.Nickname, NicknamePrefix) && c.Nickname != nick && c.Nickname != current {
			if err := w.store.Delete(ctx, c.Nickname); err != nil {
				w.log.Warn("remove an old imported certificate", "nickname", c.Nickname, "err", err)
			}
		}
	}

	if w.cfg.AutoRestart && w.hooks.Restart != nil {
		if err := w.hooks.Restart(ctx); err != nil {
			return src, true, fmt.Errorf("imported as %s, but the restart failed: %w", nick, err)
		}
		w.log.Info("directory server restarted to present the new certificate", "nickname", nick)
	}
	return src, true, nil
}

func (w *Watcher) record(ctx context.Context, target string, err error) {
	if w.hooks.Record != nil {
		w.hooks.Record(ctx, target, err)
	}
}
