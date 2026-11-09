//go:build integration

// Package integration runs ds-ui code against a real 389-ds container.
// Run with: make itest   (builds deploy/ds and sets DS_IMAGE)
package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

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
		image = "389-ds-ui/ds:dev"
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
			// The entrypoint logs this once the suffix exists.
			WaitingFor: wait.ForLog("ds-entrypoint: creating suffix").
				WithStartupTimeout(3 * time.Minute).
				WithPollInterval(time.Second),
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
