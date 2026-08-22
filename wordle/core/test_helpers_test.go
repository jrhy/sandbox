package core

import (
	"os"
	"testing"
)

func loadGuesses(path string) ([]Guess, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	guesses, err := ParseGuesses(b)
	if err != nil {
		return nil, err
	}
	return guesses, NormalizeGuesses(guesses)
}

func TestValidationRejectsNonFiveLetterGuess(t *testing.T) {
	if err := NormalizeGuesses([]Guess{{Word: "FOUR"}}); err == nil {
		t.Fatal("expected five-letter validation error")
	}
}
