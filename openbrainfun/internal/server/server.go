package server

import (
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/config"
)

// Server holds the web and MCP listeners.
type Server struct {
	Web *http.Server
	MCP *http.Server
}

// New constructs the web and MCP HTTP servers.
func New(cfg config.Config) *Server {
	return NewWithHandlers(cfg, NewWebRouter(cfg, nil, nil, nil, nil, nil), http.NewServeMux())
}

func NewWithHandlers(cfg config.Config, webHandler, mcpHandler http.Handler) *Server {
	if webHandler == nil {
		webHandler = NewWebRouter(cfg, nil, nil, nil, nil, nil)
	}
	if mcpHandler == nil {
		mcpHandler = http.NewServeMux()
	}
	return &Server{
		Web: &http.Server{
			Addr:    cfg.WebAddr,
			Handler: webHandler,
		},
		MCP: &http.Server{
			Addr:    cfg.MCPAddr,
			Handler: mcpHandler,
		},
	}
}
