package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestChatAPIHandler_AuthAndValidation(t *testing.T) {
	t.Parallel()

	runtime := newTestRuntime(t)
	h := NewChatAPIHandler(runtime, "secret-token")

	t.Run("unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"room_id":"!r:example.org","message":"hi"}`))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("bad request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"room_id":"","message":""}`))
		req.Header.Set("Authorization", "Bearer secret-token")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})
}

func TestChatAPIHandler_CommandAndLLM(t *testing.T) {
	t.Parallel()

	runtime := newTestRuntime(t)
	h := NewChatAPIHandler(runtime, "")

	t.Run("command", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"room_id":"!r:example.org","sender":"@u:example.org","message":"!ping"}`))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d", rr.Code)
		}
		var resp chatAPIResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Action != "command" || resp.Reply != "pong" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})

	t.Run("llm", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat", strings.NewReader(`{"room_id":"!r:example.org","sender":"@u:example.org","message":"hello"}`))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d body=%q", rr.Code, rr.Body.String())
		}
		var resp chatAPIResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Action != "llm_reply" || resp.Reply != "hello from test ollama" {
			t.Fatalf("unexpected response: %+v", resp)
		}
	})
}

func TestBotRuntime_TimeoutMapsToReply(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	memoryStore, err := NewRoomMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	runtime := &BotRuntime{
		LLMConfig:            LLMConfig{Model: "mistral-small3.1", SystemPrompt: "sys", Context: ContextConfig{}},
		OllamaClient:         &fakeOllamaClient{err: context.DeadlineExceeded},
		Conversation:         NewConversationStore(20),
		MemoryStore:          memoryStore,
		OllamaTimeoutSeconds: 42,
	}

	result, err := runtime.HandleChat(context.Background(), id.RoomID("!r:example.org"), id.UserID("@u:example.org"), "hello")
	if err != nil {
		t.Fatalf("HandleChat() error = %v", err)
	}
	if result.Action != "llm_timeout" {
		t.Fatalf("action = %q, want llm_timeout", result.Action)
	}
	if !strings.Contains(result.Reply, "42s") {
		t.Fatalf("reply = %q, expected timeout seconds", result.Reply)
	}
}

func newTestRuntime(t *testing.T) *BotRuntime {
	t.Helper()
	dir := t.TempDir()
	memoryStore, err := NewRoomMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	return &BotRuntime{
		LLMConfig:            LLMConfig{Model: "mistral-small3.1", SystemPrompt: "sys", Context: ContextConfig{}},
		OllamaClient:         &fakeOllamaClient{resp: OllamaChatResponse{Message: ChatMessage{Content: "hello from test ollama"}}},
		Conversation:         NewConversationStore(50),
		MemoryStore:          memoryStore,
		OllamaTimeoutSeconds: 90,
	}
}
