package metadata

import (
	"context"
	"testing"
)

func TestFakeExtractorNormalizesOutput(t *testing.T) {
	extractor := NewFake(map[string]Metadata{
		"remember mcp auth": {Summary: "remember mcp auth", Topics: []string{"mcp", "auth"}},
	})
	got, err := extractor.Extract(context.Background(), "remember mcp auth")
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(got.Topics) != 2 {
		t.Fatalf("Topics = %v, want normalized topics", got.Topics)
	}
}
