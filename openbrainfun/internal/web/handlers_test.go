package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openbrainfun/internal/query"
	"openbrainfun/internal/thoughts"
)

type fakeQueryService struct{}

func (fakeQueryService) ListRecent(context.Context, int, query.ViewerType) ([]thoughts.Thought, error) {
	return nil, nil
}
func (fakeQueryService) SearchKeyword(context.Context, string, int, query.ViewerType) ([]thoughts.Thought, error) {
	return nil, nil
}
func (fakeQueryService) SearchSemantic(context.Context, string, int, query.ViewerType) ([]thoughts.Thought, error) {
	return nil, nil
}

func TestIndexPageShowsCaptureForm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h := NewHandlers(fakeQueryService{})
	h.Index(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `name="content"`) {
		t.Fatalf("body missing capture form: %s", rr.Body.String())
	}
}
