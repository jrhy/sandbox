package auth

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewUserRejectsBlankUsername(t *testing.T) {
	_, err := NewUser(uuid.New(), "   ", "hash")
	if !errors.Is(err, ErrInvalidUsername) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidUsername)
	}
}
