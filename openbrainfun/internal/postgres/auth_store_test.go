package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jrhy/sandbox/openbrainfun/internal/auth"
)

func TestAuthStoreFindUserByUsernameReturnsScannedUser(t *testing.T) {
	userID := uuid.New()
	createdAt := time.Unix(1700000000, 0).UTC()
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, query string, args ...any) rowScanner {
			if len(args) != 1 || args[0] != "alice" {
				t.Fatalf("args = %#v, want alice", args)
			}
			return fakeRow{scanFunc: func(dest ...any) error {
				*(dest[0].(*uuid.UUID)) = userID
				*(dest[1].(*string)) = "alice"
				*(dest[2].(*string)) = "hash"
				*(dest[3].(*time.Time)) = createdAt
				*(dest[4].(*time.Time)) = createdAt
				return nil
			}}
		},
	}

	store := NewAuthStore(db)
	user, err := store.FindUserByUsername(context.Background(), "alice")
	if err != nil {
		t.Fatalf("FindUserByUsername() error = %v", err)
	}
	if user.ID != userID || user.Username != "alice" {
		t.Fatalf("user = %+v, want scanned values", user)
	}
}

func TestAuthStoreFindUserByUsernameTranslatesNotFound(t *testing.T) {
	db := &fakeDB{
		queryRowFunc: func(ctx context.Context, query string, args ...any) rowScanner {
			return fakeRow{scanFunc: func(dest ...any) error { return sql.ErrNoRows }}
		},
	}

	store := NewAuthStore(db)
	_, err := store.FindUserByUsername(context.Background(), "missing")
	if err != auth.ErrUserNotFound {
		t.Fatalf("error = %v, want %v", err, auth.ErrUserNotFound)
	}
}

type fakeDB struct {
	queryRowFunc func(ctx context.Context, query string, args ...any) rowScanner
	queryFunc    func(ctx context.Context, query string, args ...any) (rowsScanner, error)
	execFunc     func(ctx context.Context, query string, args ...any) (commandTag, error)
}

func (f *fakeDB) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return f.queryRowFunc(ctx, query, args...)
}

func (f *fakeDB) QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error) {
	if f.queryFunc == nil {
		return &fakeAuthRows{}, nil
	}
	return f.queryFunc(ctx, query, args...)
}

func (f *fakeDB) ExecContext(ctx context.Context, query string, args ...any) (commandTag, error) {
	if f.execFunc == nil {
		return fakeCommandTag{}, nil
	}
	return f.execFunc(ctx, query, args...)
}

type fakeRow struct {
	scanFunc func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error {
	return f.scanFunc(dest...)
}

type fakeCommandTag struct{}

func (fakeCommandTag) RowsAffected() int64 { return 1 }

type fakeAuthRows struct {
	nextIndex int
	values    []func(dest ...any) error
	err       error
}

func (f *fakeAuthRows) Next() bool {
	return f.nextIndex < len(f.values)
}

func (f *fakeAuthRows) Scan(dest ...any) error {
	scan := f.values[f.nextIndex]
	f.nextIndex++
	return scan(dest...)
}

func (f *fakeAuthRows) Err() error { return f.err }

func (f *fakeAuthRows) Close() {}
