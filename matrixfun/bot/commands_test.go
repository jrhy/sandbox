package main

import (
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestHandleCommand_BaseCommands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "!ping", want: "pong"},
		{in: "!echo hello", want: "hello"},
		{in: "!help", want: "Commands: !ping, !echo <text>, !help, !memory, !memory forget <text>, !memory clear, !policy ..."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			reply, handled := handleCommand(tc.in, "!room:example.org", nil, nil)
			if !handled {
				t.Fatalf("expected handled for %q", tc.in)
			}
			if reply != tc.want {
				t.Fatalf("reply = %q, want %q", reply, tc.want)
			}
		})
	}

	reply, handled := handleCommand("not-a-command", "!room:example.org", nil, nil)
	if handled || reply != "" {
		t.Fatalf("unexpected handling for non-command: handled=%v reply=%q", handled, reply)
	}
}

func TestHandleCommand_Memory(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	roomID := id.RoomID("!room:example.org")
	if err := store.Save(roomID, RoomMemory{
		RoomID:         string(roomID),
		RollingSummary: "summary text",
		DurableMemory:  []string{"Constraint: keep local", "I prefer concise replies"},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reply, handled := handleCommand("!memory", roomID, store, nil)
	if !handled {
		t.Fatal("expected !memory handled")
	}
	if !strings.Contains(reply, "Summary: summary text") {
		t.Fatalf("unexpected !memory output: %q", reply)
	}
	if !strings.Contains(reply, "Durable memory:") {
		t.Fatalf("expected durable memory section: %q", reply)
	}
}

func TestHandleCommand_MemoryClear(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	conversation := NewConversationStore(20)
	roomID := id.RoomID("!room:example.org")
	conversation.Add(roomID, TranscriptMessage{EventID: "$1", Sender: "@alice:example.org", Body: "hello"})

	if err := store.Save(roomID, RoomMemory{RoomID: string(roomID), RollingSummary: "old", DurableMemory: []string{"I prefer concise replies"}}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reply, handled := handleCommand("!memory clear", roomID, store, conversation)
	if !handled {
		t.Fatal("expected !memory clear handled")
	}
	if reply != "Memory cleared for this room." {
		t.Fatalf("reply = %q", reply)
	}

	mem, err := store.Load(roomID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if mem.RollingSummary != "" || len(mem.DurableMemory) != 0 {
		t.Fatalf("expected cleared memory, got %+v", mem)
	}

	recent := conversation.Recent(roomID, 10)
	if len(recent) != 0 {
		t.Fatalf("expected conversation cleared, got %+v", recent)
	}
}

func TestHandleCommand_MemoryForget(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	roomID := id.RoomID("!room:example.org")
	if err := store.Save(roomID, RoomMemory{
		RoomID:        string(roomID),
		DurableMemory: []string{"Julian bath and homework", "Remember milk"},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reply, handled := handleCommand("!memory forget julian", roomID, store, nil)
	if !handled {
		t.Fatal("expected !memory forget handled")
	}
	if !strings.Contains(reply, "Removed 1 durable memory entry.") {
		t.Fatalf("unexpected forget reply: %q", reply)
	}

	mem, err := store.Load(roomID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(mem.DurableMemory) != 1 || mem.DurableMemory[0] != "Remember milk" {
		t.Fatalf("unexpected durable memory after forget: %+v", mem.DurableMemory)
	}

	reply, handled = handleCommand("!memory forget missing", roomID, store, nil)
	if !handled {
		t.Fatal("expected !memory forget handled")
	}
	if !strings.Contains(reply, "No durable memory entries matched") {
		t.Fatalf("unexpected no-match reply: %q", reply)
	}
}

func TestFormatRoomMemory(t *testing.T) {
	t.Parallel()

	reply := formatRoomMemory(RoomMemory{})
	if !strings.Contains(reply, "Summary: (empty)") {
		t.Fatalf("unexpected empty memory formatting: %q", reply)
	}
}
