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
