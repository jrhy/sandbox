package config

import (
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OPENBRAIN_WEB_ADDR", "127.0.0.1:8080")
	t.Setenv("OPENBRAIN_MCP_ADDR", "127.0.0.1:8081")
	t.Setenv("OPENBRAIN_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/openbrain?sslmode=disable")
	t.Setenv("OPENBRAIN_OLLAMA_URL", "http://127.0.0.1:11434")
	t.Setenv("OPENBRAIN_EMBED_MODEL", "all-minilm:22m")
	t.Setenv("OPENBRAIN_METADATA_MODEL", "small-metadata-model")
	t.Setenv("OPENBRAIN_SESSION_TTL", "24h")
	t.Setenv("OPENBRAIN_COOKIE_SECURE", "false")
	t.Setenv("OPENBRAIN_CSRF_KEY", "test-csrf-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:8080" || cfg.MCPAddr != "127.0.0.1:8081" {
		t.Fatalf("unexpected addrs: %+v", cfg)
	}
	if cfg.SessionTTL != 24*time.Hour {
		t.Fatalf("SessionTTL = %v, want %v", cfg.SessionTTL, 24*time.Hour)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure = true, want false")
	}
}
