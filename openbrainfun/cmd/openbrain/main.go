package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"openbrainfun/internal/config"
	obhttp "openbrainfun/internal/http"
	"openbrainfun/internal/mcp"
	"openbrainfun/internal/query"
	"openbrainfun/internal/thoughts"
	"openbrainfun/internal/web"
)

type memoryRepo struct {
	mu   sync.Mutex
	rows []thoughts.Thought
}

func (m *memoryRepo) CreateThought(_ context.Context, p thoughts.CreateThoughtParams) (thoughts.Thought, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, p.Thought)
	return p.Thought, nil
}
func (m *memoryRepo) GetThought(_ context.Context, id uuid.UUID) (thoughts.Thought, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.rows {
		if t.ID == id {
			return t, nil
		}
	}
	return thoughts.Thought{}, nil
}
func (m *memoryRepo) ListRecent(_ context.Context, p thoughts.ListRecentParams) ([]thoughts.Thought, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := append([]thoughts.Thought(nil), m.rows...)
	return out, nil
}
func (m *memoryRepo) SearchKeyword(_ context.Context, p thoughts.SearchKeywordParams) ([]thoughts.Thought, error) {
	return m.ListRecent(context.Background(), thoughts.ListRecentParams{Limit: p.Limit})
}
func (m *memoryRepo) SearchSemantic(_ context.Context, p thoughts.SearchSemanticParams) ([]thoughts.Thought, error) {
	return m.ListRecent(context.Background(), thoughts.ListRecentParams{Limit: p.Limit})
}
func (m *memoryRepo) ClaimPending(_ context.Context, _ int) ([]thoughts.Thought, error) {
	return nil, nil
}
func (m *memoryRepo) MarkReady(context.Context, thoughts.MarkReadyParams) error { return nil }
func (m *memoryRepo) MarkFailed(context.Context, uuid.UUID, string) error       { return nil }

type fakeEmbedder struct{}

func (fakeEmbedder) Embed(context.Context, []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2, 0.3}}, nil
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	repo := &memoryRepo{}
	captureSvc := thoughts.NewService(repo)
	querySvc := query.NewService(repo, fakeEmbedder{})
	webHandler := web.NewHandlers(querySvc)
	captureHandler := obhttp.NewCaptureHandler(captureSvc)
	mcpServer := mcp.NewServer()

	r := obhttp.NewRouter(obhttp.Dependencies{CaptureHandler: captureHandler, WebHandler: webHandler, MCPHandler: mcpServer.Handler(), MCPBearerToken: cfg.MCPBearerToken})
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: r}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("listening on %s", cfg.HTTPAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
