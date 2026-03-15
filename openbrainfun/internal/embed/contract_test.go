package embed

import "testing"

func TestFakeEmbedderSatisfiesContract(t *testing.T) {
	RunContractSuite(t, func(t *testing.T) Embedder {
		return NewFake(map[string][]float32{
			"mcp auth note":     {1, 0, 0},
			"totally unrelated": {0, 1, 0},
		})
	})
}
