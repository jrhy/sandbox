package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type embedRequest struct {
	Model  string   `json:"model"`
	Inputs []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Format   any           `json:"format,omitempty"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func (c *Client) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	body, err := json.Marshal(embedRequest{Model: model, Inputs: inputs})
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	var response embedResponse
	if err := c.postJSON(ctx, "/api/embed", body, &response); err != nil {
		return nil, err
	}
	return response.Embeddings, nil
}

func (c *Client) ProbeDimensions(ctx context.Context, model string) (int, error) {
	vectors, err := c.Embed(ctx, model, []string{"probe"})
	if err != nil {
		return 0, err
	}
	if len(vectors) == 0 {
		return 0, fmt.Errorf("probe %s: no embeddings returned", model)
	}
	return len(vectors[0]), nil
}

func (c *Client) ChatStructured(ctx context.Context, model string, messages []chatMessage, format any) (string, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: messages, Format: format, Stream: false})
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	var response chatResponse
	if err := c.postJSON(ctx, "/api/chat", body, &response); err != nil {
		return "", err
	}
	return response.Message.Content, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte, dest any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request %s: %w", path, err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		return fmt.Errorf("post %s: status %d: %s", path, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	if err := json.NewDecoder(response.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}
