package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jrhy/sandbox/openbrainfun/internal/admin"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
	"github.com/jrhy/sandbox/openbrainfun/internal/embed"
	"github.com/jrhy/sandbox/openbrainfun/internal/postgres"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

type adminRunner interface {
	UpdateUser(ctx context.Context, username, password, tokenLabel string) (admin.UpdateUserResult, error)
	DeleteUser(ctx context.Context, username string) error
	CreateToken(ctx context.Context, username, label string) (admin.CreatedToken, error)
	ListTokens(ctx context.Context, username string) ([]auth.MCPToken, error)
	DeleteToken(ctx context.Context, username, label string) (int64, error)
}

type thoughtRunner interface {
	FindUserByUsername(ctx context.Context, username string) (auth.User, error)
	CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error)
}

type commandDependencies struct {
	stdout           io.Writer
	stderr           io.Writer
	startServer      func(ctx context.Context) error
	newAdminRunner   func(ctx context.Context) (adminRunner, func(), error)
	newThoughtRunner func(ctx context.Context) (thoughtRunner, func(), error)
}

func defaultCommandDependencies() commandDependencies {
	return commandDependencies{
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		startServer: startCommand,
		newAdminRunner: func(ctx context.Context) (adminRunner, func(), error) {
			pool, err := newCommandPool(ctx)
			if err != nil {
				return nil, nil, err
			}
			return admin.NewService(postgres.NewAuthStoreFromPGX(pool)), pool.Close, nil
		},
		newThoughtRunner: func(ctx context.Context) (thoughtRunner, func(), error) {
			pool, err := newCommandPool(ctx)
			if err != nil {
				return nil, nil, err
			}
			authStore := postgres.NewAuthStoreFromPGX(pool)
			thoughtService := thoughts.NewService(postgres.NewThoughtStore(pool), embed.NewFake(nil))
			return &thoughtServiceRunner{authStore: authStore, thoughtService: thoughtService}, pool.Close, nil
		},
	}
}

func execute(ctx context.Context, args []string, deps commandDependencies) error {
	if deps.stdout == nil {
		deps.stdout = io.Discard
	}
	if deps.stderr == nil {
		deps.stderr = io.Discard
	}
	if deps.startServer == nil {
		deps.startServer = startCommand
	}
	defaults := defaultCommandDependencies()
	if deps.newAdminRunner == nil {
		deps.newAdminRunner = defaults.newAdminRunner
	}
	if deps.newThoughtRunner == nil {
		deps.newThoughtRunner = defaults.newThoughtRunner
	}

	if len(args) == 0 {
		printUsage(deps.stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(deps.stdout)
		return nil
	case "start":
		return deps.startServer(ctx)
	case "user":
		return executeUserCommand(ctx, args[1:], deps)
	case "token":
		return executeTokenCommand(ctx, args[1:], deps)
	case "thought":
		return executeThoughtCommand(ctx, args[1:], deps)
	default:
		printUsage(deps.stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func executeUserCommand(ctx context.Context, args []string, deps commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("user subcommand is required")
	}

	switch args[0] {
	case "update":
		username, password, tokenLabel, err := parseUserUpdateArgs(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newAdminRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		result, err := runner.UpdateUser(ctx, username, password, tokenLabel)
		if err != nil {
			return err
		}
		fmt.Fprintf(deps.stdout, "updated user username=%s\n", result.User.Username)
		if result.CreatedToken == nil {
			fmt.Fprintln(deps.stdout, "mcp tokens already exist; no new default token created")
			return nil
		}
		fmt.Fprintf(deps.stdout, "created default token label=%s\n", result.CreatedToken.Label)
		fmt.Fprintf(deps.stdout, "token=%s\n", result.CreatedToken.Token)
		fmt.Fprintf(deps.stdout, "note: this token will not be shown again; use `openbrain token create %s --label %s` to rotate it\n", result.User.Username, result.CreatedToken.Label)
		return nil
	case "delete":
		username, err := parseSingleUsernameArg(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newAdminRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		if err := runner.DeleteUser(ctx, username); err != nil {
			return err
		}
		fmt.Fprintf(deps.stdout, "deleted user username=%s\n", username)
		return nil
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func executeThoughtCommand(ctx context.Context, args []string, deps commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("thought subcommand is required")
	}

	switch args[0] {
	case "add":
		username, input, err := parseThoughtAddArgs(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newThoughtRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		user, err := runner.FindUserByUsername(ctx, username)
		if err != nil {
			return err
		}
		input.UserID = user.ID
		created, err := runner.CreateThought(ctx, input)
		if err != nil {
			return err
		}
		fmt.Fprintf(deps.stdout, "created thought id=%s username=%s status=%s exposure_scope=%s tags=%s\n", created.ID, user.Username, created.IngestStatus, created.ExposureScope, strings.Join(created.UserTags, ","))
		return nil
	default:
		return fmt.Errorf("unknown thought subcommand %q", args[0])
	}
}

func executeTokenCommand(ctx context.Context, args []string, deps commandDependencies) error {
	if len(args) == 0 {
		return fmt.Errorf("token subcommand is required")
	}

	switch args[0] {
	case "create":
		username, label, err := parseTokenCreateArgs(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newAdminRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		created, err := runner.CreateToken(ctx, username, label)
		if err != nil {
			return err
		}
		fmt.Fprintf(deps.stdout, "created token username=%s label=%s\n", username, created.Label)
		fmt.Fprintf(deps.stdout, "token=%s\n", created.Token)
		fmt.Fprintf(deps.stdout, "note: this token will not be shown again; rerun `openbrain token create %s --label %s` to rotate it\n", username, created.Label)
		return nil
	case "list":
		username, err := parseSingleUsernameArg(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newAdminRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		tokens, err := runner.ListTokens(ctx, username)
		if err != nil {
			return err
		}
		if len(tokens) == 0 {
			fmt.Fprintf(deps.stdout, "no tokens found for username=%s\n", username)
			return nil
		}
		writer := tabwriter.NewWriter(deps.stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(writer, "LABEL\tCREATED_AT\tLAST_USED_AT\tREVOKED")
		for _, token := range tokens {
			lastUsedAt := "-"
			if !token.LastUsedAt.IsZero() {
				lastUsedAt = token.LastUsedAt.UTC().Format(time.RFC3339)
			}
			revoked := "false"
			if token.RevokedAt != nil {
				revoked = token.RevokedAt.UTC().Format(time.RFC3339)
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", token.Label, token.CreatedAt.UTC().Format(time.RFC3339), lastUsedAt, revoked)
		}
		return writer.Flush()
	case "delete":
		username, label, err := parseTokenDeleteArgs(args[1:])
		if err != nil {
			return err
		}
		runner, closeRunner, err := deps.newAdminRunner(ctx)
		if err != nil {
			return err
		}
		defer closeRunner()
		deletedCount, err := runner.DeleteToken(ctx, username, label)
		if err != nil {
			return err
		}
		fmt.Fprintf(deps.stdout, "deleted tokens username=%s label=%s count=%d\n", username, label, deletedCount)
		return nil
	default:
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
}

func parseSingleUsernameArg(args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf("username is required")
	}
	return strings.TrimSpace(args[0]), nil
}

func parseUserUpdateArgs(args []string) (string, string, string, error) {
	username := ""
	password := ""
	tokenLabel := "default"

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--password":
			index++
			if index >= len(args) {
				return "", "", "", fmt.Errorf("--password requires a value")
			}
			password = args[index]
		case "--token-label":
			index++
			if index >= len(args) {
				return "", "", "", fmt.Errorf("--token-label requires a value")
			}
			tokenLabel = args[index]
		default:
			if strings.HasPrefix(arg, "--") {
				return "", "", "", fmt.Errorf("unknown flag %q", arg)
			}
			if username != "" {
				return "", "", "", fmt.Errorf("expected exactly one username")
			}
			username = arg
		}
	}

	if strings.TrimSpace(username) == "" {
		return "", "", "", fmt.Errorf("username is required")
	}
	if password == "" {
		return "", "", "", fmt.Errorf("--password is required")
	}

	return strings.TrimSpace(username), password, strings.TrimSpace(tokenLabel), nil
}

func parseThoughtAddArgs(args []string) (string, thoughts.CreateThoughtInput, error) {
	username := ""
	content := ""
	exposureScope := thoughts.ExposureScopeLocalOnly
	tags := make([]string, 0)

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--exposure-scope":
			index++
			if index >= len(args) {
				return "", thoughts.CreateThoughtInput{}, fmt.Errorf("--exposure-scope requires a value")
			}
			exposureScope = thoughts.ExposureScope(strings.TrimSpace(args[index]))
		case "--tag":
			index++
			if index >= len(args) {
				return "", thoughts.CreateThoughtInput{}, fmt.Errorf("--tag requires a value")
			}
			tags = append(tags, strings.TrimSpace(args[index]))
		default:
			if strings.HasPrefix(arg, "--") {
				return "", thoughts.CreateThoughtInput{}, fmt.Errorf("unknown flag %q", arg)
			}
			if username == "" {
				username = arg
				continue
			}
			if content == "" {
				content = arg
				continue
			}
			return "", thoughts.CreateThoughtInput{}, fmt.Errorf("expected exactly one username and one content argument")
		}
	}

	if strings.TrimSpace(username) == "" {
		return "", thoughts.CreateThoughtInput{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(content) == "" {
		return "", thoughts.CreateThoughtInput{}, fmt.Errorf("content is required")
	}
	if !exposureScope.Valid() {
		return "", thoughts.CreateThoughtInput{}, thoughts.ErrInvalidExposure
	}

	return strings.TrimSpace(username), thoughts.CreateThoughtInput{
		Content:       content,
		ExposureScope: exposureScope,
		UserTags:      tags,
	}, nil
}

func parseTokenCreateArgs(args []string) (string, string, error) {
	username, label, err := parseTokenArgs(args, false)
	if err != nil {
		return "", "", err
	}
	return username, label, nil
}

func parseTokenDeleteArgs(args []string) (string, string, error) {
	return parseTokenArgs(args, true)
}

func parseTokenArgs(args []string, requireLabel bool) (string, string, error) {
	username := ""
	label := ""

	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "--label":
			index++
			if index >= len(args) {
				return "", "", fmt.Errorf("--label requires a value")
			}
			label = args[index]
		default:
			if strings.HasPrefix(arg, "--") {
				return "", "", fmt.Errorf("unknown flag %q", arg)
			}
			if username != "" {
				return "", "", fmt.Errorf("expected exactly one username")
			}
			username = arg
		}
	}

	if strings.TrimSpace(username) == "" {
		return "", "", fmt.Errorf("username is required")
	}
	if requireLabel && strings.TrimSpace(label) == "" {
		return "", "", fmt.Errorf("--label is required")
	}
	if !requireLabel && label == "" {
		label = "default"
	}
	return strings.TrimSpace(username), strings.TrimSpace(label), nil
}

func newCommandPool(ctx context.Context) (*pgxpool.Pool, error) {
	databaseURL := strings.TrimSpace(os.Getenv("OPENBRAIN_DATABASE_URL"))
	if databaseURL == "" {
		return nil, fmt.Errorf("OPENBRAIN_DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return pool, nil
}

type thoughtServiceRunner struct {
	authStore      *postgres.AuthStore
	thoughtService *thoughts.Service
}

func (r *thoughtServiceRunner) FindUserByUsername(ctx context.Context, username string) (auth.User, error) {
	return r.authStore.FindUserByUsername(ctx, username)
}

func (r *thoughtServiceRunner) CreateThought(ctx context.Context, input thoughts.CreateThoughtInput) (thoughts.Thought, error) {
	return r.thoughtService.CreateThought(ctx, input)
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `Usage:
  openbrain start
  openbrain user update <username> --password <password> [--token-label <label>]
  openbrain user delete <username>
  openbrain token create <username> [--label <label>]
  openbrain token list <username>
  openbrain token delete <username> --label <label>
  openbrain thought add <username> <content> [--exposure-scope <scope>] [--tag <tag> ...]

Examples:
  go run ./cmd/openbrain start
  OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable go run ./cmd/openbrain user update demo --password demo-password
  OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable go run ./cmd/openbrain token create demo --label laptop
  OPENBRAIN_DATABASE_URL=postgres://openbrain:openbrain@127.0.0.1:5432/openbrain?sslmode=disable go run ./cmd/openbrain thought add demo 'remember pgx vectors' --tag pgx --tag notes
`)
}
