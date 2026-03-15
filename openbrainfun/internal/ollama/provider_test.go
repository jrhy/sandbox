package ollama

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
)

func TestOllamaProviderSatisfiesContract(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		vectors := make([][]float32, 0, len(req.Inputs))
		for i := range req.Inputs {
			if i%2 == 0 {
				vectors = append(vectors, []float32{0.1, 0.2, 0.3})
				continue
			}
			vectors = append(vectors, []float32{0.3, 0.2, 0.1})
		}
		payload, err := json.Marshal(embedResponse{Embeddings: vectors})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		return jsonResponse(http.StatusOK, string(payload)), nil
	})

	embed.RunContractSuite(t, func(t *testing.T) embed.Embedder {
		return NewProvider(client, "all-minilm:22m")
	})
}
