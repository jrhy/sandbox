package migrations

import (
	"strings"
	"testing"
)

func TestRenderInitialSchemaReplacesEmbedDimensions(t *testing.T) {
	tmpl := "create table thoughts (embedding vector(__EMBED_DIMENSIONS__));"
	got, err := RenderSchema(tmpl, 384)
	if err != nil {
		t.Fatalf("RenderSchema() error = %v", err)
	}
	if !strings.Contains(got, "vector(384)") {
		t.Fatalf("got %q, want rendered dimension", got)
	}
}
