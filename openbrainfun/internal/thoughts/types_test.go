package thoughts

import "testing"

func TestNewThoughtRejectsBlankContent(t *testing.T) {
	_, err := NewThought(NewThoughtParams{Content: "   "})
	if err == nil {
		t.Fatal("expected error for blank content")
	}
}
