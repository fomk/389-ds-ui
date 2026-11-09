package config

import (
	"os"
	"path/filepath"
	"testing"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaultsSidecar(t *testing.T) {
	cfg, err := load(envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != ModeSidecar {
		t.Errorf("mode = %q", cfg.Mode)
	}
	if want := "ldapi://%2Fdata%2Frun%2Fslapd-localhost.socket"; cfg.LDAPURL != want {
		t.Errorf("url = %q, want %q", cfg.LDAPURL, want)
	}
	if cfg.Lifecycle != "none" {
		t.Errorf("lifecycle = %q, want none without docker host", cfg.Lifecycle)
	}
}

func TestSidecarWithDocker(t *testing.T) {
	cfg, err := load(envMap(map[string]string{
		"DSUI_DOCKER_HOST":  "tcp://proxy:2375",
		"DSUI_DS_CONTAINER": "ds",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lifecycle != "docker" {
		t.Errorf("lifecycle = %q", cfg.Lifecycle)
	}
}

func TestRemoteRequiresURL(t *testing.T) {
	if _, err := load(envMap(map[string]string{"DSUI_MODE": "remote"})); err == nil {
		t.Fatal("expected error")
	}
}

func TestBindPasswordFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pw")
	if err := os.WriteFile(f, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := load(envMap(map[string]string{
		"DSUI_MODE":                    "remote",
		"DSUI_LDAP_URL":                "ldap://ds:3389",
		"DSUI_LDAP_BIND_DN":            "cn=Directory Manager",
		"DSUI_LDAP_BIND_PASSWORD_FILE": f,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LDAPBindPassword != "s3cret" {
		t.Errorf("password = %q", cfg.LDAPBindPassword)
	}
}

func TestBindDNWithoutPassword(t *testing.T) {
	_, err := load(envMap(map[string]string{
		"DSUI_LDAP_BIND_DN": "cn=Directory Manager",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInvalidValues(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"mode":      {"DSUI_MODE": "bogus"},
		"lifecycle": {"DSUI_LIFECYCLE": "systemd"},
		"docker":    {"DSUI_LIFECYCLE": "docker"},
		"timeout":   {"DSUI_LDAP_TIMEOUT": "ten"},
		"loglevel":  {"DSUI_LOG_LEVEL": "loud"},
	} {
		if _, err := load(envMap(env)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
