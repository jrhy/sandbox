package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

type BotRuntime struct {
	LLMConfig            LLMConfig
	OllamaClient         OllamaClient
	Conversation         *ConversationStore
	MemoryStore          *RoomMemoryStore
	OllamaTimeoutSeconds int
	TrustedPolicyEditors map[id.UserID]bool
}

type ChatResult struct {
	Reply  string
	Action string
}

type chatAPIRequest struct {
	RoomID  string `json:"room_id"`
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

type chatAPIResponse struct {
	RoomID string `json:"room_id"`
	Sender string `json:"sender"`
	Action string `json:"action"`
	Reply  string `json:"reply"`
	Model  string `json:"model"`
}

func (r *BotRuntime) HandleChat(ctx context.Context, roomID id.RoomID, sender id.UserID, body string) (ChatResult, error) {
	if r == nil {
		return ChatResult{}, fmt.Errorf("nil runtime")
	}
	if r.Conversation == nil {
		return ChatResult{}, fmt.Errorf("nil conversation store")
	}
	if r.MemoryStore == nil {
		return ChatResult{}, fmt.Errorf("nil memory store")
	}
	if strings.TrimSpace(body) == "" {
		return ChatResult{}, fmt.Errorf("empty message")
	}
	if strings.TrimSpace(string(roomID)) == "" {
		return ChatResult{}, fmt.Errorf("empty room id")
	}
	if strings.TrimSpace(string(sender)) == "" {
		sender = "@curl:local"
	}

	current := TranscriptMessage{
		EventID:     fmt.Sprintf("$api-%d", time.Now().UnixNano()),
		RoomID:      string(roomID),
		Sender:      string(sender),
		Body:        strings.TrimSpace(body),
		Timestamp:   time.Now().UTC(),
		MentionsBot: false,
	}
	history := r.Conversation.Recent(roomID, 80)
	r.Conversation.Add(roomID, current)

	if reply, handled, err := handlePolicyCommand(ctx, sender, roomID, current.Body, r.MemoryStore, r.LLMConfig, r.OllamaClient, r.TrustedPolicyEditors); handled {
		if err != nil {
			return ChatResult{}, err
		}
		r.Conversation.Add(roomID, TranscriptMessage{
			EventID:   fmt.Sprintf("$api-bot-%d", time.Now().UnixNano()),
			RoomID:    string(roomID),
			Sender:    "@gobot:local",
			Body:      reply,
			Timestamp: time.Now().UTC(),
			IsBot:     true,
		})
		return ChatResult{Reply: reply, Action: "policy_command"}, nil
	}

	if reply, handled := handleCommand(current.Body, roomID, r.MemoryStore, r.Conversation); handled {
		r.Conversation.Add(roomID, TranscriptMessage{
			EventID:   fmt.Sprintf("$api-bot-%d", time.Now().UnixNano()),
			RoomID:    string(roomID),
			Sender:    "@gobot:local",
			Body:      reply,
			Timestamp: time.Now().UTC(),
			IsBot:     true,
		})
		return ChatResult{Reply: reply, Action: "command"}, nil
	}

	roomMemory, err := r.MemoryStore.Load(roomID)
	if err != nil {
		return ChatResult{}, fmt.Errorf("load room memory: %w", err)
	}
	activeCfg := r.LLMConfig
	activeCfg.SystemPrompt = composeSystemPrompt(r.LLMConfig.SystemPrompt, roomMemory.Policy)
	activeCfg.Context = applyPolicyToContextConfig(r.LLMConfig.Context, roomMemory.Policy)
	promptCtx := BuildPromptContextForEvent(activeCfg.Context, current, history, roomMemory.RollingSummary, roomMemory.DurableMemory)
	llmReply, err := CallOllama(ctx, r.OllamaClient, activeCfg, promptCtx)
	if err != nil {
		if IsTimeoutError(err) {
			timeoutReply := fmt.Sprintf("I timed out waiting for Ollama after %ds. Please try again, or increase OLLAMA_TIMEOUT_SECONDS.", r.OllamaTimeoutSeconds)
			return ChatResult{Reply: timeoutReply, Action: "llm_timeout"}, nil
		}
		return ChatResult{}, fmt.Errorf("ollama call: %w", err)
	}
	if strings.TrimSpace(llmReply) == "" {
		return ChatResult{Reply: "", Action: "llm_empty"}, nil
	}

	roomMemory.RollingSummary = promptCtx.RollingSummary
	roomMemory.DurableMemory = MergeDurableMemory(roomMemory.DurableMemory, ExtractDurableMemoryCandidates(current.Body), 50)
	if err := r.MemoryStore.Save(roomID, roomMemory); err != nil {
		return ChatResult{}, fmt.Errorf("save room memory: %w", err)
	}

	r.Conversation.Add(roomID, TranscriptMessage{
		EventID:   fmt.Sprintf("$api-bot-%d", time.Now().UnixNano()),
		RoomID:    string(roomID),
		Sender:    "@gobot:local",
		Body:      llmReply,
		Timestamp: time.Now().UTC(),
		IsBot:     true,
	})
	return ChatResult{Reply: llmReply, Action: "llm_reply"}, nil
}

func NewChatAPIHandler(runtime *BotRuntime, authToken string) http.Handler {
	authToken = strings.TrimSpace(authToken)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if authToken != "" && !hasBearerToken(req, authToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var payload chatAPIRequest
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		payload.RoomID = strings.TrimSpace(payload.RoomID)
		payload.Sender = strings.TrimSpace(payload.Sender)
		payload.Message = strings.TrimSpace(payload.Message)
		if payload.RoomID == "" || payload.Message == "" {
			http.Error(w, "room_id and message are required", http.StatusBadRequest)
			return
		}
		if payload.Sender == "" {
			payload.Sender = "@curl:local"
		}

		result, err := runtime.HandleChat(req.Context(), id.RoomID(payload.RoomID), id.UserID(payload.Sender), payload.Message)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			http.Error(w, err.Error(), status)
			return
		}

		resp := chatAPIResponse{
			RoomID: payload.RoomID,
			Sender: payload.Sender,
			Action: result.Action,
			Reply:  result.Reply,
			Model:  runtime.LLMConfig.Model,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("encode /v1/chat response failed: %v", err)
		}
	})
	return mux
}

func hasBearerToken(req *http.Request, want string) bool {
	header := strings.TrimSpace(req.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	return token == want
}

func startChatAPIServer(ctx context.Context, listenAddr string, runtime *BotRuntime, authToken string) error {
	listenAddr = strings.TrimSpace(listenAddr)
	if listenAddr == "" {
		return nil
	}
	server := &http.Server{
		Addr:    listenAddr,
		Handler: NewChatAPIHandler(runtime, authToken),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("chat api shutdown error: %v", err)
		}
	}()
	go func() {
		log.Printf("chat api listening on %s", listenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("chat api server stopped: %v", err)
		}
	}()
	return nil
}
