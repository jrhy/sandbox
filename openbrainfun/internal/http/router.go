package http

import (
	stdhttp "net/http"

	"openbrainfun/internal/auth"
)

type Dependencies struct {
	CaptureHandler stdhttp.Handler
	WebHandler     stdhttp.Handler
	MCPHandler     stdhttp.Handler
	MCPBearerToken string
}

func NewRouter(dep Dependencies) stdhttp.Handler {
	mux := stdhttp.NewServeMux()
	mux.HandleFunc("/healthz", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if dep.CaptureHandler != nil {
		mux.Handle("/api/thoughts", dep.CaptureHandler)
	}
	if dep.WebHandler != nil {
		mux.Handle("/", dep.WebHandler)
	}
	if dep.MCPHandler != nil {
		mux.Handle("/mcp", auth.BearerToken(dep.MCPBearerToken, dep.MCPHandler))
	}
	return mux
}
