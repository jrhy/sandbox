package main

import (
	"path/filepath"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestRoomMemoryStore_SaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := NewRoomMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}

	roomID := id.RoomID("!room:example.org")
	mem := RoomMemory{
		RollingSummary: "Important decisions from prior context",
		DurableMemory:  []string{"I prefer concise answers", "Constraint: no external network calls"},
	}
	if err := store.Save(roomID, mem); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := store.Load(roomID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.RoomID != string(roomID) {
		t.Fatalf("RoomID = %q, want %q", loaded.RoomID, roomID)
	}
	if loaded.RollingSummary != mem.RollingSummary {
		t.Fatalf("RollingSummary = %q, want %q", loaded.RollingSummary, mem.RollingSummary)
	}
	if len(loaded.DurableMemory) != 2 {
		t.Fatalf("DurableMemory len = %d, want 2", len(loaded.DurableMemory))
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be populated")
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("json files len = %d, want 1", len(files))
	}
	if strings.Contains(filepath.Base(files[0]), "!room:example.org") {
		t.Fatalf("expected escaped room id filename, got %q", filepath.Base(files[0]))
	}
}

func TestRoomMemoryStore_MirrorWritesPolicyFile(t *testing.T) {
	t.Parallel()

	storeDir := t.TempDir()
	mirrorDir := t.TempDir()
	store, err := NewRoomMemoryStoreWithMirror(storeDir, mirrorDir)
	if err != nil {
		t.Fatalf("NewRoomMemoryStoreWithMirror() error = %v", err)
	}
	roomID := id.RoomID("!room:example.org")
	if err := store.Save(roomID, RoomMemory{
		RoomID: string(roomID),
		Policy: RoomPolicy{
			CustomSystemPrompt: "Be concise",
		},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(mirrorDir, "*.policy.json"))
	if err != nil {
		t.Fatalf("glob error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("mirror files = %d, want 1", len(matches))
	}
}

func TestRoomMemoryStore_LoadMissingRoom(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}

	loaded, err := store.Load("!missing:example.org")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.RoomID != "!missing:example.org" {
		t.Fatalf("RoomID = %q", loaded.RoomID)
	}
	if loaded.RollingSummary != "" || len(loaded.DurableMemory) != 0 {
		t.Fatalf("expected empty memory for missing room, got %+v", loaded)
	}
}

func TestExtractDurableMemoryCandidates(t *testing.T) {
	t.Parallel()

	candidates := ExtractDurableMemoryCandidates(
		"hello there",
		"I prefer short responses. Action item: add retry logic. lol",
		"Constraint: must run locally",
	)
	joined := strings.Join(candidates, "\n")
	if !strings.Contains(strings.ToLower(joined), "prefer short responses") {
		t.Fatalf("missing preference candidate: %q", joined)
	}
	if !strings.Contains(strings.ToLower(joined), "action item") {
		t.Fatalf("missing action item candidate: %q", joined)
	}
	if !strings.Contains(strings.ToLower(joined), "constraint") {
		t.Fatalf("missing constraint candidate: %q", joined)
	}
	if strings.Contains(strings.ToLower(joined), "hello there") {
		t.Fatalf("unexpected low-value candidate included: %q", joined)
	}
}

func TestMergeDurableMemory(t *testing.T) {
	t.Parallel()

	merged := MergeDurableMemory(
		[]string{"I prefer concise replies", "Constraint: no cloud calls"},
		[]string{"i prefer concise replies", "Action item: add room memory store"},
		3,
	)
	if len(merged) != 3 {
		t.Fatalf("len = %d, want 3", len(merged))
	}
	joined := strings.ToLower(strings.Join(merged, "\n"))
	if strings.Count(joined, "prefer concise replies") != 1 {
		t.Fatalf("expected deduped preference, got %q", joined)
	}
	if !strings.Contains(joined, "action item") {
		t.Fatalf("missing new item, got %q", joined)
	}
}
