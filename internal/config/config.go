// Package config reads ds-ui configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mode selects the default set of providers.
type Mode string

const (
	ModeSidecar Mode = "sidecar" // ds + ui containers sharing the /data volume (default)
	ModeRemote  Mode = "remote"  // LDAP over the network only
)

// Config is the complete runtime configuration of ds-ui.
type Config struct {
	Listen   string
	Mode     Mode
	Instance string

	LDAPURL          string
	LDAPBindDN       string
	LDAPBindPassword string
	LDAPCAFile       string
	LDAPTimeout      time.Duration

	DataDir string

	// Auth is how users sign in: ldap (directory accounts) or none.
	Auth           string
	AdminGroups    []string
	OperatorGroups []string
	SessionIdle    time.Duration
	SessionMax     time.Duration
	// TLSHost is where ds-ui reaches the server's LDAPS port to see the
	// certificate it presents (the port comes from nsslapd-securePort).
	TLSHost string

	// Server certificate kept in step with PEM files of an external tool
	// (certbot, lego, cert-manager); empty CertFile: off.
	CertFile          string
	CertKeyFile       string
	CertCheckInterval time.Duration
	CertAutoRestart   bool

	Lifecycle       string // docker | none
	DockerHost      string
	DSContainer     string
	AllowedOrigins  []string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// Load reads the configuration from the process environment.
func Load() (*Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (*Config, error) {
	env := func(key, def string) string {
		if v := strings.TrimSpace(getenv(key)); v != "" {
			return v
		}
		return def
	}

	cfg := &Config{
		Listen:          env("DSUI_LISTEN", ":8080"),
		Mode:            Mode(env("DSUI_MODE", string(ModeSidecar))),
		Instance:        env("DSUI_INSTANCE", "localhost"),
		LDAPBindDN:      env("DSUI_LDAP_BIND_DN", ""),
		LDAPCAFile:      env("DSUI_LDAP_CA_FILE", ""),
		DataDir:         env("DSUI_DATA_DIR", "/data"),
		DockerHost:      env("DSUI_DOCKER_HOST", ""),
		DSContainer:     env("DSUI_DS_CONTAINER", ""),
		ShutdownTimeout: 10 * time.Second,
	}

	switch cfg.Mode {
	case ModeSidecar, ModeRemote:
	default:
		return nil, fmt.Errorf("DSUI_MODE: unknown mode %q (want sidecar or remote)", cfg.Mode)
	}

	defaultURL := ""
	if cfg.Mode != ModeRemote {
		defaultURL = "ldapi://%2Fdata%2Frun%2Fslapd-" + cfg.Instance + ".socket"
	}
	cfg.LDAPURL = env("DSUI_LDAP_URL", defaultURL)
	if cfg.LDAPURL == "" {
		return nil, errors.New("DSUI_LDAP_URL is required in remote mode")
	}

	if f := env("DSUI_LDAP_BIND_PASSWORD_FILE", ""); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("DSUI_LDAP_BIND_PASSWORD_FILE: %w", err)
		}
		cfg.LDAPBindPassword = strings.TrimRight(string(b), "\r\n")
	}
	if cfg.LDAPBindDN != "" && cfg.LDAPBindPassword == "" {
		return nil, errors.New("DSUI_LDAP_BIND_DN is set but DSUI_LDAP_BIND_PASSWORD_FILE is empty")
	}

	cfg.TLSHost = env("DSUI_TLS_HOST", defaultTLSHost(cfg.LDAPURL))

	cfg.Auth = env("DSUI_AUTH", "ldap")
	if cfg.Auth != "ldap" && cfg.Auth != "none" {
		return nil, fmt.Errorf("DSUI_AUTH: %q (want ldap or none)", cfg.Auth)
	}
	// DNs contain commas: groups are separated by semicolons.
	cfg.AdminGroups = splitDNs(env("DSUI_ADMIN_GROUPS", ""))
	cfg.OperatorGroups = splitDNs(env("DSUI_OPERATOR_GROUPS", ""))

	var err error
	if cfg.LDAPTimeout, err = time.ParseDuration(env("DSUI_LDAP_TIMEOUT", "10s")); err != nil {
		return nil, fmt.Errorf("DSUI_LDAP_TIMEOUT: %w", err)
	}
	if cfg.SessionIdle, err = time.ParseDuration(env("DSUI_SESSION_IDLE", "30m")); err != nil {
		return nil, fmt.Errorf("DSUI_SESSION_IDLE: %w", err)
	}
	if cfg.SessionMax, err = time.ParseDuration(env("DSUI_SESSION_MAX", "12h")); err != nil {
		return nil, fmt.Errorf("DSUI_SESSION_MAX: %w", err)
	}

	cfg.CertFile = env("DSUI_CERT_FILE", "")
	cfg.CertKeyFile = env("DSUI_CERT_KEY_FILE", "")
	if cfg.CertCheckInterval, err = time.ParseDuration(env("DSUI_CERT_CHECK_INTERVAL", "10m")); err != nil || cfg.CertCheckInterval < 10*time.Second {
		return nil, fmt.Errorf("DSUI_CERT_CHECK_INTERVAL: %q (a duration of at least 10s)", env("DSUI_CERT_CHECK_INTERVAL", "10m"))
	}
	if cfg.CertAutoRestart, err = strconv.ParseBool(env("DSUI_CERT_AUTO_RESTART", "false")); err != nil {
		return nil, fmt.Errorf("DSUI_CERT_AUTO_RESTART: %w", err)
	}
	if cfg.CertFile == "" && cfg.CertKeyFile != "" {
		return nil, errors.New("DSUI_CERT_KEY_FILE is set but DSUI_CERT_FILE is empty")
	}
	if cfg.CertFile != "" && cfg.Mode == ModeRemote {
		return nil, errors.New("DSUI_CERT_FILE needs the server's NSS database (sidecar mode)")
	}

	defaultLifecycle := "none"
	if cfg.Mode == ModeSidecar && cfg.DockerHost != "" {
		defaultLifecycle = "docker"
	}
	cfg.Lifecycle = env("DSUI_LIFECYCLE", defaultLifecycle)
	switch cfg.Lifecycle {
	case "none":
	case "docker":
		if cfg.DockerHost == "" || cfg.DSContainer == "" {
			return nil, errors.New("DSUI_LIFECYCLE=docker requires DSUI_DOCKER_HOST and DSUI_DS_CONTAINER")
		}
	default:
		return nil, fmt.Errorf("DSUI_LIFECYCLE: unknown value %q (want docker or none)", cfg.Lifecycle)
	}

	for _, o := range strings.Split(env("DSUI_ALLOWED_ORIGINS", "http://127.0.0.1:8080,http://localhost:8080"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(env("DSUI_LOG_LEVEL", "info"))); err != nil {
		return nil, fmt.Errorf("DSUI_LOG_LEVEL: %w", err)
	}

	return cfg, nil
}

// defaultTLSHost is the host of an ldap(s) URL; with LDAPI (sidecar) it is
// "ds", the service name of the server in the Compose files.
func defaultTLSHost(ldapURL string) string {
	if u, err := url.Parse(ldapURL); err == nil && (u.Scheme == "ldap" || u.Scheme == "ldaps") && u.Hostname() != "" {
		return u.Hostname()
	}
	return "ds"
}

func splitDNs(v string) []string {
	var out []string
	for _, d := range strings.Split(v, ";") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}
