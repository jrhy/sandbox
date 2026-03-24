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
				fillThoughtScan(dest, thoughtID, userID, "remember pgx", thoughts.ExposureScopeLocalOnly, []string{"pgx"}, []byte(`{"summary":"remember pgx","topics":["pgx"],"entities":[]}`), updatedAt)
				*(dest[6].(*string)) = "all-minilm:22m"
				*(dest[7].(*string)) = "all-minilm:22m|embed:v1"
				*(dest[8].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[9].(*string)) = ""
				*(dest[10].(*string)) = "qwen3:0.6b"
				*(dest[11].(*string)) = "qwen3:0.6b|metadata:v1"
				*(dest[12].(*string)) = string(thoughts.IngestStatusReady)
				*(dest[13].(*string)) = ""
				*(dest[14].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[15].(*string)) = ""
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

func TestThoughtStoreCreateThoughtNormalizesNilTags(t *testing.T) {
	thoughtID := uuid.New()
	userID := uuid.New()
	updatedAt := time.Unix(1700000000, 0).UTC()
	db := &fakeThoughtDB{
		queryRowFunc: func(ctx context.Context, query string, args ...any) pgx.Row {
			if len(args) != 18 {
				t.Fatalf("len(args) = %d, want 18", len(args))
			}
			tags, ok := args[4].([]string)
			if !ok {
				t.Fatalf("args[4] type = %T, want []string", args[4])
			}
			if tags == nil {
				t.Fatal("args[4] = nil, want empty slice")
			}
			if len(tags) != 0 {
				t.Fatalf("len(tags) = %d, want 0", len(tags))
			}
			return fakePGXRow{scanFunc: func(dest ...any) error {
				fillThoughtScan(dest, thoughtID, userID, "remember pgx", thoughts.ExposureScopeLocalOnly, []string{}, []byte(`{"summary":"remember pgx","topics":["pgx"],"entities":[]}`), updatedAt)
				*(dest[6].(*string)) = ""
				*(dest[7].(*string)) = ""
				*(dest[8].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[9].(*string)) = ""
				*(dest[10].(*string)) = ""
				*(dest[11].(*string)) = ""
				*(dest[12].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[13].(*string)) = ""
				*(dest[14].(*string)) = string(thoughts.IngestStatusPending)
				*(dest[15].(*string)) = ""
				return nil
			}}
		},
	}

	store := NewThoughtStore(db)
	_, err := store.CreateThought(context.Background(), thoughts.Thought{
		ID:              thoughtID,
		UserID:          userID,
		Content:         "remember pgx",
		ExposureScope:   thoughts.ExposureScopeLocalOnly,
		UserTags:        nil,
		Metadata:        metadata.Normalize(nil),
		EmbeddingStatus: thoughts.IngestStatusPending,
		MetadataStatus:  thoughts.IngestStatusPending,
		IngestStatus:    thoughts.IngestStatusPending,
		CreatedAt:       updatedAt,
		UpdatedAt:       updatedAt,
	})
	if err != nil {
		t.Fatalf("CreateThought() error = %v", err)
	}
}

func TestThoughtStoreMarkProcessedPersistsMetadataAndEmbeddingModel(t *testing.T) {
	thoughtID := uuid.New()
	processedAt := time.Unix(1700001111, 0).UTC()
	db := &fakeThoughtDB{
		execFunc: func(ctx context.Context, query string, args ...any) (pgconn.CommandTag, error) {
			if len(args) != 16 {
				t.Fatalf("len(args) = %d, want 16", len(args))
			}
			if args[0] != thoughtID {
				t.Fatalf("args[0] = %v, want thoughtID", args[0])
			}
			if args[1] != processedAt {
				t.Fatalf("args[1] = %v, want processedAt", args[1])
			}
			if args[2] != string(thoughts.IngestStatusReady) {
				t.Fatalf("args[2] = %v, want ready status", args[2])
			}
			if args[3] != "" {
				t.Fatalf("args[3] = %v, want empty ingest error", args[3])
			}
			if args[4] != "[0.1,0.2,0.3]" {
				t.Fatalf("args[4] = %v, want vector literal", args[4])
			}
			if args[5] != "all-minilm:22m" {
				t.Fatalf("args[5] = %v, want model", args[5])
			}
			if args[6] != "all-minilm:22m|embed:v1" {
				t.Fatalf("args[6] = %v, want embedding fingerprint", args[6])
			}
			if args[7] != string(thoughts.IngestStatusReady) {
				t.Fatalf("args[7] = %v, want embedding ready", args[7])
			}
			if args[8] != "" {
				t.Fatalf("args[8] = %v, want empty embedding error", args[8])
			}
			if args[9] != processedAt {
				t.Fatalf("args[9] = %v, want embedding updated at", args[9])
			}
			metadataJSON, ok := args[10].([]byte)
			if !ok {
				t.Fatalf("args[2] type = %T, want []byte", args[2])
			}
			got := decodeMetadata(metadataJSON)
			if got.Summary != "remember pgx" || len(got.Topics) != 1 || got.Topics[0] != "pgx" {
				t.Fatalf("decoded metadata = %+v, want normalized metadata", got)
			}
			if args[11] != "qwen3:0.6b" {
				t.Fatalf("args[11] = %v, want metadata model", args[11])
			}
			if args[12] != "qwen3:0.6b|metadata:v1" {
				t.Fatalf("args[12] = %v, want metadata fingerprint", args[12])
			}
			if args[13] != string(thoughts.IngestStatusReady) {
				t.Fatalf("args[13] = %v, want metadata ready", args[13])
			}
			if args[14] != "" {
				t.Fatalf("args[14] = %v, want empty metadata error", args[14])
			}
			if args[15] != processedAt {
				t.Fatalf("args[15] = %v, want metadata updated at", args[15])
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}

	store := NewThoughtStore(db)
	err := store.MarkProcessed(context.Background(), thoughts.MarkProcessedParams{
		ThoughtID:            thoughtID,
		UpdateEmbedding:      true,
		Embedding:            []float32{0.1, 0.2, 0.3},
		EmbeddingModel:       "all-minilm:22m",
		EmbeddingFingerprint: "all-minilm:22m|embed:v1",
		EmbeddingStatus:      thoughts.IngestStatusReady,
		UpdateMetadata:       true,
		Metadata:             metadata.Normalize(map[string]any{"summary": "remember pgx", "topics": []string{"pgx"}}),
		MetadataModel:        "qwen3:0.6b",
		MetadataFingerprint:  "qwen3:0.6b|metadata:v1",
		MetadataStatus:       thoughts.IngestStatusReady,
		IngestStatus:         thoughts.IngestStatusReady,
		ProcessedAt:          processedAt,
	})
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
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

func TestThoughtStoreSearchSemanticUsesCosineSimilarityAndThreshold(t *testing.T) {
	userID := uuid.New()
	updatedAt := time.Unix(1700000000, 0).UTC()
	db := &fakeThoughtDB{
		queryFunc: func(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
			if len(args) != 8 {
				t.Fatalf("len(args) = %d, want 8", len(args))
			}
			if args[0] != userID {
				t.Fatalf("args[0] = %v, want userID", args[0])
			}
			if args[1] != "[0.1,0.2,0.3]" {
				t.Fatalf("args[1] = %v, want vector literal", args[1])
			}
			if args[5] != 0.5 {
				t.Fatalf("args[5] = %v, want threshold 0.5", args[5])
			}
			return &fakeRows{scanRows: []func(dest ...any) error{
				func(dest ...any) error {
					fillThoughtScan(dest, uuid.New(), userID, "career change note", thoughts.ExposureScopeRemoteOK, []string{"career"}, []byte(`{"summary":"career","topics":["career"],"entities":[]}`), updatedAt)
					*(dest[6].(*string)) = "all-minilm:22m"
					*(dest[7].(*string)) = "all-minilm:22m|embed:v1"
					*(dest[8].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[9].(*string)) = ""
					*(dest[10].(*string)) = "qwen3:0.6b"
					*(dest[11].(*string)) = "qwen3:0.6b|metadata:v1"
					*(dest[12].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[13].(*string)) = ""
					*(dest[14].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[15].(*string)) = ""
					*(dest[18].(*float64)) = 0.88
					return nil
				},
			}}, nil
		},
	}

	store := NewThoughtStore(db)
	got, err := store.SearchSemantic(context.Background(), thoughts.SearchSemanticParams{
		UserID:         userID,
		QueryEmbedding: []float32{0.1, 0.2, 0.3},
		Threshold:      0.5,
		Exposure:       string(thoughts.ExposureScopeRemoteOK),
		Tag:            "career",
		Page:           2,
		PageSize:       5,
	})
	if err != nil {
		t.Fatalf("SearchSemantic() error = %v", err)
	}
	if len(got) != 1 || got[0].Similarity != 0.88 || got[0].Thought.Content != "career change note" {
		t.Fatalf("got = %+v, want scored semantic result", got)
	}
}

func TestThoughtStoreRelatedThoughtsUsesAnchorEmbedding(t *testing.T) {
	userID := uuid.New()
	anchorID := uuid.New()
	updatedAt := time.Unix(1700000000, 0).UTC()
	db := &fakeThoughtDB{
		queryFunc: func(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
			if len(args) != 5 {
				t.Fatalf("len(args) = %d, want 5", len(args))
			}
			if args[0] != userID || args[1] != anchorID {
				t.Fatalf("args = %#v, want userID and anchorID first", args)
			}
			if args[4] != 3 {
				t.Fatalf("args[4] = %v, want limit 3", args[4])
			}
			return &fakeRows{scanRows: []func(dest ...any) error{
				func(dest ...any) error {
					fillThoughtScan(dest, uuid.New(), userID, "similar thought", thoughts.ExposureScopeLocalOnly, []string{"career"}, []byte(`{"summary":"similar","topics":["career"],"entities":[]}`), updatedAt)
					*(dest[6].(*string)) = "all-minilm:22m"
					*(dest[7].(*string)) = "all-minilm:22m|embed:v1"
					*(dest[8].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[9].(*string)) = ""
					*(dest[10].(*string)) = "qwen3:0.6b"
					*(dest[11].(*string)) = "qwen3:0.6b|metadata:v1"
					*(dest[12].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[13].(*string)) = ""
					*(dest[14].(*string)) = string(thoughts.IngestStatusReady)
					*(dest[15].(*string)) = ""
					*(dest[18].(*float64)) = 0.91
					return nil
				},
			}}, nil
		},
	}

	store := NewThoughtStore(db)
	got, err := store.RelatedThoughts(context.Background(), thoughts.RelatedThoughtsParams{
		UserID:    userID,
		ThoughtID: anchorID,
		Limit:     3,
	})
	if err != nil {
		t.Fatalf("RelatedThoughts() error = %v", err)
	}
	if len(got) != 1 || got[0].Similarity != 0.91 || got[0].Thought.Content != "similar thought" {
		t.Fatalf("got = %+v, want scored related result", got)
	}
}

func fillThoughtScan(dest []any, thoughtID, userID uuid.UUID, content string, exposure thoughts.ExposureScope, tags []string, metadataJSON []byte, updatedAt time.Time) {
	*(dest[0].(*uuid.UUID)) = thoughtID
	*(dest[1].(*uuid.UUID)) = userID
	*(dest[2].(*string)) = content
	*(dest[3].(*string)) = string(exposure)
	*(dest[4].(*[]string)) = tags
	*(dest[5].(*[]byte)) = metadataJSON
	*(dest[16].(*time.Time)) = updatedAt
	*(dest[17].(*time.Time)) = updatedAt
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

type fakeRows struct {
	scanRows []func(dest ...any) error
	index    int
}

func (*fakeRows) Close()                                       {}
func (*fakeRows) Err() error                                   { return nil }
func (*fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.NewCommandTag("SELECT 0") }
func (*fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (f *fakeRows) Next() bool {
	return f.index < len(f.scanRows)
}
func (f *fakeRows) Scan(dest ...any) error {
	if f.index >= len(f.scanRows) {
		return nil
	}
	scan := f.scanRows[f.index]
	f.index++
	return scan(dest...)
}
func (*fakeRows) Values() ([]any, error) { return nil, nil }
func (*fakeRows) RawValues() [][]byte    { return nil }
func (*fakeRows) Conn() *pgx.Conn        { return nil }
func (f *fakeRows) NextRow() bool        { return f.Next() }
