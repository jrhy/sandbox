package ollama

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
)

const (
	metadataPromptVersion    = "prompt:v1"
	metadataSchemaVersion    = "schema:v1"
	metadataNormalizeVersion = "normalize:v1"
)

type MetadataProvider struct {
	client *Client
	model  string
}

func NewMetadataProvider(client *Client, model string) *MetadataProvider {
	return &MetadataProvider{client: client, model: model}
}

func (p *MetadataProvider) Extract(ctx context.Context, content string) (metadata.Metadata, error) {
	message := chatMessage{
		Role:    "user",
		Content: "Extract a concise summary, topics, and entities as JSON for this thought:\n\n" + content,
	}
	payload, err := p.client.ChatStructured(ctx, p.model, []chatMessage{message}, metadataSchema())
	if err != nil {
		return metadata.Metadata{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return metadata.Metadata{}, fmt.Errorf("decode metadata JSON: %w", err)
	}
	return metadata.Normalize(raw), nil
}

func (p *MetadataProvider) Model() string {
	return p.model
}

func (p *MetadataProvider) Fingerprint() string {
	return fmt.Sprintf("%s|%s|%s|%s", p.model, metadataPromptVersion, metadataSchemaVersion, metadataNormalizeVersion)
}

func metadataSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{"type": "string"},
			"topics": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"entities": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []string{"summary", "topics", "entities"},
	}
}
