package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestClientEmbedPostsModelAndInput(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/embed")
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.Model != "all-minilm:22m" {
			t.Fatalf("Model = %q, want %q", req.Model, "all-minilm:22m")
		}
		if len(req.Inputs) != 1 || req.Inputs[0] != "remember mcp auth" {
			t.Fatalf("Inputs = %#v, want one input", req.Inputs)
		}
		return jsonResponse(http.StatusOK, `{"embeddings":[[0.1,0.2,0.3]]}`), nil
	})

	vectors, err := client.Embed(context.Background(), "all-minilm:22m", []string{"remember mcp auth"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("vectors = %v, want one 3d vector", vectors)
	}
}

func TestClientProbeDimensionsUsesEmbeddingLength(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"embeddings":[[0.1,0.2,0.3,0.4]]}`), nil
	})

	dimensions, err := client.ProbeDimensions(context.Background(), "all-minilm:22m")
	if err != nil {
		t.Fatalf("ProbeDimensions() error = %v", err)
	}
	if dimensions != 4 {
		t.Fatalf("dimensions = %d, want 4", dimensions)
	}
}

func TestClientChatStructuredReturnsContent(t *testing.T) {
	client := newTestClient(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/chat")
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.Model != "gpt-oss" {
			t.Fatalf("Model = %q, want %q", req.Model, "gpt-oss")
		}
		if req.Stream {
			t.Fatal("Stream = true, want false")
		}
		return jsonResponse(http.StatusOK, `{"message":{"content":"{\"summary\":\"remember mcp auth\",\"topics\":[\"mcp\",\"auth\"]}"}}`), nil
	})

	content, err := client.ChatStructured(context.Background(), "gpt-oss", []chatMessage{{Role: "user", Content: "extract metadata"}}, map[string]any{"type": "object"})
	if err != nil {
		t.Fatalf("ChatStructured() error = %v", err)
	}
	if content == "" {
		t.Fatal("content = empty, want JSON string")
	}
}
