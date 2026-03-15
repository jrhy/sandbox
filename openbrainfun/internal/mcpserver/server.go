package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

const defaultListLimit = 20
const statsPageSize = 1000

type AuthService interface {
	RequireMCPTokenUser(ctx context.Context, token string) (auth.User, error)
}

type QueryService interface {
	ListThoughts(ctx context.Context, params thoughts.ListThoughtsParams) ([]thoughts.Thought, error)
	GetThought(ctx context.Context, userID, thoughtID uuid.UUID) (thoughts.Thought, error)
}

type Service struct {
	thoughts QueryService
}

type SearchThoughtsInput struct {
	Query      string `json:"query"`
	SearchMode string `json:"search_mode,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type RecentThoughtsInput struct {
	Limit int `json:"limit,omitempty"`
}

type GetThoughtInput struct {
	ID string `json:"id"`
}

type ThoughtView struct {
	ID            string   `json:"id"`
	Content       string   `json:"content"`
	ExposureScope string   `json:"exposure_scope"`
	UserTags      []string `json:"user_tags"`
	IngestStatus  string   `json:"ingest_status"`
}

type SearchThoughtsOutput struct {
	Thoughts []ThoughtView `json:"thoughts"`
}

type RecentThoughtsOutput struct {
	Thoughts []ThoughtView `json:"thoughts"`
}

type GetThoughtOutput struct {
	Thought ThoughtView `json:"thought"`
}

type StatsOutput struct {
	Total  int `json:"total"`
	Ready  int `json:"ready"`
	Failed int `json:"failed"`
}

func New(queryService QueryService) *Service {
	return &Service{thoughts: queryService}
}

func (s *Service) SearchThoughts(ctx context.Context, user auth.User, input SearchThoughtsInput) (SearchThoughtsOutput, error) {
	items, err := s.thoughts.ListThoughts(ctx, thoughts.ListThoughtsParams{
		UserID:     user.ID,
		Q:          strings.TrimSpace(input.Query),
		SearchMode: thoughts.SearchMode(input.SearchMode),
		Exposure:   string(thoughts.ExposureScopeRemoteOK),
		Page:       1,
		PageSize:   normalizeLimit(input.Limit),
	})
	if err != nil {
		return SearchThoughtsOutput{}, err
	}
	return SearchThoughtsOutput{Thoughts: projectThoughts(filterRemoteThoughts(items, user.ID, normalizeLimit(input.Limit)))}, nil
}

func (s *Service) RecentThoughts(ctx context.Context, user auth.User, input RecentThoughtsInput) (RecentThoughtsOutput, error) {
	items, err := s.thoughts.ListThoughts(ctx, thoughts.ListThoughtsParams{
		UserID:   user.ID,
		Exposure: string(thoughts.ExposureScopeRemoteOK),
		Page:     1,
		PageSize: normalizeLimit(input.Limit),
	})
	if err != nil {
		return RecentThoughtsOutput{}, err
	}
	return RecentThoughtsOutput{Thoughts: projectThoughts(filterRemoteThoughts(items, user.ID, normalizeLimit(input.Limit)))}, nil
}

func (s *Service) GetThought(ctx context.Context, user auth.User, input GetThoughtInput) (GetThoughtOutput, error) {
	thoughtID, err := uuid.Parse(strings.TrimSpace(input.ID))
	if err != nil {
		return GetThoughtOutput{}, fmt.Errorf("parse thought id: %w", err)
	}
	thought, err := s.thoughts.GetThought(ctx, user.ID, thoughtID)
	if err != nil {
		return GetThoughtOutput{}, err
	}
	if thought.UserID != user.ID || thought.ExposureScope != thoughts.ExposureScopeRemoteOK {
		return GetThoughtOutput{}, thoughts.ErrThoughtNotFound
	}
	return GetThoughtOutput{Thought: projectThought(thought)}, nil
}

func (s *Service) Stats(ctx context.Context, user auth.User) (StatsOutput, error) {
	items, err := s.thoughts.ListThoughts(ctx, thoughts.ListThoughtsParams{
		UserID:   user.ID,
		Exposure: string(thoughts.ExposureScopeRemoteOK),
		Page:     1,
		PageSize: statsPageSize,
	})
	if err != nil {
		return StatsOutput{}, err
	}
	filtered := filterRemoteThoughts(items, user.ID, statsPageSize)
	stats := StatsOutput{Total: len(filtered)}
	for _, item := range filtered {
		switch item.IngestStatus {
		case thoughts.IngestStatusReady:
			stats.Ready++
		case thoughts.IngestStatusFailed:
			stats.Failed++
		}
	}
	return stats, nil
}

func NewAuthMiddleware(authService AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authService == nil {
				unauthorized(w)
				return
			}
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w)
				return
			}
			user, err := authService.RequireMCPTokenUser(r.Context(), token)
			if err != nil {
				unauthorized(w)
				return
			}
			ctx := context.WithValue(r.Context(), auth.ContextUserKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}
	return parts[1], true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

func filterRemoteThoughts(items []thoughts.Thought, userID uuid.UUID, limit int) []thoughts.Thought {
	filtered := make([]thoughts.Thought, 0, len(items))
	for _, item := range items {
		if item.UserID != userID || item.ExposureScope != thoughts.ExposureScopeRemoteOK {
			continue
		}
		filtered = append(filtered, item)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return filtered
}

func projectThoughts(items []thoughts.Thought) []ThoughtView {
	result := make([]ThoughtView, 0, len(items))
	for _, item := range items {
		result = append(result, projectThought(item))
	}
	return result
}

func projectThought(item thoughts.Thought) ThoughtView {
	return ThoughtView{
		ID:            item.ID.String(),
		Content:       item.Content,
		ExposureScope: string(item.ExposureScope),
		UserTags:      append([]string(nil), item.UserTags...),
		IngestStatus:  string(item.IngestStatus),
	}
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	return limit
}

var errAuthenticationRequired = errors.New("authentication required")
