package ollama

import (
	"context"
	"fmt"
	"sync"
)

type Provider struct {
	client *Client
	model  string

	mu         sync.Mutex
	dimensions int
}

func NewProvider(client *Client, model string) *Provider {
	return &Provider{client: client, model: model}
}

func (p *Provider) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	vectors, err := p.client.Embed(ctx, p.model, inputs)
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embed %s: no embeddings returned", p.model)
	}
	p.setDimensions(len(vectors[0]))
	return vectors, nil
}

func (p *Provider) Dimensions() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dimensions
}

func (p *Provider) Model() string {
	return p.model
}

func (p *Provider) Fingerprint() string {
	return fmt.Sprintf("%s|embed:v1", p.model)
}

func (p *Provider) setDimensions(dimensions int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if dimensions > 0 {
		p.dimensions = dimensions
	}
}
