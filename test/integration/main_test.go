//go:build integration

// Package integration runs ds-ui code against a real 389-ds container.
// Run with: make itest   (sets DS_IMAGE; see the Makefile)
package integration

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ldapx"
)

const (
	dmDN       = "cn=Directory Manager"
	dmPassword = "Integration-Test-1"
	suffix     = "dc=example,dc=com"
)

// ldapURL points at the shared DS container for the whole package.
var ldapURL string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	image := os.Getenv("DS_IMAGE")
	if image == "" {
		image = "ghcr.io/fomk/389-ds-base-docker:3.3.1"
	}
	ctx := context.Background()
	ds, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"3389/tcp"},
			Env: map[string]string{
				"DS_DM_PASSWORD": dmPassword,
				"DS_SUFFIX":      suffix,
			},
			WaitingFor: dsReady(),
		},
		Started: true,
	})
	if err != nil {
		log.Printf("start DS container (%s): %v", image, err)
		return 1
	}
	defer func() { _ = ds.Terminate(ctx) }()

	endpoint, err := ds.PortEndpoint(ctx, "3389/tcp", "ldap")
	if err != nil {
		log.Printf("DS endpoint: %v", err)
		return 1
	}
	ldapURL = endpoint
	if err := waitForSuffix(ctx); err != nil {
		log.Printf("DS not ready: %v", err)
		return 1
	}
	if err := loadFixtures(ctx, ds); err != nil {
		log.Printf("load fixtures: %v", err)
		return 1
	}
	return m.Run()
}

func newClient(t testing.TB) *ldapx.Client {
	t.Helper()
	c, err := newClientWith(dmDN, dmPassword)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func newClientWith(bindDN, password string) (*ldapx.Client, error) {
	return ldapx.New(ldapx.Options{URL: ldapURL, BindDN: bindDN, BindPassword: password, Timeout: 10 * time.Second})
}

func searchBase(conn *ldap.Conn, dn string) error {
	_, err := conn.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"1.1"}, nil))
	return err
}

// waitForSuffix polls until the suffix root entry is searchable.
func waitForSuffix(ctx context.Context) error {
	c, err := newClientWith(dmDN, dmPassword)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	deadline := time.Now().Add(time.Minute)
	for {
		err = c.Do(ctx, func(conn *ldap.Conn) error { return searchBase(conn, suffix) })
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("suffix %s: %w", suffix, err)
		}
		time.Sleep(time.Second)
	}
}

// loadFixtures adds test/fixtures/sample.ldif with ldapadd inside the container.
func loadFixtures(ctx context.Context, ds testcontainers.Container) error {
	if err := ds.CopyFileToContainer(ctx, "../fixtures/sample.ldif", "/tmp/sample.ldif", 0o644); err != nil {
		return err
	}
	code, out, err := ds.Exec(ctx, []string{"ldapadd", "-x", "-H", "ldap://localhost:3389",
		"-D", dmDN, "-w", dmPassword, "-f", "/tmp/sample.ldif"})
	if err != nil {
		return err
	}
	if code != 0 {
		b, _ := io.ReadAll(out)
		return fmt.Errorf("ldapadd exit %d: %s", code, b)
	}
	return nil
}

// httptestRequest builds an API request with the CSRF header set.
func httptestRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(api.CSRFHeader, "ds-ui")
	return req
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// dsReady waits until the suffix exists and the Directory Manager password
// is set. The image's entrypoint creates the suffix over LDAPI, so its
// "ready" line can come before dscontainer applies DS_DM_PASSWORD
// ("389-ds-container started."); the order varies between starts.
func dsReady() wait.Strategy {
	return wait.ForAll(
		wait.ForLog("ds-entrypoint: suffix "+suffix+" ready"),
		wait.ForLog("INFO: 389-ds-container started."),
	).WithDeadline(3 * time.Minute)
}
