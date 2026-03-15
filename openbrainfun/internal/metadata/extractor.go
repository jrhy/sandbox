package metadata

import "context"

type Extractor interface {
	Extract(ctx context.Context, content string) (Metadata, error)
}

type FakeExtractor struct {
	metadataByContent map[string]Metadata
}

func NewFake(metadataByContent map[string]Metadata) *FakeExtractor {
	copied := make(map[string]Metadata, len(metadataByContent))
	for key, value := range metadataByContent {
		copied[key] = Normalize(map[string]any{
			"summary":  value.Summary,
			"topics":   append([]string(nil), value.Topics...),
			"entities": append([]string(nil), value.Entities...),
		})
	}
	return &FakeExtractor{metadataByContent: copied}
}

func (f *FakeExtractor) Extract(ctx context.Context, content string) (Metadata, error) {
	if metadata, ok := f.metadataByContent[content]; ok {
		return metadata, nil
	}
	return Normalize(nil), nil
}
