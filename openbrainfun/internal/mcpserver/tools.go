package mcpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func NewServer(service *Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "openbrainfun", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "search_thoughts", Description: "Search remote_ok thoughts for the authenticated user."}, service.searchTool)
	mcp.AddTool(server, &mcp.Tool{Name: "recent_thoughts", Description: "List recent remote_ok thoughts for the authenticated user."}, service.recentTool)
	mcp.AddTool(server, &mcp.Tool{Name: "get_thought", Description: "Fetch one remote_ok thought by id for the authenticated user."}, service.getTool)
	mcp.AddTool(server, &mcp.Tool{Name: "stats", Description: "Summarize remote_ok thoughts for the authenticated user."}, service.statsTool)
	return server
}

func NewHandler(authService AuthService, service *Service) http.Handler {
	server := NewServer(service)
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return NewAuthMiddleware(authService)(handler)
}

func (s *Service) searchTool(ctx context.Context, req *mcp.CallToolRequest, input SearchThoughtsInput) (*mcp.CallToolResult, SearchThoughtsOutput, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, SearchThoughtsOutput{}, err
	}
	output, err := s.SearchThoughts(ctx, user, input)
	if err != nil {
		return nil, SearchThoughtsOutput{}, err
	}
	return nil, output, nil
}

func (s *Service) recentTool(ctx context.Context, req *mcp.CallToolRequest, input RecentThoughtsInput) (*mcp.CallToolResult, RecentThoughtsOutput, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, RecentThoughtsOutput{}, err
	}
	output, err := s.RecentThoughts(ctx, user, input)
	if err != nil {
		return nil, RecentThoughtsOutput{}, err
	}
	return nil, output, nil
}

func (s *Service) getTool(ctx context.Context, req *mcp.CallToolRequest, input GetThoughtInput) (*mcp.CallToolResult, GetThoughtOutput, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, GetThoughtOutput{}, err
	}
	output, err := s.GetThought(ctx, user, input)
	if err != nil {
		return nil, GetThoughtOutput{}, err
	}
	return nil, output, nil
}

func (s *Service) statsTool(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, StatsOutput, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, StatsOutput{}, err
	}
	output, err := s.Stats(ctx, user)
	if err != nil {
		return nil, StatsOutput{}, err
	}
	return nil, output, nil
}

func currentUser(ctx context.Context) (auth.User, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return auth.User{}, errAuthenticationRequired
	}
	return user, nil
}

var _ = errors.Is
