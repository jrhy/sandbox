package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jrhy/sandbox/openbrainfun/internal/config"
)

func TestNewBuildsWebAndMCPServers(t *testing.T) {
	cfg := config.Config{
		WebAddr: "127.0.0.1:8080",
		MCPAddr: "127.0.0.1:8081",
	}

	srv := New(cfg)

	if srv.Web == nil {
		t.Fatal("Web = nil")
	}
	if srv.MCP == nil {
		t.Fatal("MCP = nil")
	}
	if srv.Web.Addr != cfg.WebAddr {
		t.Fatalf("Web.Addr = %q, want %q", srv.Web.Addr, cfg.WebAddr)
	}
	if srv.MCP.Addr != cfg.MCPAddr {
		t.Fatalf("MCP.Addr = %q, want %q", srv.MCP.Addr, cfg.MCPAddr)
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Web.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Fatalf("GET /healthz body = %q, want %q", body, "ok\n")
	}
}

func TestNewWithHandlersUsesProvidedHandlers(t *testing.T) {
	cfg := config.Config{
		WebAddr: "127.0.0.1:8080",
		MCPAddr: "127.0.0.1:8081",
	}
	webHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})
	mcpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	srv := NewWithHandlers(cfg, webHandler, mcpHandler)

	webReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	webResp := httptest.NewRecorder()
	srv.Web.Handler.ServeHTTP(webResp, webReq)
	if webResp.Code != http.StatusAccepted {
		t.Fatalf("web status = %d, want %d", webResp.Code, http.StatusAccepted)
	}

	mcpReq := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	mcpResp := httptest.NewRecorder()
	srv.MCP.Handler.ServeHTTP(mcpResp, mcpReq)
	if mcpResp.Code != http.StatusNoContent {
		t.Fatalf("mcp status = %d, want %d", mcpResp.Code, http.StatusNoContent)
	}
}
