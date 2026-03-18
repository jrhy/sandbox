package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jrhy/sandbox/openbrainfun/internal/config"
)

func TestStartBackgroundWorkerRunsLoopInBackground(t *testing.T) {
	runner := &fakeStartupRunner{calls: make(chan int, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startBackgroundWorker(ctx, runner, time.Hour, nil)

	select {
	case <-runner.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("background worker did not run")
	}
}

func TestListenAndServeAllStartsBothServers(t *testing.T) {
	web := &fakeHTTPServer{calls: make(chan struct{}, 1), err: http.ErrServerClosed}
	mcp := &fakeHTTPServer{calls: make(chan struct{}, 1), err: http.ErrServerClosed}

	if err := listenAndServeAll(web, mcp); err != nil {
		t.Fatalf("listenAndServeAll() error = %v", err)
	}

	select {
	case <-web.calls:
	default:
		t.Fatal("web server was not started")
	}
	select {
	case <-mcp.calls:
	default:
		t.Fatal("mcp server was not started")
	}
}

func TestListenAndServeAllReturnsRealServerError(t *testing.T) {
	want := errors.New("boom")
	web := &fakeHTTPServer{calls: make(chan struct{}, 1), err: want}
	mcp := &fakeHTTPServer{calls: make(chan struct{}, 1), err: http.ErrServerClosed}

	err := listenAndServeAll(web, mcp)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestStartupServicesMessage(t *testing.T) {
	cfg := config.Config{WebAddr: "127.0.0.1:18080", MCPAddr: "127.0.0.1:18081"}

	got := startupServicesMessage(cfg)
	want := "starting services: web UI + JSON API on http://127.0.0.1:18080, MCP on http://127.0.0.1:18081/mcp"
	if got != want {
		t.Fatalf("startupServicesMessage() = %q, want %q", got, want)
	}
}

type fakeStartupRunner struct {
	calls chan int
	count int
}

func (f *fakeStartupRunner) RunOnce(ctx context.Context) error {
	f.count++
	f.calls <- f.count
	return nil
}

type fakeHTTPServer struct {
	calls chan struct{}
	err   error
}

func (f *fakeHTTPServer) ListenAndServe() error {
	f.calls <- struct{}{}
	return f.err
}
