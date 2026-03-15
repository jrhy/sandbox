package ollama

import (
	"context"
	"net/http"
	"testing"
)

func TestMetadataProviderNormalizesJSON(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"message":{"content":"{\"summary\":\"remember mcp auth\",\"topics\":[\"mcp\",\"auth\"],\"entities\":[\"Open WebUI\"]}"}}`), nil
	})

	provider := NewMetadataProvider(client, "gpt-oss")
	got, err := provider.Extract(context.Background(), "remember mcp auth")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if got.Summary != "remember mcp auth" {
		t.Fatalf("Summary = %q, want normalized summary", got.Summary)
	}
	if len(got.Topics) != 2 {
		t.Fatalf("Topics = %v, want normalized topics", got.Topics)
	}
}
