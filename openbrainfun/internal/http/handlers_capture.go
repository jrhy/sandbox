package http

import (
	"context"
	"encoding/json"
	stdhttp "net/http"

	"openbrainfun/internal/thoughts"
)

type CaptureService interface {
	CreateThought(ctx context.Context, in thoughts.CreateThoughtInput) (thoughts.Thought, error)
}

type CaptureHandler struct{ svc CaptureService }

func NewCaptureHandler(svc CaptureService) *CaptureHandler { return &CaptureHandler{svc: svc} }

type createThoughtRequest struct {
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags"`
	Source        string   `json:"source"`
}

func (h *CaptureHandler) ServeHTTP(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if r.Method != stdhttp.MethodPost {
		w.WriteHeader(stdhttp.StatusMethodNotAllowed)
		return
	}
	var req createThoughtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		stdhttp.Error(w, err.Error(), stdhttp.StatusBadRequest)
		return
	}
	th, err := h.svc.CreateThought(r.Context(), thoughts.CreateThoughtInput{
		Content:       req.Content,
		ExposureScope: thoughts.ExposureScope(req.ExposureScope),
		UserTags:      req.UserTags,
		Source:        req.Source,
	})
	if err != nil {
		stdhttp.Error(w, err.Error(), stdhttp.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(stdhttp.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"id": th.ID, "ingest_status": th.IngestStatus})
}
