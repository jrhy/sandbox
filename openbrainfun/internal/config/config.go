package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds application configuration loaded from the environment.
type Config struct {
	WebAddr       string
	MCPAddr       string
	DatabaseURL   string
	OllamaURL     string
	EmbedModel    string
	MetadataModel string
	SessionTTL    time.Duration
	CookieSecure  bool
	CSRFKey       string
	LogLevel      string
}

// Load reads and validates configuration from environment variables.
func Load() (Config, error) {
	sessionTTL, err := requiredDuration("OPENBRAIN_SESSION_TTL")
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := requiredBool("OPENBRAIN_COOKIE_SECURE")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		WebAddr:       os.Getenv("OPENBRAIN_WEB_ADDR"),
		MCPAddr:       os.Getenv("OPENBRAIN_MCP_ADDR"),
		DatabaseURL:   os.Getenv("OPENBRAIN_DATABASE_URL"),
		OllamaURL:     os.Getenv("OPENBRAIN_OLLAMA_URL"),
		EmbedModel:    os.Getenv("OPENBRAIN_EMBED_MODEL"),
		MetadataModel: os.Getenv("OPENBRAIN_METADATA_MODEL"),
		SessionTTL:    sessionTTL,
		CookieSecure:  cookieSecure,
		CSRFKey:       os.Getenv("OPENBRAIN_CSRF_KEY"),
		LogLevel:      os.Getenv("OPENBRAIN_LOG_LEVEL"),
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if err := validateRequired(cfg.WebAddr, "OPENBRAIN_WEB_ADDR"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.MCPAddr, "OPENBRAIN_MCP_ADDR"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.DatabaseURL, "OPENBRAIN_DATABASE_URL"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.OllamaURL, "OPENBRAIN_OLLAMA_URL"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.EmbedModel, "OPENBRAIN_EMBED_MODEL"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.MetadataModel, "OPENBRAIN_METADATA_MODEL"); err != nil {
		return Config{}, err
	}
	if err := validateRequired(cfg.CSRFKey, "OPENBRAIN_CSRF_KEY"); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func requiredDuration(name string) (time.Duration, error) {
	value, err := requiredString(name)
	if err != nil {
		return 0, err
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return duration, nil
}

func requiredBool(name string) (bool, error) {
	value, err := requiredString(name)
	if err != nil {
		return false, err
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func requiredString(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func validateRequired(value, name string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
