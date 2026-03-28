package scriptstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInitEnvWritesGeneratedSecretsAndSecurePermissions(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")

	cmd := exec.Command("bash", "scripts/init-env.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"OPENBRAIN_ENV_FILE="+envFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init-env.sh failed: %v\n%s", err, output)
	}

	rawEnv, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	envText := string(rawEnv)
	for _, want := range []string{
		"OPENBRAIN_POSTGRES_CONTAINER_NAME=openbrain-postgres",
		"OPENBRAIN_POSTGRES_VOLUME_NAME=openbrain-postgres-data",
		"OPENBRAIN_OLLAMA_URL=http://127.0.0.1:11434",
		"OPENBRAIN_COOKIE_SECURE=false",
	} {
		if !strings.Contains(envText, want) {
			t.Fatalf(".env missing %q:\n%s", want, envText)
		}
	}
	if strings.Contains(envText, "__GENERATE_") || strings.Contains(envText, "CHANGE_ME") {
		t.Fatalf(".env still contains template placeholders:\n%s", envText)
	}
	if strings.Contains(envText, "OPENBRAIN_POSTGRES_PGDATA=") {
		t.Fatalf(".env should not expose internal postgres PGDATA path:\n%s", envText)
	}

	postgresPassword := envValue(t, envText, "OPENBRAIN_POSTGRES_PASSWORD")
	if len(postgresPassword) < 24 {
		t.Fatalf("generated postgres password too short: %q", postgresPassword)
	}
	databaseURL := envValue(t, envText, "OPENBRAIN_DATABASE_URL")
	wantDatabaseURLPrefix := "postgres://openbrain:"
	if !strings.HasPrefix(databaseURL, wantDatabaseURLPrefix) {
		t.Fatalf("database url = %q, want prefix %q", databaseURL, wantDatabaseURLPrefix)
	}
	if !strings.Contains(databaseURL, "@127.0.0.1:5432/openbrain?sslmode=disable") {
		t.Fatalf("database url missing expected host/db details: %q", databaseURL)
	}
	if !strings.Contains(databaseURL, postgresPassword) {
		t.Fatalf("database url should contain generated password %q, got %q", postgresPassword, databaseURL)
	}

	csrfKey := envValue(t, envText, "OPENBRAIN_CSRF_KEY")
	if matched := regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(csrfKey); !matched {
		t.Fatalf("generated csrf key should be 64 hex chars, got %q", csrfKey)
	}

	info, err := os.Stat(envFile)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf(".env permissions = %o, want 600", got)
	}
}

func TestInitEnvRefusesToOverwriteExistingFileWithoutForce(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")
	const original = "OPENBRAIN_CSRF_KEY=keep-me\n"
	if err := os.WriteFile(envFile, []byte(original), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cmd := exec.Command("bash", "scripts/init-env.sh")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"OPENBRAIN_ENV_FILE="+envFile,
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("init-env.sh unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(string(output), "already exists") {
		t.Fatalf("unexpected output:\n%s", output)
	}

	rawEnv, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("read env file: %v", readErr)
	}
	if string(rawEnv) != original {
		t.Fatalf(".env should remain unchanged, got:\n%s", rawEnv)
	}
}

func TestInitEnvForceOverwritesExistingFile(t *testing.T) {
	repo := repoRoot(t)
	tempDir := t.TempDir()
	envFile := filepath.Join(tempDir, ".env")
	if err := os.WriteFile(envFile, []byte("OPENBRAIN_CSRF_KEY=old\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	cmd := exec.Command("bash", "scripts/init-env.sh", "--force")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"OPENBRAIN_ENV_FILE="+envFile,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init-env.sh --force failed: %v\n%s", err, output)
	}

	rawEnv, readErr := os.ReadFile(envFile)
	if readErr != nil {
		t.Fatalf("read env file: %v", readErr)
	}
	if strings.Contains(string(rawEnv), "OPENBRAIN_CSRF_KEY=old") {
		t.Fatalf(".env was not overwritten:\n%s", rawEnv)
	}
}

func envValue(t *testing.T, envText, key string) string {
	t.Helper()
	prefix := key + "="
	for _, line := range strings.Split(envText, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("missing env key %s in:\n%s", key, envText)
	return ""
}
