// Command ds-ui is a web administration UI for 389 Directory Server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/fomk/389-ds-ui/internal/api"
	"github.com/fomk/389-ds-ui/internal/config"
	"github.com/fomk/389-ds-ui/internal/ds"
	"github.com/fomk/389-ds-ui/internal/entries"
	"github.com/fomk/389-ds-ui/internal/events"
	"github.com/fomk/389-ds-ui/internal/ldapx"
	"github.com/fomk/389-ds-ui/internal/ldapx/schema"
	"github.com/fomk/389-ds-ui/internal/monitor"
	"github.com/fomk/389-ds-ui/internal/providers"
	"github.com/fomk/389-ds-ui/internal/templates"
	"github.com/fomk/389-ds-ui/internal/webui"
)

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

const usage = `usage: ds-ui [command]

commands:
  serve        run the HTTP server (default)
  healthcheck  exit 0 if the local server answers /healthz (for container HEALTHCHECK)
  openapi      print the OpenAPI document
  version      print the version
`

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve()
	case "healthcheck":
		err = healthcheck()
	case "openapi":
		var b []byte
		if b, err = api.OpenAPI(version); err == nil {
			_, err = os.Stdout.Write(append(b, '\n'))
		}
	case "version":
		fmt.Println(version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ds-ui:", err)
		os.Exit(1)
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	client, err := ldapx.New(ldapx.Options{
		URL:          cfg.LDAPURL,
		BindDN:       cfg.LDAPBindDN,
		BindPassword: cfg.LDAPBindPassword,
		CAFile:       cfg.LDAPCAFile,
		Timeout:      cfg.LDAPTimeout,
		Logger:       log,
	})
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	var lifecycle providers.Lifecycle = providers.NoLifecycle{}
	switch cfg.Lifecycle {
	case "docker":
		if lifecycle, err = providers.NewDockerLifecycle(cfg.DockerHost, cfg.DSContainer); err != nil {
			return err
		}
	case "s6":
		return errors.New("DSUI_LIFECYCLE=s6 (all-in-one mode) is not implemented yet")
	}

	var files *providers.Files
	if cfg.Mode != config.ModeRemote {
		files = providers.DetectFiles(cfg.DataDir)
	}
	caps := providers.Capabilities{
		Mode:      string(cfg.Mode),
		LDAPAuth:  string(client.Auth()),
		Lifecycle: lifecycle.Name(),
		Files:     files != nil,
		Certs:     providers.DetectCerts(files),
		Offline:   false,
	}

	static := webui.FS()
	if static == nil {
		log.Warn("binary was built without the web UI; only the API is served")
	}

	tpls, err := templates.Load()
	if err != nil {
		return fmt.Errorf("entry templates: %w", err)
	}
	schemas := schema.NewCache(time.Minute, log)
	// One hour of counters at 2 s resolution, for the monitoring charts (the
	// UI refreshes as often as every second).
	history := monitor.NewHistory(client, 2*time.Second, 1800, log)
	bus := &events.Bus{}
	handler := api.NewHandler(api.Deps{
		LDAP:           client,
		Schemas:        schemas,
		Entries:        entries.New(client, schemas, log),
		Templates:      tpls,
		History:        history,
		Restart:        &ds.RestartRegistry{},
		Tasks:          ds.NewTasks(client, bus, log),
		Events:         bus,
		Files:          files,
		Lifecycle:      lifecycle,
		Capabilities:   caps,
		Instance:       cfg.Instance,
		Version:        version,
		AllowedOrigins: cfg.AllowedOrigins,
		Static:         static,
		Logger:         log,
	})

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	log.Info("ds-ui started", "version", version, "listen", ln.Addr().String(), "mode", cfg.Mode,
		"ldap", cfg.LDAPURL, "auth", caps.LDAPAuth, "lifecycle", caps.Lifecycle,
		"files", caps.Files, "certs", caps.Certs)
	log.Warn("ds-ui has NO authentication: expose it only on localhost or a trusted network")

	go func() {
		// Warm up the connection so that misconfiguration is visible in the log immediately.
		err := client.Do(ctx, func(*ldap.Conn) error { return nil })
		if err != nil {
			log.Warn("directory server is not reachable yet", "err", err)
		}
	}()

	go history.Run(ctx)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func healthcheck() error {
	addr := os.Getenv("DSUI_LISTEN")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}
