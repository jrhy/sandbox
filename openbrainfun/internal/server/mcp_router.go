package server

import (
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/mcpserver"
)

func NewMCPRouter(authService mcpserver.AuthService, queryService mcpserver.QueryService) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpserver.NewHandler(authService, mcpserver.New(queryService)))
	return mux
}
