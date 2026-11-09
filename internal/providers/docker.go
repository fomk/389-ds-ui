package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DockerLifecycle controls the DS container through the Docker Engine API,
// normally via a docker-socket-proxy. It only ever touches one container.
type DockerLifecycle struct {
	base      string // http://host:port
	container string
	http      *http.Client
}

// NewDockerLifecycle accepts tcp://host:port or http(s)://host:port.
func NewDockerLifecycle(host, container string) (*DockerLifecycle, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("docker host: %w", err)
	}
	switch u.Scheme {
	case "tcp":
		u.Scheme = "http"
	case "http", "https":
	default:
		return nil, fmt.Errorf("docker host: unsupported scheme %q (want tcp, http or https)", u.Scheme)
	}
	if container == "" {
		return nil, fmt.Errorf("docker lifecycle: container name is empty")
	}
	return &DockerLifecycle{
		base:      strings.TrimRight(u.String(), "/"),
		container: container,
		http:      &http.Client{Timeout: 90 * time.Second},
	}, nil
}

func (d *DockerLifecycle) Name() string { return "docker" }

type containerInspect struct {
	State struct {
		Status     string `json:"Status"`
		Restarting bool   `json:"Restarting"`
		Running    bool   `json:"Running"`
	} `json:"State"`
}

func (d *DockerLifecycle) Status(ctx context.Context) (ServerState, error) {
	var ci containerInspect
	if err := d.call(ctx, http.MethodGet, "/json", &ci); err != nil {
		return StateUnknown, err
	}
	switch {
	case ci.State.Restarting || ci.State.Status == "restarting":
		return StateRestarting, nil
	case ci.State.Running:
		return StateRunning, nil
	case ci.State.Status == "exited" || ci.State.Status == "created" || ci.State.Status == "dead":
		return StateStopped, nil
	default:
		return StateUnknown, nil
	}
}

// Restart gives ns-slapd up to 60 s to shut down cleanly.
func (d *DockerLifecycle) Restart(ctx context.Context) error {
	return d.call(ctx, http.MethodPost, "/restart?t=60", nil)
}

func (d *DockerLifecycle) Stop(ctx context.Context) error {
	return d.call(ctx, http.MethodPost, "/stop?t=60", nil)
}

func (d *DockerLifecycle) Start(ctx context.Context) error {
	return d.call(ctx, http.MethodPost, "/start", nil)
}

func (d *DockerLifecycle) call(ctx context.Context, method, suffix string, out any) error {
	u := d.base + "/containers/" + url.PathEscape(d.container) + suffix
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("docker %s %s: %w", method, suffix, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 304: container already in the requested state (start/stop).
	if resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("docker %s %s: %s: %s", method, suffix, resp.Status, strings.TrimSpace(string(body)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("docker %s %s: decode: %w", method, suffix, err)
		}
	}
	return nil
}
