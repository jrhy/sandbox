package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jrhy/sandbox/openbrainfun/internal/app"
	"github.com/jrhy/sandbox/openbrainfun/internal/config"
	"github.com/jrhy/sandbox/openbrainfun/internal/migrations"
	"github.com/jrhy/sandbox/openbrainfun/internal/ollama"
	"github.com/jrhy/sandbox/openbrainfun/internal/postgres"
	"github.com/jrhy/sandbox/openbrainfun/internal/server"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
	"github.com/jrhy/sandbox/openbrainfun/internal/worker"
)

const defaultWorkerPollInterval = 5 * time.Second

func main() {
	if err := execute(context.Background(), os.Args[1:], defaultCommandDependencies()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func startCommand(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	ollamaClient := ollama.NewClient(cfg.OllamaURL, nil)
	embedDimensions, err := ollamaClient.ProbeDimensions(ctx, cfg.EmbedModel)
	if err != nil {
		return fmt.Errorf("probe embedding dimensions: %w", err)
	}
	if err := migrations.NewMigrator().Ensure(ctx, pool, embedDimensions); err != nil {
		return err
	}

	thoughtRepo := postgres.NewThoughtStore(pool)
	embedder := ollama.NewProvider(ollamaClient, cfg.EmbedModel)
	extractor := ollama.NewMetadataProvider(ollamaClient, cfg.MetadataModel)
	if _, err := thoughtRepo.ReconcileModels(ctx, thoughts.ReconcileModelsParams{
		EmbeddingModel:       embedder.Model(),
		EmbeddingFingerprint: embedder.Fingerprint(),
		MetadataModel:        extractor.Model(),
		MetadataFingerprint:  extractor.Fingerprint(),
		ReconciledAt:         time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("reconcile stored model fingerprints: %w", err)
	}

	runtime := app.Build(
		cfg,
		postgres.NewAuthStoreFromPGX(pool),
		thoughtRepo,
		embedder,
		extractor,
	)
	startBackgroundWorker(ctx, runtime.Processor, defaultWorkerPollInterval, func(err error) {
		log.Printf("background worker error: %v", err)
	})

	srv := server.NewWithHandlers(cfg, runtime.WebHandler, runtime.MCPHandler)
	return listenAndServeAll(srv.Web, srv.MCP)
}

type runOnceWorker interface {
	RunOnce(ctx context.Context) error
}

type httpServer interface {
	ListenAndServe() error
}

func startBackgroundWorker(ctx context.Context, runner runOnceWorker, pollInterval time.Duration, reportError func(error)) {
	if runner == nil {
		return
	}
	go worker.RunLoop(ctx, runner, pollInterval, reportError)
}

func listenAndServeAll(webServer, mcpServer httpServer) error {
	errCh := make(chan error, 2)
	go func() {
		errCh <- normalizeServeError(webServer.ListenAndServe())
	}()
	go func() {
		errCh <- normalizeServeError(mcpServer.ListenAndServe())
	}()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
