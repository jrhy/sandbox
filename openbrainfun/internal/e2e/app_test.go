package e2e

import (
	"os"
	"testing"
)

func TestEndToEndCRUDAndMCPIsolation(t *testing.T) {
	env := NewTestEnv(t)
	client := env.LoginAsDemoUser(t)
	thought := env.CreateThought(t, client, CreateThoughtRequest{
		Content:       "Remember MCP auth",
		ExposureScope: "remote_ok",
	})

	got := env.GetThought(t, client, thought.ID)
	if len(got.Metadata.Topics) == 0 {
		t.Fatal("expected extracted metadata")
	}

	env.AssertMCPFindsThought(t, env.DemoToken(), "MCP auth")
	env.AssertOtherUsersMCPTokenCannotSeeThought(t, thought.ID)

	updated := env.UpdateThought(t, client, thought.ID, UpdateThoughtRequest{
		Content:       "Remember MCP auth and sessions",
		ExposureScope: "remote_ok",
		UserTags:      []string{"mcp", "sessions"},
	})
	if updated.IngestStatus != "pending" {
		t.Fatalf("updated ingest_status = %q, want pending", updated.IngestStatus)
	}
	env.ProcessPending(t)

	afterUpdate := env.GetThought(t, client, thought.ID)
	if afterUpdate.Content != "Remember MCP auth and sessions" {
		t.Fatalf("updated content = %q, want updated value", afterUpdate.Content)
	}

	env.DeleteThought(t, client, thought.ID)
	env.AssertThoughtNotFound(t, client, thought.ID)
}

func TestMetadataExtractionSmokeWhenEnabled(t *testing.T) {
	if os.Getenv("OPENBRAIN_METADATA_BACKEND") != "ollama" {
		t.Skip("set OPENBRAIN_METADATA_BACKEND=ollama to run against a real metadata extractor")
	}

	env := NewTestEnv(t)
	client := env.LoginAsDemoUser(t)
	thought := env.CreateThought(t, client, CreateThoughtRequest{
		Content:       "Remember MCP auth for Open WebUI local sessions",
		ExposureScope: "remote_ok",
	})

	got := env.GetThought(t, client, thought.ID)
	if got.Metadata.Summary == "" || got.Metadata.Summary == "No summary available." {
		t.Fatalf("metadata summary = %q, want real extracted summary", got.Metadata.Summary)
	}
}

func TestSemanticSearchAndRelatedThoughts(t *testing.T) {
	env := NewTestEnv(t)
	client := env.LoginAsDemoUser(t)

	env.CreateThought(t, client, CreateThoughtRequest{
		Content:       "Remember MCP auth",
		ExposureScope: "remote_ok",
	})
	middle := env.CreateThought(t, client, CreateThoughtRequest{
		Content:       "Remember MCP auth and sessions",
		ExposureScope: "remote_ok",
	})
	env.CreateThought(t, client, CreateThoughtRequest{
		Content:       "Remember MCP auth for Open WebUI local sessions",
		ExposureScope: "remote_ok",
	})

	searchResults := env.SearchThoughts(t, client, "Remember MCP auth and sessions")
	if len(searchResults) == 0 {
		t.Fatal("semantic search returned no results")
	}
	if searchResults[0].Content != "Remember MCP auth and sessions" || searchResults[0].Similarity == nil {
		t.Fatalf("top semantic result = %+v, want exact anchor with similarity", searchResults[0])
	}

	related := env.RelatedThoughts(t, client, middle.ID)
	if len(related) == 0 {
		t.Fatal("related thoughts returned no results")
	}
	if related[0].Content != "Remember MCP auth for Open WebUI local sessions" || related[0].Similarity == nil {
		t.Fatalf("top related result = %+v, want Open WebUI sessions match", related[0])
	}

	env.AssertMCPRelatedThoughts(t, env.DemoToken(), middle.ID, "Open WebUI local sessions")
}
