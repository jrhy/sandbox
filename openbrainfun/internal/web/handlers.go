package web

import (
	"context"
	"embed"
	"html/template"
	"net/http"

	"openbrainfun/internal/query"
	"openbrainfun/internal/thoughts"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

type QueryService interface {
	ListRecent(ctx context.Context, limit int, viewer query.ViewerType) ([]thoughts.Thought, error)
	SearchKeyword(ctx context.Context, query string, limit int, viewer query.ViewerType) ([]thoughts.Thought, error)
	SearchSemantic(ctx context.Context, query string, limit int, viewer query.ViewerType) ([]thoughts.Thought, error)
}

type Handlers struct {
	q    QueryService
	tmpl *template.Template
}

func NewHandlers(q QueryService) *Handlers {
	t := template.Must(template.ParseFS(templateFS, "templates/*.tmpl"))
	return &Handlers{q: q, tmpl: t}
}

func (h *Handlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		h.Index(w, r)
	case "/thoughts":
		h.Thoughts(w, r)
	case "/search":
		h.Search(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handlers) Index(w http.ResponseWriter, _ *http.Request) {
	_ = h.tmpl.ExecuteTemplate(w, "index", nil)
}

func (h *Handlers) Thoughts(w http.ResponseWriter, r *http.Request) {
	rows, _ := h.q.ListRecent(r.Context(), 50, query.ViewerLocalUI)
	_ = h.tmpl.ExecuteTemplate(w, "thoughts", map[string]any{"Thoughts": rows})
}

func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	mode := r.URL.Query().Get("mode")
	var rows []thoughts.Thought
	if q != "" {
		if mode == "semantic" {
			rows, _ = h.q.SearchSemantic(r.Context(), q, 20, query.ViewerLocalUI)
		} else {
			rows, _ = h.q.SearchKeyword(r.Context(), q, 20, query.ViewerLocalUI)
		}
	}
	_ = h.tmpl.ExecuteTemplate(w, "search", map[string]any{"Thoughts": rows, "Q": q, "Mode": mode})
}
