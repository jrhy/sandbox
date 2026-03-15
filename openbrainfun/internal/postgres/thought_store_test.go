package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jrhy/sandbox/openbrainfun/internal/metadata"
	"github.com/jrhy/sandbox/openbrainfun/internal/thoughts"
)

func TestThoughtStoreGetThoughtScopesByUserID(t *testing.T) {
	userID := uuid.New()
	thoughtID := uuid.New()
	updatedAt := time.Unix(1700000000, 0).UTC()
	db := &fakeThoughtDB{
		queryRowFunc: func(ctx context.Context, query string, args ...any) pgx.Row {
			if len(args) != 2 || args[0] != userID || args[1] != thoughtID {
				t.Fatalf("args = %#v, want userID and thoughtID", args)
			}
			return fakePGXRow{scanFunc: func(dest ...any) error {
				*(dest[0].(*uuid.UUID)) = thoughtID
				*(dest[1].(*uuid.UUID)) = userID
				*(dest[2].(*string)) = "remember pgx"
				*(dest[3].(*string)) = string(thoughts.ExposureScopeLocalOnly)
				*(dest[4].(*[]string)) = []string{"pgx"}
				*(dest[5].(*[]byte)) = []byte(`{"summary":"remember pgx","topics":["pgx"],"entities":[]}`)
				*(dest[6].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[7].(*string)) = "all-minilm:22m"
				*(dest[8].(*string)) = ""
				*(dest[9].(*time.Time)) = updatedAt
				*(dest[10].(*time.Time)) = updatedAt
				return nil
			}}
		},
	}

	store := NewThoughtStore(db)
	thought, err := store.GetThought(context.Background(), userID, thoughtID)
	if err != nil {
		t.Fatalf("GetThought() error = %v", err)
	}
	if thought.ID != thoughtID || thought.UserID != userID {
		t.Fatalf("thought = %+v, want scanned values", thought)
	}
	if thought.Metadata.Summary != "remember pgx" {
		t.Fatalf("Summary = %q, want scanned metadata", thought.Metadata.Summary)
	}
	if thought.EmbeddingModel != "all-minilm:22m" {
		t.Fatalf("EmbeddingModel = %q, want scanned model", thought.EmbeddingModel)
	}
}

func TestThoughtStoreMarkReadyPersistsMetadataAndEmbeddingModel(t *testing.T) {
	thoughtID := uuid.New()
	processedAt := time.Unix(1700001111, 0).UTC()
	db := &fakeThoughtDB{
		execFunc: func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
			if len(args) != 6 {
				t.Fatalf("len(args) = %d, want 6", len(args))
			}
			if args[0] != thoughtID {
				t.Fatalf("args[0] = %v, want thoughtID", args[0])
			}
			if args[1] != string(thoughts.IngestStatusReady) {
				t.Fatalf("args[1] = %v, want ready status", args[1])
			}
			metadataJSON, ok := args[2].([]byte)
			if !ok {
				t.Fatalf("args[2] type = %T, want []byte", args[2])
			}
			got := decodeMetadata(metadataJSON)
			if got.Summary != "remember pgx" || len(got.Topics) != 1 || got.Topics[0] != "pgx" {
				t.Fatalf("decoded metadata = %+v, want normalized metadata", got)
			}
			if args[3] != "[0.1,0.2,0.3]" {
				t.Fatalf("args[3] = %v, want vector literal", args[3])
			}
			if args[4] != "all-minilm:22m" {
				t.Fatalf("args[4] = %v, want model", args[4])
			}
			if args[5] != processedAt {
				t.Fatalf("args[5] = %v, want processedAt", args[5])
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	store := NewThoughtStore(db)
	err := store.MarkReady(context.Background(), thoughts.MarkReadyParams{
		ThoughtID:      thoughtID,
		Embedding:      []float32{0.1, 0.2, 0.3},
		EmbeddingModel: "all-minilm:22m",
		Metadata:       metadata.Normalize(map[string]any{"summary": "remember pgx", "topics": []string{"pgx"}}),
		ProcessedAt:    processedAt,
	})
	if err != nil {
		t.Fatalf("MarkReady() error = %v", err)
	}
}

func TestThoughtStoreDeleteThoughtTranslatesMissingRow(t *testing.T) {
	db := &fakeThoughtDB{
		execFunc: func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("DELETE 0"), nil
		},
	}

	store := NewThoughtStore(db)
	err := store.DeleteThought(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, thoughts.ErrThoughtNotFound) {
		t.Fatalf("error = %v, want %v", err, thoughts.ErrThoughtNotFound)
	}
}

type fakeThoughtDB struct {
	queryRowFunc func(ctx context.Context, query string, args ...any) pgx.Row
	execFunc     func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error)
	queryFunc    func(ctx context.Context, query string, args ...any) (pgx.Rows, error)
}

func (f *fakeThoughtDB) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	return f.queryRowFunc(ctx, query, args...)
}

func (f *fakeThoughtDB) Exec(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc == nil {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}
	return f.execFunc(ctx, query, args...)
}

func (f *fakeThoughtDB) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	if f.queryFunc == nil {
		return &fakeRows{}, nil
	}
	return f.queryFunc(ctx, query, args...)
}

type fakePGXRow struct {
	scanFunc func(dest ...any) error
}

func (f fakePGXRow) Scan(dest ...any) error { return f.scanFunc(dest...) }

type fakeRows struct{}

func (*fakeRows) Close()                                       {}
func (*fakeRows) Err() error                                   { return nil }
func (*fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (*fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (*fakeRows) Next() bool                                   { return false }
func (*fakeRows) Scan(dest ...any) error                       { return nil }
func (*fakeRows) Values() ([]any, error)                       { return nil, nil }
func (*fakeRows) RawValues() [][]byte                          { return nil }
func (*fakeRows) Conn() *pgx.Conn                              { return nil }
func (*fakeRows) NextRow() bool                                { return false }
