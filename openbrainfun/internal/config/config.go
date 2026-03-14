package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type EmbedProvider string

const (
	EmbedProviderOllama EmbedProvider = "ollama"
	EmbedProviderFake   EmbedProvider = "fake"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	OllamaURL       string
	EmbedModel      string
	EmbedDimensions int
	EmbedProvider   EmbedProvider
	MCPBearerToken  string
	LogLevel        string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:       envOr("OPENBRAIN_HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:    os.Getenv("OPENBRAIN_DATABASE_URL"),
		OllamaURL:      os.Getenv("OPENBRAIN_OLLAMA_URL"),
		EmbedModel:     os.Getenv("OPENBRAIN_EMBED_MODEL"),
		MCPBearerToken: os.Getenv("OPENBRAIN_MCP_BEARER_TOKEN"),
		LogLevel:       envOr("OPENBRAIN_LOG_LEVEL", "info"),
		EmbedProvider:  EmbedProvider(envOr("OPENBRAIN_EMBED_PROVIDER", string(EmbedProviderOllama))),
	}

	dimRaw := envOr("OPENBRAIN_EMBED_DIMENSIONS", "384")
	dim, err := strconv.Atoi(dimRaw)
	if err != nil || dim <= 0 {
		return Config{}, fmt.Errorf("invalid OPENBRAIN_EMBED_DIMENSIONS %q", dimRaw)
	}
	cfg.EmbedDimensions = dim

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("OPENBRAIN_DATABASE_URL is required")
	}
	if cfg.EmbedProvider == EmbedProviderOllama {
		if cfg.OllamaURL == "" {
			return Config{}, errors.New("OPENBRAIN_OLLAMA_URL is required")
		}
		if cfg.EmbedModel == "" {
			return Config{}, errors.New("OPENBRAIN_EMBED_MODEL is required")
		}
	}
	if cfg.MCPBearerToken == "" {
		return Config{}, errors.New("OPENBRAIN_MCP_BEARER_TOKEN is required")
	}
	return cfg, nil
}

func envOr(k, d string) string {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	return v
}
