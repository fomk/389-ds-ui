package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDockerLifecycle(t *testing.T) {
	var calls []string
	state := `{"State":{"Status":"running","Running":true}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/containers/my-ds/json":
			_, _ = w.Write([]byte(state))
		case r.Method == http.MethodPost && r.URL.Path == "/containers/my-ds/restart":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/containers/my-ds/start":
			w.WriteHeader(http.StatusNotModified)
		default:
			http.Error(w, "forbidden", http.StatusForbidden)
		}
	}))
	defer srv.Close()

	d, err := NewDockerLifecycle(strings.Replace(srv.URL, "http://", "tcp://", 1), "my-ds")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if st, err := d.Status(ctx); err != nil || st != StateRunning {
		t.Fatalf("status = %v, %v", st, err)
	}
	state = `{"State":{"Status":"exited"}}`
	if st, _ := d.Status(ctx); st != StateStopped {
		t.Errorf("status = %v, want stopped", st)
	}
	if err := d.Restart(ctx); err != nil {
		t.Errorf("restart: %v", err)
	}
	if err := d.Start(ctx); err != nil {
		t.Errorf("start (304) should succeed: %v", err)
	}
	if err := d.Stop(ctx); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("stop: want 403 error, got %v", err)
	}
	if want := "POST /containers/my-ds/restart?t=60"; calls[2] != want {
		t.Errorf("call = %q, want %q", calls[2], want)
	}
}

func TestNewDockerLifecycleValidation(t *testing.T) {
	if _, err := NewDockerLifecycle("unix:///var/run/docker.sock", "ds"); err == nil {
		t.Error("unix scheme should be rejected")
	}
	if _, err := NewDockerLifecycle("tcp://proxy:2375", ""); err == nil {
		t.Error("empty container should be rejected")
	}
}
