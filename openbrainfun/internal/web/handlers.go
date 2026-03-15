package web

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/security"
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

type Handlers struct {
	thoughts         ThoughtService
	csrfKey          string
	indexTemplate    *template.Template
	thoughtsTemplate *template.Template
	thoughtTemplate  *template.Template
	deleteTemplate   *template.Template
}

type pageData struct {
	Title           string
	Page            string
	Path            string
	Flash           string
	CSRFToken       string
	SearchQuery     string
	SearchMode      string
	Exposure        string
	IngestStatus    string
	Tag             string
	Thought         thoughts.Thought
	Thoughts        []thoughts.Thought
	SearchResults   []thoughts.ScoredThought
	RelatedThoughts []thoughts.ScoredThought
	SemanticSearch  bool
	Error           string
}

func NewHandlers(thoughtService ThoughtService, csrfKey string) *Handlers {
	funcs := template.FuncMap{
		"join": strings.Join,
		"formatTime": func(value time.Time) string {
			if value.IsZero() {
				return ""
			}
			return value.Format(time.RFC3339)
		},
		"mul100": func(value float64) float64 {
			return value * 100
		},
	}
	return &Handlers{
		thoughts:         thoughtService,
		csrfKey:          csrfKey,
		indexTemplate:    mustParseTemplate(funcs, "templates/layout.tmpl", "templates/index.tmpl"),
		thoughtsTemplate: mustParseTemplate(funcs, "templates/layout.tmpl", "templates/thoughts.tmpl"),
		thoughtTemplate:  mustParseTemplate(funcs, "templates/layout.tmpl", "templates/thought.tmpl"),
		deleteTemplate:   mustParseTemplate(funcs, "templates/layout.tmpl", "templates/delete_confirm.tmpl"),
	}
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	recent, err := h.thoughts.ListThoughts(r.Context(), thoughts.ListThoughtsParams{UserID: user.ID, Page: 1, PageSize: 10})
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	h.renderHTML(w, h.indexTemplate, h.newPageData(r, "Home", "index", pageData{Thoughts: recent}))
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	_, err := h.thoughts.CreateThought(r.Context(), thoughts.CreateThoughtInput{
		UserID:        user.ID,
		Content:       r.PostForm.Get("content"),
		ExposureScope: thoughts.ExposureScope(r.PostForm.Get("exposure_scope")),
		UserTags:      splitTags(r.PostForm.Get("user_tags")),
	})
	if err != nil {
		h.writeThoughtError(w, err)
		return
	}
	redirectWithFlash(w, r, "/", "Thought saved")
}

func (h *Handlers) Thoughts(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	page, err := parseOptionalInt(r.URL.Query().Get("page"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	pageSize, err := parseOptionalInt(r.URL.Query().Get("page_size"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	params := thoughts.ListThoughtsParams{
		UserID:       user.ID,
		Q:            r.URL.Query().Get("q"),
		SearchMode:   thoughts.SearchMode(r.URL.Query().Get("search_mode")),
		Exposure:     r.URL.Query().Get("exposure_scope"),
		IngestStatus: r.URL.Query().Get("ingest_status"),
		Tag:          r.URL.Query().Get("tag"),
		Page:         page,
		PageSize:     pageSize,
	}
	data := pageData{
		SearchQuery:    params.Q,
		SearchMode:     string(params.SearchMode),
		Exposure:       params.Exposure,
		IngestStatus:   params.IngestStatus,
		Tag:            params.Tag,
		SemanticSearch: params.SearchMode == thoughts.SearchModeSemantic && strings.TrimSpace(params.Q) != "",
	}
	if data.SemanticSearch {
		results, err := h.thoughts.SearchThoughts(r.Context(), thoughts.SearchThoughtsInput{
			UserID:       user.ID,
			Query:        params.Q,
			SearchMode:   params.SearchMode,
			Exposure:     params.Exposure,
			IngestStatus: params.IngestStatus,
			Tag:          params.Tag,
			Page:         params.Page,
			PageSize:     params.PageSize,
		})
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		data.SearchResults = results
	} else {
		list, err := h.thoughts.ListThoughts(r.Context(), params)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		data.Thoughts = list
	}

	h.renderHTML(w, h.thoughtsTemplate, h.newPageData(r, "Thoughts", "thoughts", data))
}

func (h *Handlers) ThoughtDetail(w http.ResponseWriter, r *http.Request) {
	thought, err := h.loadThought(r)
	if err != nil {
		h.writeThoughtError(w, err)
		return
	}
	data := pageData{Thought: thought}
	if thought.IngestStatus == thoughts.IngestStatusReady {
		user, ok := auth.UserFromContext(r.Context())
		if !ok {
			h.writeThoughtError(w, errors.New("missing user"))
			return
		}
		related, err := h.thoughts.RelatedThoughts(r.Context(), thoughts.RelatedThoughtsInput{
			UserID:    user.ID,
			ThoughtID: thought.ID,
			Limit:     5,
		})
		if err != nil {
			h.writeThoughtError(w, err)
			return
		}
		data.RelatedThoughts = related
	}

	h.renderHTML(w, h.thoughtTemplate, h.newPageData(r, "Edit thought", "thought", data))
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	_, err := h.thoughts.UpdateThought(r.Context(), thoughts.UpdateThoughtInput{
		ThoughtID:     thoughtID,
		UserID:        user.ID,
		Content:       r.PostForm.Get("content"),
		ExposureScope: thoughts.ExposureScope(r.PostForm.Get("exposure_scope")),
		UserTags:      splitTags(r.PostForm.Get("user_tags")),
	})
	if err != nil {
		h.writeThoughtError(w, err)
		return
	}
	redirectWithFlash(w, r, "/thoughts/"+thoughtID.String(), "Thought saved")
}

func (h *Handlers) DeleteConfirm(w http.ResponseWriter, r *http.Request) {
	thought, err := h.loadThought(r)
	if err != nil {
		h.writeThoughtError(w, err)
		return
	}

	h.renderHTML(w, h.deleteTemplate, h.newPageData(r, "Confirm delete", "delete", pageData{Thought: thought}))
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}
	if err := h.thoughts.DeleteThought(r.Context(), user.ID, thoughtID); err != nil {
		h.writeThoughtError(w, err)
		return
	}
	redirectWithFlash(w, r, "/thoughts", "Thought deleted")
}

func (h *Handlers) Retry(w http.ResponseWriter, r *http.Request) {
	user, thoughtID, ok := h.userAndThoughtID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	_, err := h.thoughts.RetryThought(r.Context(), user.ID, thoughtID)
	if err != nil {
		h.writeThoughtError(w, err)
		return
	}
	returnTo := strings.TrimSpace(r.PostForm.Get("return_to"))
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") {
		returnTo = "/thoughts"
	}
	redirectWithFlash(w, r, returnTo, "Retry queued")
}

func (h *Handlers) loadThought(r *http.Request) (thoughts.Thought, error) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return thoughts.Thought{}, errors.New("missing user")
	}
	thoughtID, err := thoughtIDFromPath(r.URL.Path)
	if err != nil {
		return thoughts.Thought{}, err
	}
	return h.thoughts.GetThought(r.Context(), user.ID, thoughtID)
}

func (h *Handlers) userAndThoughtID(w http.ResponseWriter, r *http.Request) (auth.User, uuid.UUID, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return auth.User{}, uuid.Nil, false
	}
	thoughtID, err := thoughtIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return auth.User{}, uuid.Nil, false
	}
	return user, thoughtID, true
}

func (h *Handlers) writeThoughtError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, thoughts.ErrThoughtNotFound):
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	case errors.Is(err, thoughts.ErrBlankContent), errors.Is(err, thoughts.ErrInvalidExposure):
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	case err != nil && err.Error() == "missing user":
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
	default:
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handlers) renderHTML(w http.ResponseWriter, tmpl *template.Template, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handlers) newPageData(r *http.Request, title, page string, data pageData) pageData {
	data.Title = title
	data.Page = page
	data.Path = r.URL.RequestURI()
	data.Flash = r.URL.Query().Get("flash")
	data.CSRFToken = security.Token(h.csrfKey, sessionToken(r.Context()))
	return data
}

func mustParseTemplate(funcs template.FuncMap, files ...string) *template.Template {
	return template.Must(template.New("layout.tmpl").Funcs(funcs).ParseFS(templateFS, files...))
}

func thoughtIDFromPath(path string) (uuid.UUID, error) {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 || parts[0] != "thoughts" {
		return uuid.UUID{}, errors.New("invalid thought path")
	}
	return uuid.Parse(parts[1])
}

func splitTags(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func sessionToken(ctx context.Context) string {
	token, _ := auth.SessionTokenFromContext(ctx)
	return token
}

func redirectWithFlash(w http.ResponseWriter, r *http.Request, path, flash string) {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	http.Redirect(w, r, path+separator+"flash="+url.QueryEscape(flash), http.StatusSeeOther)
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
