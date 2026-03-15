package metadata

import "testing"

func TestNormalizeProvidesSafeDefaults(t *testing.T) {
	got := Normalize(map[string]any{"topics": "not-an-array"})
	if got.Summary == "" {
		t.Fatal("expected non-empty summary default")
	}
	if got.Topics == nil {
		t.Fatal("expected topics slice to be initialized")
	}
}
