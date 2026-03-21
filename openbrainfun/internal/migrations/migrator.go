package migrations

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

//go:embed 0001_initial.sql.tmpl
var initialTemplate string

//go:embed 0002_option_a.sql
var optionASQL string

const advisoryLockKey int64 = 2026032101

var vectorTypePattern = regexp.MustCompile(`^vector\((\d+)\)$`)

type db interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Definition struct {
	Version int
	Name    string
	SQL     string
}

type Migrator struct {
	migrations []Definition
}

func NewMigrator() Migrator {
	return Migrator{migrations: []Definition{{Version: 1, Name: "initial", SQL: initialTemplate}, {Version: 2, Name: "option_a_refresh_tracking", SQL: optionASQL}}}
}

func (m Migrator) Ensure(ctx context.Context, conn db, embedDimensions int) error {
	if _, err := conn.Exec(ctx, `select pg_advisory_lock($1)`, advisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), `select pg_advisory_unlock($1)`, advisoryLockKey)

	if _, err := conn.Exec(ctx, `
create table if not exists schema_migrations (
	version bigint primary key,
	name text not null,
	applied_at timestamptz not null default now()
)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	applied, err := m.appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		legacy, err := hasLegacySchema(ctx, conn)
		if err != nil {
			return err
		}
		if legacy {
			if _, err := conn.Exec(ctx, `insert into schema_migrations (version, name) values ($1, $2) on conflict (version) do nothing`, 1, "initial"); err != nil {
				return fmt.Errorf("baseline legacy schema: %w", err)
			}
			applied[1] = true
		}
	}

	for _, migration := range m.sorted() {
		if applied[migration.Version] {
			continue
		}
		sqlText := migration.SQL
		if migration.Version == 1 {
			rendered, err := RenderSchema(initialTemplate, embedDimensions)
			if err != nil {
				return fmt.Errorf("render initial schema: %w", err)
			}
			sqlText = rendered
		}
		if _, err := conn.Exec(ctx, sqlText); err != nil {
			return fmt.Errorf("apply migration %04d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := conn.Exec(ctx, `insert into schema_migrations (version, name) values ($1, $2)`, migration.Version, migration.Name); err != nil {
			return fmt.Errorf("record migration %04d_%s: %w", migration.Version, migration.Name, err)
		}
	}

	if err := verifyEmbeddingDimensions(ctx, conn, embedDimensions); err != nil {
		return err
	}
	return nil
}

func (m Migrator) sorted() []Definition {
	out := append([]Definition(nil), m.migrations...)
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

func (m Migrator) appliedVersions(ctx context.Context, conn db) (map[int]bool, error) {
	rows, err := conn.Query(ctx, `select version from schema_migrations`)
	if err != nil {
		if strings.Contains(err.Error(), "relation \"schema_migrations\" does not exist") {
			return map[int]bool{}, nil
		}
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return applied, nil
}

func hasLegacySchema(ctx context.Context, conn db) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, `select exists (select 1 from information_schema.tables where table_schema = current_schema() and table_name = 'thoughts')`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check legacy schema: %w", err)
	}
	return exists, nil
}

func verifyEmbeddingDimensions(ctx context.Context, conn db, want int) error {
	var formatted string
	if err := conn.QueryRow(ctx, `
select format_type(a.atttypid, a.atttypmod)
from pg_attribute a
join pg_class c on c.oid = a.attrelid
join pg_namespace n on n.oid = c.relnamespace
where n.nspname = current_schema()
  and c.relname = 'thoughts'
  and a.attname = 'embedding'
  and a.attnum > 0
  and not a.attisdropped`).Scan(&formatted); err != nil {
		return fmt.Errorf("read thoughts.embedding type: %w", err)
	}
	match := vectorTypePattern.FindStringSubmatch(strings.TrimSpace(formatted))
	if len(match) != 2 {
		return fmt.Errorf("unexpected thoughts.embedding type %q", formatted)
	}
	got, err := strconv.Atoi(match[1])
	if err != nil {
		return fmt.Errorf("parse thoughts.embedding dimensions from %q: %w", formatted, err)
	}
	if got != want {
		return fmt.Errorf("configured embedding dimensions %d do not match schema dimension %d", want, got)
	}
	return nil
}
