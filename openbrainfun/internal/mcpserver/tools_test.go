package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerExposesSearchTool(t *testing.T) {
	ctx := context.WithValue(context.Background(), auth.ContextUserKey{}, auth.User{ID: uuid.New(), Username: "alice"})
	service := New(fakeQueryService{list: []thoughts.Thought{{
		ID: uuid.New(), UserID: ctx.Value(auth.ContextUserKey{}).(auth.User).ID, Content: "remote visible", ExposureScope: thoughts.ExposureScopeRemoteOK, IngestStatus: thoughts.IngestStatusReady,
	}}})
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := NewServer(service).Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("Connect(server) error = %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect(client) error = %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search_thoughts",
		Arguments: map[string]any{"query": "remote"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("content = empty, want tool output")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok || textContent.Text == "" || !strings.Contains(textContent.Text, "remote visible") {
		t.Fatalf("content = %#v, want thought content", result.Content)
	}
}
