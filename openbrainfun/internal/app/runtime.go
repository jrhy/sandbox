package app

import (
	"net/http"

	"github.com/jrhy/sandbox/openbrainfun/internal/api"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/server"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
	"github.com/jrhy/sandbox/openbrainfun/internal/web"
	"github.com/jrhy/sandbox/openbrainfun/internal/worker"
)

type Runtime struct {
	WebHandler http.Handler
	MCPHandler http.Handler
	Processor  *worker.Processor
}

func Build(cfg config.Config, authRepo auth.Repository, thoughtRepo thoughts.Repository, embedder embed.Embedder, extractor metadata.Extractor) Runtime {
	authService := auth.NewService(authRepo, cfg.SessionTTL)
	thoughtService := thoughts.NewService(thoughtRepo, embedder)
	webAuthHandlers := web.NewAuthHandlers(authService, cfg.CookieSecure, cfg.CSRFKey)
	apiSessionHandlers := api.NewSessionHandlers(authService, cfg.CookieSecure, cfg.CSRFKey)
	webThoughtHandlers := web.NewHandlers(thoughtService, cfg.CSRFKey)
	apiThoughtHandlers := api.NewThoughtHandlers(thoughtService)

	return Runtime{
		WebHandler: server.NewWebRouter(cfg, authService, webAuthHandlers, apiSessionHandlers, webThoughtHandlers, apiThoughtHandlers),
		MCPHandler: server.NewMCPRouter(authService, thoughtService),
		Processor:  worker.NewProcessor(thoughtRepo, embedder, extractor),
	}
}
