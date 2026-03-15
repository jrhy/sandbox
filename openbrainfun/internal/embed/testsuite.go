package embed

import (
	"context"
	"math"
	"testing"
)

func RunContractSuite(t *testing.T, makeEmbedder func(t *testing.T) Embedder) {
	t.Helper()
	embedder := makeEmbedder(t)
	inputs := []string{"mcp auth note", "totally unrelated"}
	vectors, err := embedder.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != len(inputs) {
		t.Fatalf("len(vectors) = %d, want %d", len(vectors), len(inputs))
	}
	if embedder.Dimensions() <= 0 {
		t.Fatalf("Dimensions() = %d, want positive", embedder.Dimensions())
	}
	for i, vector := range vectors {
		if len(vector) != embedder.Dimensions() {
			t.Fatalf("len(vectors[%d]) = %d, want %d", i, len(vector), embedder.Dimensions())
		}
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				t.Fatalf("vectors[%d] contains non-finite value %v", i, value)
			}
		}
	}
	again, err := embedder.Embed(context.Background(), []string{inputs[0]})
	if err != nil {
		t.Fatalf("Embed() repeat error = %v", err)
	}
	if len(again) != 1 || len(again[0]) != embedder.Dimensions() {
		t.Fatalf("repeat embedding malformed: %v", again)
	}
}
