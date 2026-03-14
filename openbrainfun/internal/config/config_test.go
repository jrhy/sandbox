package config

import "testing"

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("OPENBRAIN_HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("OPENBRAIN_DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/openbrain?sslmode=disable")
	t.Setenv("OPENBRAIN_OLLAMA_URL", "http://127.0.0.1:11434")
	t.Setenv("OPENBRAIN_EMBED_MODEL", "embeddinggemma")
	t.Setenv("OPENBRAIN_MCP_BEARER_TOKEN", "test-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.EmbedModel != "embeddinggemma" {
		t.Fatalf("EmbedModel = %q, want embeddinggemma", cfg.EmbedModel)
	}
}
