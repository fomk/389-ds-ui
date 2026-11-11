// Package ldapx is the generic LDAP layer: connection management, binding and
// helpers. It knows nothing about 389 Directory Server specifics.
package ldapx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// AuthMethod describes how the client authenticates to the server.
type AuthMethod string

const (
	AuthExternal  AuthMethod = "sasl-external" // LDAPI autobind
	AuthSimple    AuthMethod = "simple"
	AuthAnonymous AuthMethod = "anonymous"
)

// Options configure a Client.
type Options struct {
	URL          string
	BindDN       string
	BindPassword string
	CAFile       string
	Timeout      time.Duration
	Logger       *slog.Logger
}

// Client holds one shared LDAP connection and re-establishes it on demand.
// go-ldap connections are safe for concurrent use, so a single connection is
// enough for an admin UI; this can be replaced by a pool without API changes.
type Client struct {
	opts   Options
	scheme string
	auth   AuthMethod
	tls    *tls.Config
	log    *slog.Logger

	mu   sync.Mutex
	conn *ldap.Conn
}

// New validates options; it does not connect.
func New(opts Options) (*Client, error) {
	scheme, _, ok := strings.Cut(opts.URL, "://")
	if !ok {
		return nil, fmt.Errorf("LDAP URL %q has no scheme", opts.URL)
	}
	c := &Client{opts: opts, scheme: strings.ToLower(scheme), log: opts.Logger}
	if c.log == nil {
		c.log = slog.Default()
	}
	if c.opts.Timeout <= 0 {
		c.opts.Timeout = 10 * time.Second
	}

	switch c.scheme {
	case "ldapi":
		c.auth = AuthExternal
		if opts.BindDN != "" {
			c.auth = AuthSimple
		}
	case "ldap", "ldaps":
		// ldapi URLs carry a percent-encoded socket path in the host part,
		// which net/url rejects, so only network URLs are parsed here.
		u, err := url.Parse(opts.URL)
		if err != nil {
			return nil, fmt.Errorf("parse LDAP URL: %w", err)
		}
		c.auth = AuthAnonymous
		if opts.BindDN != "" {
			c.auth = AuthSimple
		}
		c.tls = &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}
		if opts.CAFile != "" {
			pem, err := os.ReadFile(opts.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read CA file: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("CA file %s contains no certificates", opts.CAFile)
			}
			c.tls.RootCAs = pool
		}
	default:
		return nil, fmt.Errorf("unsupported LDAP URL scheme %q", scheme)
	}
	return c, nil
}

// Auth reports the authentication method in use.
func (c *Client) Auth() AuthMethod { return c.auth }

// BindDN is the DN of simple binds ("" for EXTERNAL and anonymous).
func (c *Client) BindDN() string {
	if c.auth != AuthSimple {
		return ""
	}
	return c.opts.BindDN
}

// SetBindPassword replaces the password for future simple binds, e.g. after
// ds-ui changed the root DN password it binds with. The open connection stays
// bound.
func (c *Client) SetBindPassword(pw string) {
	c.mu.Lock()
	c.opts.BindPassword = pw
	c.mu.Unlock()
}

// URL returns the configured server URL.
func (c *Client) URL() string { return c.opts.URL }

// Do runs fn with a live connection. If fn fails because the connection was
// lost, the connection is re-established and fn is retried once.
func (c *Client) Do(ctx context.Context, fn func(*ldap.Conn) error) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := c.get()
		if err != nil {
			return err
		}
		err = fn(conn)
		if err == nil || !IsConnError(err) || attempt > 0 {
			return err
		}
		c.log.Debug("ldap connection lost, reconnecting", "err", err)
		c.drop(conn)
	}
}

// Close closes the shared connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *Client) get() (*ldap.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosing() {
		return c.conn, nil
	}
	conn, err := c.dial()
	if err != nil {
		return nil, err
	}
	c.conn = conn
	return conn, nil
}

func (c *Client) drop(conn *ldap.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == conn {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *Client) dial() (*ldap.Conn, error) {
	conn, err := c.connect()
	if err != nil {
		return nil, err
	}
	switch c.auth {
	case AuthExternal:
		err = conn.ExternalBind()
	case AuthSimple:
		err = conn.Bind(c.opts.BindDN, c.opts.BindPassword)
	}
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ldap bind (%s): %w", c.auth, err)
	}
	return conn, nil
}

// CheckPassword verifies a DN and password with a simple bind on a separate
// connection (same URL and TLS settings as the client); the shared
// connection keeps its own identity.
func (c *Client) CheckPassword(dn, password string) error {
	if password == "" {
		// An empty password would be an unauthenticated bind, which succeeds.
		return ldap.NewError(ldap.LDAPResultInvalidCredentials, errors.New("empty password"))
	}
	conn, err := c.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return conn.Bind(dn, password)
}

// connect opens a connection (with StartTLS when configured), not yet bound.
func (c *Client) connect() (*ldap.Conn, error) {
	opts := []ldap.DialOpt{ldap.DialWithDialer(&net.Dialer{Timeout: c.opts.Timeout})}
	if c.scheme == "ldaps" {
		opts = append(opts, ldap.DialWithTLSConfig(c.tls))
	}
	conn, err := ldap.DialURL(c.opts.URL, opts...)
	if err != nil {
		return nil, &ConnError{Op: "dial", Err: err}
	}
	conn.SetTimeout(c.opts.Timeout)

	if c.scheme == "ldap" && c.opts.CAFile != "" {
		if err := conn.StartTLS(c.tls); err != nil {
			_ = conn.Close()
			return nil, &ConnError{Op: "starttls", Err: err}
		}
	}
	return conn, nil
}

// ConnError reports a failure to reach the server.
type ConnError struct {
	Op  string
	Err error
}

func (e *ConnError) Error() string { return "ldap " + e.Op + ": " + e.Err.Error() }
func (e *ConnError) Unwrap() error { return e.Err }

// IsConnError reports whether err means the server is unreachable or the
// connection was lost (as opposed to an LDAP result error).
func IsConnError(err error) bool {
	var ce *ConnError
	if errors.As(err, &ce) {
		return true
	}
	if ldap.IsErrorWithCode(err, ldap.ErrorNetwork) ||
		ldap.IsErrorWithCode(err, ldap.LDAPResultServerDown) ||
		ldap.IsErrorWithCode(err, ldap.LDAPResultUnavailable) {
		return true
	}
	return errors.Is(err, net.ErrClosed)
}
