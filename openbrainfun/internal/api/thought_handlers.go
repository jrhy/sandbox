package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

type ThoughtService interface {
	CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error)
	GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error)
	UpdateThought(ctx context.Context, input thoughts.UpdateThoughtInput) (thoughts.Thought, error)
	DeleteThought(ctx context.Context, userID, thoughtID uuid.UUID) error
	ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error)
	SearchThoughts(ctx context.Context, input thoughts.SearchThoughtsInput) ([]thoughts.ScoredThought, error)
	RelatedThoughts(ctx context.Context, input thoughts.RelatedThoughtsInput) ([]thoughts.ScoredThought, error)
	RetryThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error)
}

type ThoughtHandlers struct {
	service ThoughtService
}

type createThoughtRequest struct {
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags"`
}

type updateThoughtRequest struct {
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags"`
}

type thoughtResponse struct {
	ID             string            `json:"id"`
	Content        string            `json:"content"`
	ExposureScope  string            `json:"exposure_scope"`
	UserTags       []string          `json:"user_tags"`
	Metadata       metadata.Metadata `json:"metadata"`
	Similarity     *float64          `json:"similarity,omitempty"`
	EmbeddingModel string            `json:"embedding_model,omitempty"`
	IngestStatus   string            `json:"ingest_status"`
	IngestError    string            `json:"ingest_error,omitempty"`
	CreatedAt      string            `json:"created_at"`
	UpdatedAt      string            `json:"updated_at"`
}

func NewThoughtHandlers(service ThoughtService) *ThoughtHandlers {
	return &ThoughtHandlers{service: service}
}

func (h *ThoughtHandlers) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req createThoughtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	thought, err := h.service.CreateThought(r.Context(), thoughts.CreateThoughtInput{
		UserID:        user.ID,
		Content:       req.Content,
		ExposureScope: thoughts.ExposureScope(req.ExposureScope),
		UserTags:      req.UserTags,
	})
	if err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api create thought failed: user_id=%s content_len=%d exposure_scope=%q tags=%q err=%v", user.ID, len(strings.TrimSpace(req.Content)), req.ExposureScope, strings.Join(req.UserTags, ","), err)
		}
		h.writeThoughtError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toThoughtResponse(thought, nil))
}

func (h *ThoughtHandlers) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	page, err := parseOptionalInt(r.URL.Query().Get("page"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid page")
		return
	}
	pageSize, err := parseOptionalInt(r.URL.Query().Get("page_size"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid page_size")
		return
	}

	searchMode, ok := thoughts.ParseSearchMode(r.URL.Query().Get("search_mode"))
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid search_mode")
		return
	}
	searchQuery := r.URL.Query().Get("q")
	if searchMode == thoughts.SearchModeSemantic && strings.TrimSpace(searchQuery) != "" {
		items, err := h.service.SearchThoughts(r.Context(), thoughts.SearchThoughtsInput{
			UserID:       user.ID,
			Query:        searchQuery,
			SearchMode:   searchMode,
			Exposure:     r.URL.Query().Get("exposure_scope"),
			IngestStatus: r.URL.Query().Get("ingest_status"),
			Tag:          r.URL.Query().Get("tag"),
			Page:         page,
			PageSize:     pageSize,
		})
		if err != nil {
			log.Printf("api semantic search failed: user_id=%s query=%q exposure=%q ingest_status=%q tag=%q err=%v", user.ID, searchQuery, r.URL.Query().Get("exposure_scope"), r.URL.Query().Get("ingest_status"), r.URL.Query().Get("tag"), err)
			writeJSONError(w, http.StatusInternalServerError, "search thoughts")
			return
		}
		response := make([]thoughtResponse, 0, len(items))
		for _, item := range items {
			response = append(response, toThoughtResponse(item.Thought, &item.Similarity))
		}
		writeJSON(w, http.StatusOK, map[string]any{"thoughts": response})
		return
	}

	items, err := h.service.ListThoughts(r.Context(), thoughts.ListThoughtsParams{
		UserID:       user.ID,
		Q:            searchQuery,
		SearchMode:   searchMode,
		Exposure:     r.URL.Query().Get("exposure_scope"),
		IngestStatus: r.URL.Query().Get("ingest_status"),
		Tag:          r.URL.Query().Get("tag"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		log.Printf("api list thoughts failed: user_id=%s query=%q exposure=%q ingest_status=%q tag=%q err=%v", user.ID, searchQuery, r.URL.Query().Get("exposure_scope"), r.URL.Query().Get("ingest_status"), r.URL.Query().Get("tag"), err)
		writeJSONError(w, http.StatusInternalServerError, "list thoughts")
		return
	}

	response := make([]thoughtResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toThoughtResponse(item, nil))
	}
	writeJSON(w, http.StatusOK, map[string]any{"thoughts": response})
}

func (h *ThoughtHandlers) Get(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}

	thought, err := h.service.GetThought(r.Context(), user.ID, thoughtID)
	if err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api get thought failed: user_id=%s thought_id=%s err=%v", user.ID, thoughtID, err)
		}
		h.writeThoughtError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toThoughtResponse(thought, nil))
}

func (h *ThoughtHandlers) Update(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}

	var req updateThoughtRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	thought, err := h.service.UpdateThought(r.Context(), thoughts.UpdateThoughtInput{
		ThoughtID:     thoughtID,
		UserID:        user.ID,
		Content:       req.Content,
		ExposureScope: thoughts.ExposureScope(req.ExposureScope),
		UserTags:      req.UserTags,
	})
	if err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api update thought failed: user_id=%s thought_id=%s content_len=%d exposure_scope=%q tags=%q err=%v", user.ID, thoughtID, len(strings.TrimSpace(req.Content)), req.ExposureScope, strings.Join(req.UserTags, ","), err)
		}
		h.writeThoughtError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toThoughtResponse(thought, nil))
}

func (h *ThoughtHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}

	if err := h.service.DeleteThought(r.Context(), user.ID, thoughtID); err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api delete thought failed: user_id=%s thought_id=%s err=%v", user.ID, thoughtID, err)
		}
		h.writeThoughtError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ThoughtHandlers) Related(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	thoughtID, err := thoughtIDWithSuffix(r.URL.Path, "/related")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid thought ID")
		return
	}
	limit, err := parseOptionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid limit")
		return
	}

	items, err := h.service.RelatedThoughts(r.Context(), thoughts.RelatedThoughtsInput{
		UserID:    user.ID,
		ThoughtID: thoughtID,
		Limit:     limit,
	})
	if err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api related thoughts failed: user_id=%s thought_id=%s limit=%d err=%v", user.ID, thoughtID, limit, err)
		}
		h.writeThoughtError(w, err)
		return
	}

	response := make([]thoughtResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toThoughtResponse(item.Thought, &item.Similarity))
	}
	writeJSON(w, http.StatusOK, map[string]any{"thoughts": response})
}

func (h *ThoughtHandlers) Retry(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	thoughtID, err := thoughtIDWithSuffix(r.URL.Path, "/retry")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid thought ID")
		return
	}

	thought, err := h.service.RetryThought(r.Context(), user.ID, thoughtID)
	if err != nil {
		if shouldLogThoughtError(err) {
			log.Printf("api retry thought failed: user_id=%s thought_id=%s err=%v", user.ID, thoughtID, err)
		}
		h.writeThoughtError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toThoughtResponse(thought, nil))
}

func (h *ThoughtHandlers) userAndThoughtID(w http.ResponseWriter, r *http.Request) (auth.User, uuid.UUID, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return auth.User{}, uuid.Nil, false
	}
	thoughtID, err := thoughtIDFromPath(r.URL.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid thought ID")
		return auth.User{}, uuid.Nil, false
	}
	return user, thoughtID, true
}

func (h *ThoughtHandlers) writeThoughtError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, thoughts.ErrThoughtNotFound):
		writeJSONError(w, http.StatusNotFound, "thought not found")
	case errors.Is(err, thoughts.ErrBlankContent), errors.Is(err, thoughts.ErrInvalidExposure):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
	}
}

func toThoughtResponse(thought thoughts.Thought, similarity *float64) thoughtResponse {
	return thoughtResponse{
		ID:             thought.ID.String(),
		Content:        thought.Content,
		ExposureScope:  string(thought.ExposureScope),
		UserTags:       append([]string(nil), thought.UserTags...),
		Metadata:       thought.Metadata,
		Similarity:     similarity,
		EmbeddingModel: thought.EmbeddingModel,
		IngestStatus:   string(thought.IngestStatus),
		IngestError:    thought.IngestError,
		CreatedAt:      thought.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      thought.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func shouldLogThoughtError(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, thoughts.ErrThoughtNotFound), errors.Is(err, thoughts.ErrBlankContent), errors.Is(err, thoughts.ErrInvalidExposure):
		return false
	default:
		return true
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func parseOptionalInt(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func thoughtIDFromPath(path string) (uuid.UUID, error) {
	return thoughtIDWithSuffix(path, "")
}

func thoughtIDWithSuffix(path, suffix string) (uuid.UUID, error) {
	trimmed := strings.TrimPrefix(path, "/api/thoughts/")
	trimmed = strings.TrimSuffix(trimmed, suffix)
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return uuid.Nil, fmt.Errorf("invalid thought path %q", path)
	}
	return uuid.Parse(trimmed)
}
