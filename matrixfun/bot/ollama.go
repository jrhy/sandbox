package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const defaultOllamaBaseURL = "http://127.0.0.1:11434"
const defaultOllamaTimeout = 90 * time.Second

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaChatRequest struct {
	Model    string         `json:"model"`
	Messages []ChatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type OllamaChatResponse struct {
	Message ChatMessage `json:"message"`
	Done    bool        `json:"done"`
}

type OllamaClient interface {
	Chat(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error)
}

type HTTPOllamaClient struct {
	BaseURL string
	Client  *http.Client
}

func NewHTTPOllamaClient(baseURL string) *HTTPOllamaClient {
	return NewHTTPOllamaClientWithTimeout(baseURL, defaultOllamaTimeout)
}

func NewHTTPOllamaClientWithTimeout(baseURL string, timeout time.Duration) *HTTPOllamaClient {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	if timeout <= 0 {
		timeout = defaultOllamaTimeout
	}
	return &HTTPOllamaClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Client:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPOllamaClient) Chat(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return OllamaChatResponse{}, fmt.Errorf("missing model")
	}
	if len(req.Messages) == 0 {
		return OllamaChatResponse{}, fmt.Errorf("missing messages")
	}

	rawReq, err := json.Marshal(req)
	if err != nil {
		return OllamaChatResponse{}, fmt.Errorf("marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/chat", bytes.NewReader(rawReq))
	if err != nil {
		return OllamaChatResponse{}, fmt.Errorf("create ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpClient := c.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultOllamaTimeout}
	}

	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return OllamaChatResponse{}, fmt.Errorf("ollama chat failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return OllamaChatResponse{}, fmt.Errorf("ollama chat status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	var resp OllamaChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return OllamaChatResponse{}, fmt.Errorf("decode ollama response: %w", err)
	}
	return resp, nil
}

func IsTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "client.timeout exceeded")
}

func BuildChatRequest(model, systemPrompt string, promptCtx PromptContext) OllamaChatRequest {
	messages := []ChatMessage{{
		Role:    "system",
		Content: strings.TrimSpace(systemPrompt),
	}}

	contextBlock := renderContextBlock(promptCtx)
	if contextBlock != "" {
		messages = append(messages, ChatMessage{Role: "system", Content: contextBlock})
	}

	messages = append(messages, ChatMessage{Role: "user", Content: strings.TrimSpace(promptCtx.Current.Body)})

	return OllamaChatRequest{
		Model:    strings.TrimSpace(model),
		Messages: messages,
		Stream:   false,
	}
}

func renderContextBlock(promptCtx PromptContext) string {
	parts := make([]string, 0, 4)

	if len(promptCtx.DurableMemory) > 0 {
		parts = append(parts, "Durable memory:\n- "+strings.Join(promptCtx.DurableMemory, "\n- "))
	}

	if summary := strings.TrimSpace(promptCtx.RollingSummary); summary != "" {
		parts = append(parts, "Rolling summary:\n"+summary)
	}

	if len(promptCtx.Included) > 0 {
		lines := make([]string, 0, len(promptCtx.Included))
		for _, msg := range promptCtx.Included {
			lines = append(lines, formatContextLine(msg))
		}
		parts = append(parts, "Recent relevant transcript:\n"+strings.Join(lines, "\n"))
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func formatContextLine(msg TranscriptMessage) string {
	sender := strings.TrimSpace(msg.Sender)
	if sender == "" {
		sender = "unknown"
	}
	body := strings.TrimSpace(msg.Body)
	if body == "" {
		body = "<empty>"
	}
	return "[" + sender + "] " + body
}
