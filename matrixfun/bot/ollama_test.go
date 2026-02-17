package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPOllamaClient_Chat(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		if req.URL.Path != "/api/chat" {
			t.Fatalf("path = %s, want /api/chat", req.URL.Path)
		}
		var payload OllamaChatRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "mistral-small3.1" {
			t.Fatalf("model = %q, want mistral-small3.1", payload.Model)
		}
		if len(payload.Messages) == 0 {
			t.Fatal("expected non-empty messages")
		}

		respBody, err := json.Marshal(OllamaChatResponse{Message: ChatMessage{Role: "assistant", Content: "hello from ollama"}, Done: true})
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		}, nil
	})

	client := &HTTPOllamaClient{
		BaseURL: "http://ollama.local",
		Client:  &http.Client{Transport: transport},
	}

	resp, err := client.Chat(context.Background(), OllamaChatRequest{
		Model: "mistral-small3.1",
		Messages: []ChatMessage{{
			Role:    "user",
			Content: "hi",
		}},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if resp.Message.Content != "hello from ollama" {
		t.Fatalf("content = %q, want hello from ollama", resp.Message.Content)
	}
}

func TestHTTPOllamaClient_ChatErrors(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("boom")),
		}, nil
	})

	client := &HTTPOllamaClient{BaseURL: "http://ollama.local", Client: &http.Client{Transport: transport}}
	_, err := client.Chat(context.Background(), OllamaChatRequest{Model: "mistral-small3.1", Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestBuildChatRequest(t *testing.T) {
	t.Parallel()

	promptCtx := PromptContext{
		Current: TranscriptMessage{Body: "What is the plan?"},
		Included: []TranscriptMessage{
			{Sender: "@alice:example.org", Body: "Decision: keep ambient mode disabled"},
		},
		RollingSummary: "Previous branch resolved.",
		DurableMemory:  []string{"User likes concise bullet points"},
	}

	now := time.Date(2026, 2, 17, 12, 34, 56, 0, time.UTC)
	req := buildChatRequestAt("mistral-small3.1", "You are helpful.", promptCtx, now)
	if req.Model != "mistral-small3.1" {
		t.Fatalf("model = %q", req.Model)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are helpful." {
		t.Fatalf("unexpected system prompt: %+v", req.Messages[0])
	}
	if !strings.Contains(req.Messages[1].Content, "Durable memory") || !strings.Contains(req.Messages[1].Content, "Recent relevant transcript") {
		t.Fatalf("context block missing sections: %q", req.Messages[1].Content)
	}
	if !strings.Contains(req.Messages[1].Content, "Time context:") || !strings.Contains(req.Messages[1].Content, "interpret dates/times in Pacific time") {
		t.Fatalf("context block missing time guidance: %q", req.Messages[1].Content)
	}
	if req.Messages[2].Role != "user" || req.Messages[2].Content != "What is the plan?" {
		t.Fatalf("unexpected user message: %+v", req.Messages[2])
	}
}

func TestIsTimeoutError(t *testing.T) {
	t.Parallel()

	if !IsTimeoutError(context.DeadlineExceeded) {
		t.Fatal("expected context deadline exceeded to be timeout")
	}
	if !IsTimeoutError(errors.New("Client.Timeout exceeded while awaiting headers")) {
		t.Fatal("expected client timeout text to be timeout")
	}
	if IsTimeoutError(errors.New("other failure")) {
		t.Fatal("unexpected timeout detection for non-timeout error")
	}
}

func TestNewHTTPOllamaClientWithTimeout(t *testing.T) {
	t.Parallel()

	client := NewHTTPOllamaClientWithTimeout("", 12*time.Second)
	if client.BaseURL != defaultOllamaBaseURL {
		t.Fatalf("BaseURL = %q, want %q", client.BaseURL, defaultOllamaBaseURL)
	}
	if client.Client == nil || client.Client.Timeout != 12*time.Second {
		t.Fatalf("timeout = %v, want %v", client.Client.Timeout, 12*time.Second)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
