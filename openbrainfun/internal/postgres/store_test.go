package postgres

import "testing"

func TestNewStoreNilPool(t *testing.T) {
	s := NewStore(nil)
	if s == nil {
		t.Fatal("expected store")
	}
}
