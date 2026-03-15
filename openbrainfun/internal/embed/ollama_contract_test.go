package embed_test

import (
	"os"
	"testing"

	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/ollama"
)

func TestOllamaEmbedderSatisfiesContractWhenEnabled(t *testing.T) {
	if os.Getenv("OPENBRAIN_EMBED_BACKEND") != "ollama" {
		t.Skip("set OPENBRAIN_EMBED_BACKEND=ollama to run against a real provider")
	}

	ollamaURL := os.Getenv("OPENBRAIN_OLLAMA_URL")
	model := os.Getenv("OPENBRAIN_EMBED_MODEL")
	if ollamaURL == "" || model == "" {
		t.Fatal("OPENBRAIN_OLLAMA_URL and OPENBRAIN_EMBED_MODEL are required for real embedder contract tests")
	}

	client := ollama.NewClient(ollamaURL, nil)
	embed.RunContractSuite(t, func(t *testing.T) embed.Embedder {
		return ollama.NewProvider(client, model)
	})
}
