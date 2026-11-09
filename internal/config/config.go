// Package config reads ds-ui configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Mode selects the default set of providers.
type Mode string

const (
	ModeSidecar Mode = "sidecar" // ds + ui containers sharing the /data volume (default)
	ModeRemote  Mode = "remote"  // LDAP over the network only
	ModeAIO     Mode = "aio"     // ds and ui in one container under s6
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

	Lifecycle       string // docker | s6 | none
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
	case ModeSidecar, ModeRemote, ModeAIO:
	default:
		return nil, fmt.Errorf("DSUI_MODE: unknown mode %q (want sidecar, remote or aio)", cfg.Mode)
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

	var err error
	if cfg.LDAPTimeout, err = time.ParseDuration(env("DSUI_LDAP_TIMEOUT", "10s")); err != nil {
		return nil, fmt.Errorf("DSUI_LDAP_TIMEOUT: %w", err)
	}

	defaultLifecycle := "none"
	switch cfg.Mode {
	case ModeSidecar:
		if cfg.DockerHost != "" {
			defaultLifecycle = "docker"
		}
	case ModeAIO:
		defaultLifecycle = "s6"
	}
	cfg.Lifecycle = env("DSUI_LIFECYCLE", defaultLifecycle)
	switch cfg.Lifecycle {
	case "none", "s6":
	case "docker":
		if cfg.DockerHost == "" || cfg.DSContainer == "" {
			return nil, errors.New("DSUI_LIFECYCLE=docker requires DSUI_DOCKER_HOST and DSUI_DS_CONTAINER")
		}
	default:
		return nil, fmt.Errorf("DSUI_LIFECYCLE: unknown value %q (want docker, s6 or none)", cfg.Lifecycle)
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
