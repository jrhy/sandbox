package embed

import (
	"context"
	"testing"
)

func TestFakeEmbedderReturnsConfiguredVectors(t *testing.T) {
	embedder := NewFake(map[string][]float32{"alpha": {1, 2, 3}})
	vectors, err := embedder.Embed(context.Background(), []string{"alpha"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if got := vectors[0]; len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("vector = %v, want [1 2 3]", got)
	}
}
