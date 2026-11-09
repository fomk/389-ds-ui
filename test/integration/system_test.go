//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/providers"
)

func TestServerInfo(t *testing.T) {
	c := newClient(t)
	var info *ds.ServerInfo
	err := c.Do(context.Background(), func(conn *ldap.Conn) (err error) {
		info, err = ds.GetServerInfo(conn)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.VendorVersion, "389-Directory/") || info.Version == "" {
		t.Errorf("vendorVersion=%q version=%q", info.VendorVersion, info.Version)
	}
	if info.DBImplementation != "mdb" && info.DBImplementation != "bdb" {
		t.Errorf("dbImplementation = %q", info.DBImplementation)
	}
	if !slices.Contains(info.NamingContexts, suffix) {
		t.Errorf("namingContexts %v lack %s", info.NamingContexts, suffix)
	}
	if !strings.EqualFold(info.BoundAs, dmDN) {
		t.Errorf("boundAs = %q", info.BoundAs)
	}
	// Paged results is required by the LDAP browser (stage 1).
	if !slices.Contains(info.SupportedControls, ldap.ControlTypePaging) {
		t.Errorf("paged results control not advertised")
	}
}

func TestReconnectAfterConnectionLoss(t *testing.T) {
	c := newClient(t)
	ctx := context.Background()
	if err := c.Do(ctx, func(conn *ldap.Conn) error { return searchBase(conn, suffix) }); err != nil {
		t.Fatal(err)
	}
	// Kill the shared connection behind the client's back.
	_ = c.Do(ctx, func(conn *ldap.Conn) error { return conn.Close() })
	if err := c.Do(ctx, func(conn *ldap.Conn) error { return searchBase(conn, suffix) }); err != nil {
		t.Fatalf("after connection loss: %v", err)
	}
}

func TestBadCredentials(t *testing.T) {
	c, err := newClientWith(dmDN, "wrong")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	err = c.Do(context.Background(), func(conn *ldap.Conn) error { return nil })
	if !ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
		t.Fatalf("want invalidCredentials, got %v", err)
	}
}

func TestAPIAgainstRealServer(t *testing.T) {
	h := api.NewHandler(api.Deps{
		LDAP:         newClient(t),
		Lifecycle:    providers.NoLifecycle{},
		Capabilities: providers.Capabilities{Mode: "remote", LDAPAuth: "simple", Lifecycle: "none"},
		Instance:     "localhost",
		Version:      "itest",
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/system/info", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("info: %d %s", rec.Code, rec.Body)
	}
	var info api.SystemInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Server.Version == "" || info.UIVersion != "itest" {
		t.Errorf("info = %+v", info)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("readyz: %d %s", rec.Code, rec.Body)
	}
}
