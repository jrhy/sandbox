package mcp

import (
	"encoding/json"
	"net/http"
)

type Server struct {
	tools []string
}

func NewServer() *Server {
	return &Server{tools: []string{"search_thoughts", "recent_thoughts", "get_thought", "stats"}}
}

func (s *Server) ToolNames() []string { return append([]string(nil), s.tools...) }

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"tools": s.tools, "transport": "streamable-http"})
	})
}
