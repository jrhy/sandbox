package thoughts

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewThoughtRejectsBlankContent(t *testing.T) {
	_, err := NewThought(NewThoughtParams{
		UserID:        uuid.New(),
		Content:       "   ",
		ExposureScope: ExposureScopeLocalOnly,
	})
	if !errors.Is(err, ErrBlankContent) {
		t.Fatalf("error = %v, want %v", err, ErrBlankContent)
	}
}

func TestNewThoughtInitializesMetadataDefaults(t *testing.T) {
	thought, err := NewThought(NewThoughtParams{
		UserID:        uuid.New(),
		Content:       "remember pgx",
		ExposureScope: ExposureScopeLocalOnly,
	})
	if err != nil {
		t.Fatalf("NewThought() error = %v", err)
	}
	if thought.Metadata.Summary != "No summary available." {
		t.Fatalf("Summary = %q, want default summary", thought.Metadata.Summary)
	}
	if thought.Metadata.Topics == nil || thought.Metadata.Entities == nil {
		t.Fatalf("Metadata = %+v, want initialized slices", thought.Metadata)
	}
}

func TestNewThoughtNormalizesNilTagsToEmptySlice(t *testing.T) {
	thought, err := NewThought(NewThoughtParams{
		UserID:        uuid.New(),
		Content:       "remember pgx",
		ExposureScope: ExposureScopeLocalOnly,
		UserTags:      nil,
	})
	if err != nil {
		t.Fatalf("NewThought() error = %v", err)
	}
	if thought.UserTags == nil {
		t.Fatalf("UserTags = nil, want empty slice")
	}
	if len(thought.UserTags) != 0 {
		t.Fatalf("len(UserTags) = %d, want 0", len(thought.UserTags))
	}
}
