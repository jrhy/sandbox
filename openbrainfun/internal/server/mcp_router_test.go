package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestNewMCPRouterRejectsUnauthorizedRequest(t *testing.T) {
	handler := NewMCPRouter(fakeMCPAuthService{err: auth.ErrTokenRevoked}, fakeMCPQueryService{})
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer revoked-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

type fakeMCPAuthService struct {
	user auth.User
	err  error
}

func (f fakeMCPAuthService) RequireMCPTokenUser(ctx context.Context, token string) (auth.User, error) {
	if f.err != nil {
		return auth.User{}, f.err
	}
	if f.user.ID == uuid.Nil {
		f.user = auth.User{ID: uuid.New(), Username: "alice"}
	}
	return f.user, nil
}

type fakeMCPQueryService struct{}

func (fakeMCPQueryService) ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error) {
	return nil, nil
}

func (fakeMCPQueryService) GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error) {
	return thoughts.Thought{}, thoughts.ErrThoughtNotFound
}
