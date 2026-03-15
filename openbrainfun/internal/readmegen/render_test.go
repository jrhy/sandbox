package readmegen

import (
	"strings"
	"testing"
)

func TestRenderWalkthroughIncludesCurlAndMCPSections(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres", "./var/ollama"},
		Steps: []Step{
			{Title: "Create a thought", Command: "curl ... /api/thoughts", Response: "201 Created"},
			{Title: "Query MCP", Command: "curl ... :8081/mcp", Response: "200 OK"},
		},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if !strings.Contains(md, "./var/postgres") || !strings.Contains(md, "Query MCP") {
		t.Fatalf("walkthrough missing required sections: %s", md)
	}
}

func TestRenderWalkthroughNormalizesCRLFResponses(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres"},
		Steps: []Step{
			{
				Title:    "Log in",
				Command:  "curl ... /api/session",
				Response: "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"csrf_token\":\"abc\"}\r\n",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if strings.Contains(md, "\r") {
		t.Fatalf("walkthrough should not contain carriage returns: %q", md)
	}
}

func TestRenderWalkthroughEndsWithSingleNewline(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres"},
		Steps: []Step{
			{Title: "Query MCP", Command: "curl ... /mcp", Response: "200 OK"},
		},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if strings.HasSuffix(md, "\n\n") {
		t.Fatalf("walkthrough should not end with a blank line: %q", md)
	}
	if !strings.HasSuffix(md, "\n") {
		t.Fatalf("walkthrough should end with a single newline: %q", md)
	}
}

func TestRenderWalkthroughPrettyPrintsJSONResponseBodies(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres"},
		Steps: []Step{
			{
				Title:    "Create a thought",
				Command:  "curl -sS -i -X POST 'http://127.0.0.1:18080/api/thoughts' --data '{\"content\":\"remember\",\"user_tags\":[\"mcp\",\"sessions\"]}'",
				Response: "HTTP/1.1 201 Created\r\nContent-Type: application/json\r\n\r\n{\"content\":\"remember\",\"metadata\":{\"Summary\":\"hello\",\"Topics\":[\"mcp\"],\"Entities\":[]}}",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if !strings.Contains(md, "\"content\": \"remember\"") || !strings.Contains(md, "\"Topics\": [\n") || !strings.Contains(md, "\"mcp\"") {
		t.Fatalf("walkthrough should pretty-print JSON bodies: %s", md)
	}
}

func TestRenderWalkthroughWrapsCurlCommandsAcrossLines(t *testing.T) {
	md, err := RenderWalkthrough(Transcript{
		StoragePaths: []string{"./var/postgres"},
		Steps: []Step{
			{
				Title:    "Create a thought",
				Command:  "curl -sS -i -b '/tmp/cookies' -H 'Content-Type: application/json' -H 'X-CSRF-Token: token' -X POST 'http://127.0.0.1:18080/api/thoughts' --data '{\"content\":\"remember\",\"user_tags\":[\"mcp\",\"sessions\"]}'",
				Response: "HTTP/1.1 201 Created",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderWalkthrough() error = %v", err)
	}
	if !strings.Contains(md, "curl \\\n  -sS \\\n  -i") {
		t.Fatalf("walkthrough should wrap curl commands: %s", md)
	}
	if !strings.Contains(md, "--data '{\n  \"content\": \"remember\",") {
		t.Fatalf("walkthrough should pretty-print curl JSON payloads: %s", md)
	}
}
