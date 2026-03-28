package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/admin"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestExecuteWithoutArgsShowsUsage(t *testing.T) {
	var stdout bytes.Buffer
	deps := commandDependencies{stdout: &stdout, stderr: &bytes.Buffer{}}

	if err := execute(context.Background(), []string{}, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "openbrain start") || !strings.Contains(got, "openbrain user update") {
		t.Fatalf("usage output missing commands: %s", got)
	}
}

func TestExecuteStartInvokesServerCommand(t *testing.T) {
	called := false
	deps := commandDependencies{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		startServer: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	if err := execute(context.Background(), []string{"start"}, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected start server to be invoked")
	}
}

func TestExecuteThoughtAddCreatesThoughtWithOptions(t *testing.T) {
	var stdout bytes.Buffer
	userID := uuid.New()
	runner := &fakeThoughtRunner{
		user:    auth.User{ID: userID, Username: "jimbob"},
		created: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "here is the thought", ExposureScope: thoughts.ExposureScopeRemoteOK, UserTags: []string{"alpha", "beta"}, IngestStatus: thoughts.IngestStatusPending},
	}
	deps := commandDependencies{
		stdout:           &stdout,
		stderr:           &bytes.Buffer{},
		newThoughtRunner: func(ctx context.Context) (thoughtRunner, func(), error) { return runner, func() {}, nil },
	}

	err := execute(context.Background(), []string{"thought", "add", "jimbob", "here is the thought", "--exposure-scope", "remote_ok", "--tag", "alpha", "--tag", "beta"}, deps)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if runner.foundUsername != "jimbob" {
		t.Fatalf("found username = %q, want jimbob", runner.foundUsername)
	}
	if runner.createInput.UserID != userID || runner.createInput.Content != "here is the thought" {
		t.Fatalf("create input = %+v", runner.createInput)
	}
	if runner.createInput.ExposureScope != thoughts.ExposureScopeRemoteOK {
		t.Fatalf("exposure scope = %q, want %q", runner.createInput.ExposureScope, thoughts.ExposureScopeRemoteOK)
	}
	if len(runner.createInput.UserTags) != 2 || runner.createInput.UserTags[0] != "alpha" || runner.createInput.UserTags[1] != "beta" {
		t.Fatalf("tags = %#v, want alpha/beta", runner.createInput.UserTags)
	}
	if got := stdout.String(); !strings.Contains(got, "created thought") || !strings.Contains(got, "username=jimbob") || !strings.Contains(got, "status=pending") {
		t.Fatalf("stdout = %s, want thought creation details", got)
	}
}

func TestExecuteThoughtAddRequiresContent(t *testing.T) {
	deps := commandDependencies{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	err := execute(context.Background(), []string{"thought", "add", "jimbob"}, deps)
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("error = %v, want missing content", err)
	}
}

func TestExecuteThoughtAddClosesResources(t *testing.T) {
	userID := uuid.New()
	runner := &fakeThoughtRunner{
		user:    auth.User{ID: userID, Username: "jimbob"},
		created: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "here is the thought", ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusPending},
	}
	closed := false
	deps := commandDependencies{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		newThoughtRunner: func(ctx context.Context) (thoughtRunner, func(), error) {
			return runner, func() { closed = true }, nil
		},
	}

	err := execute(context.Background(), []string{"thought", "add", "jimbob", "here is the thought"}, deps)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !closed {
		t.Fatal("expected thought runner cleanup to run")
	}
}

func TestExecuteThoughtAddDefaultsExposureScope(t *testing.T) {
	userID := uuid.New()
	runner := &fakeThoughtRunner{
		user:    auth.User{ID: userID, Username: "jimbob"},
		created: thoughts.Thought{ID: uuid.New(), UserID: userID, Content: "here is the thought", ExposureScope: thoughts.ExposureScopeLocalOnly, IngestStatus: thoughts.IngestStatusPending},
	}
	deps := commandDependencies{
		stdout:           &bytes.Buffer{},
		stderr:           &bytes.Buffer{},
		newThoughtRunner: func(ctx context.Context) (thoughtRunner, func(), error) { return runner, func() {}, nil },
	}

	err := execute(context.Background(), []string{"thought", "add", "jimbob", "here is the thought"}, deps)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if runner.createInput.ExposureScope != thoughts.ExposureScopeLocalOnly {
		t.Fatalf("exposure scope = %q, want %q", runner.createInput.ExposureScope, thoughts.ExposureScopeLocalOnly)
	}
}

func TestExecuteUserUpdateRequiresPassword(t *testing.T) {
	deps := commandDependencies{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	err := execute(context.Background(), []string{"user", "update", "demo"}, deps)
	if err == nil || !strings.Contains(err.Error(), "--password is required") {
		t.Fatalf("error = %v, want missing password", err)
	}
}

func TestExecuteUserUpdatePrintsCreatedDefaultToken(t *testing.T) {
	var stdout bytes.Buffer
	runner := &fakeAdminRunner{
		updateResult: admin.UpdateUserResult{CreatedToken: &admin.CreatedToken{Label: "default", Token: "plain-token"}},
	}
	deps := commandDependencies{
		stdout:         &stdout,
		stderr:         &bytes.Buffer{},
		newAdminRunner: func(ctx context.Context) (adminRunner, func(), error) { return runner, func() {}, nil },
	}

	if err := execute(context.Background(), []string{"user", "update", "demo", "--password", "secret"}, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if runner.updatedUsername != "demo" || runner.updatedPassword != "secret" {
		t.Fatalf("updated user = %q/%q, want demo/secret", runner.updatedUsername, runner.updatedPassword)
	}
	got := stdout.String()
	if !strings.Contains(got, "token=plain-token") || !strings.Contains(got, "created default token") {
		t.Fatalf("stdout = %s, want token creation details", got)
	}
}

func TestExecuteTokenCreatePrintsPlaintextToken(t *testing.T) {
	var stdout bytes.Buffer
	runner := &fakeAdminRunner{createdToken: admin.CreatedToken{Label: "laptop", Token: "plain-laptop-token"}}
	deps := commandDependencies{
		stdout:         &stdout,
		stderr:         &bytes.Buffer{},
		newAdminRunner: func(ctx context.Context) (adminRunner, func(), error) { return runner, func() {}, nil },
	}

	if err := execute(context.Background(), []string{"token", "create", "demo", "--label", "laptop"}, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if runner.createdUsername != "demo" || runner.createdLabel != "laptop" {
		t.Fatalf("token create called with %q/%q, want demo/laptop", runner.createdUsername, runner.createdLabel)
	}
	if got := stdout.String(); !strings.Contains(got, "token=plain-laptop-token") {
		t.Fatalf("stdout = %s, want plaintext token", got)
	}
}

func TestExecuteTokenListPrintsLabels(t *testing.T) {
	var stdout bytes.Buffer
	now := time.Unix(1700000000, 0).UTC()
	runner := &fakeAdminRunner{listedTokens: []auth.MCPToken{{Label: "default", CreatedAt: now}}}
	deps := commandDependencies{
		stdout:         &stdout,
		stderr:         &bytes.Buffer{},
		newAdminRunner: func(ctx context.Context) (adminRunner, func(), error) { return runner, func() {}, nil },
	}

	if err := execute(context.Background(), []string{"token", "list", "demo"}, deps); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "default") || !strings.Contains(got, now.Format(time.RFC3339)) {
		t.Fatalf("stdout = %s, want label and timestamp", got)
	}
}

func TestExecuteTokenCreateClosesResources(t *testing.T) {
	runner := &fakeAdminRunner{createdToken: admin.CreatedToken{Label: "laptop", Token: "plain-laptop-token"}}
	closed := false
	deps := commandDependencies{
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		newAdminRunner: func(ctx context.Context) (adminRunner, func(), error) {
			return runner, func() { closed = true }, nil
		},
	}

	err := execute(context.Background(), []string{"token", "create", "demo", "--label", "laptop"}, deps)
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !closed {
		t.Fatal("expected admin runner cleanup to run")
	}
}

func TestExecuteTokenDeleteRequiresLabel(t *testing.T) {
	deps := commandDependencies{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}

	err := execute(context.Background(), []string{"token", "delete", "demo"}, deps)
	if err == nil || !strings.Contains(err.Error(), "--label is required") {
		t.Fatalf("error = %v, want missing label", err)
	}
}

type fakeAdminRunner struct {
	updateResult      admin.UpdateUserResult
	createdToken      admin.CreatedToken
	listedTokens      []auth.MCPToken
	updatedUsername   string
	updatedPassword   string
	updatedTokenLabel string
	createdUsername   string
	createdLabel      string
	deletedUsername   string
	deletedLabel      string
}

func (f *fakeAdminRunner) UpdateUser(ctx context.Context, username, password, tokenLabel string) (admin.UpdateUserResult, error) {
	f.updatedUsername = username
	f.updatedPassword = password
	f.updatedTokenLabel = tokenLabel
	return f.updateResult, nil
}

func (f *fakeAdminRunner) DeleteUser(ctx context.Context, username string) error {
	f.deletedUsername = username
	return nil
}

func (f *fakeAdminRunner) CreateToken(ctx context.Context, username, label string) (admin.CreatedToken, error) {
	f.createdUsername = username
	f.createdLabel = label
	return f.createdToken, nil
}

func (f *fakeAdminRunner) ListTokens(ctx context.Context, username string) ([]auth.MCPToken, error) {
	return append([]auth.MCPToken(nil), f.listedTokens...), nil
}

func (f *fakeAdminRunner) DeleteToken(ctx context.Context, username, label string) (int64, error) {
	f.deletedUsername = username
	f.deletedLabel = label
	return 1, nil
}

type fakeThoughtRunner struct {
	user          auth.User
	created       thoughts.Thought
	foundUsername string
	createInput   thoughts.CreateThoughtInput
}

func (f *fakeThoughtRunner) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	f.foundUsername = username
	return f.user, nil
}

func (f *fakeThoughtRunner) CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	f.createInput = input
	return f.created, nil
}
