package auth

import "testing"

func TestHashTokenDeterministic(t *testing.T) {
	first := HashToken("token-value")
	second := HashToken("token-value")
	if first == "" {
		t.Fatal("HashToken() returned empty string")
	}
	if first != second {
		t.Fatalf("HashToken() = %q and %q, want deterministic output", first, second)
	}
}
