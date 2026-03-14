package http

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"openbrainfun/internal/thoughts"
)

type fakeCaptureSvc struct{}

func (fakeCaptureSvc) CreateThought(_ context.Context, in thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	return thoughts.NewThought(thoughts.NewThoughtParams{Content: in.Content, ExposureScope: in.ExposureScope, UserTags: in.UserTags, Source: in.Source})
}

func TestPostThoughtReturnsCreated(t *testing.T) {
	h := NewCaptureHandler(fakeCaptureSvc{})
	r := httptest.NewRequest(http.MethodPost, "/api/thoughts", bytes.NewBufferString(`{"content":"hi","exposure_scope":"remote_ok"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d", w.Code)
	}
}
