package e2e

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jrhy/sandbox/openbrainfun/internal/config"
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

func TestBrowserLoginNeedsInsecureCookieForDirectHTTP(t *testing.T) {
	t.Run("cookie secure disabled", func(t *testing.T) {
		env := newTestEnvWithConfig(t, config.Config{
			CookieSecure: false,
			CSRFKey:      "test-csrf-key",
			SessionTTL:   defaultSessionTTL,
		})

		resp := browserLogin(t, env, "demo", defaultDemoPassword)
		if got := resp.Request.URL.Path; got != "/" {
			t.Fatalf("final path = %q, want %q", got, "/")
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		if !strings.Contains(string(body), "Save thought") {
			t.Fatalf("expected home page after login, got body:\n%s", body)
		}
	})

	t.Run("cookie secure enabled", func(t *testing.T) {
		env := newTestEnvWithConfig(t, config.Config{
			CookieSecure: true,
			CSRFKey:      "test-csrf-key",
			SessionTTL:   defaultSessionTTL,
		})

		resp := browserLogin(t, env, "demo", defaultDemoPassword)
		if got := resp.Request.URL.Path; got != "/login" {
			t.Fatalf("final path = %q, want %q when secure cookie is served over plain HTTP", got, "/login")
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		if !strings.Contains(string(body), "<h1>Log in</h1>") {
			t.Fatalf("expected login page after secure-cookie redirect loop, got body:\n%s", body)
		}
	})
}

func browserLogin(t *testing.T, env *TestEnv, username, password string) *http.Response {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	client := &http.Client{
		Jar:       jar,
		Transport: handlerRoundTripper{handler: env.runtime.WebHandler},
	}
	form := url.Values{
		"username": {username},
		"password": {password},
	}
	resp, err := client.PostForm(env.webBaseURL+"/login", form)
	if err != nil {
		t.Fatalf("POST /login error = %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}
