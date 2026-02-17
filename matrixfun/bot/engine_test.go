package main

import (
	"context"
	"errors"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestConversationStore(t *testing.T) {
	t.Parallel()

	store := NewConversationStore(2)
	roomID := id.RoomID("!room:example.org")
	store.Add(roomID, TranscriptMessage{EventID: "$1", Sender: "@alice:example.org", Body: "one"})
	store.Add(roomID, TranscriptMessage{EventID: "$2", Sender: "@bob:example.org", Body: "two"})
	store.Add(roomID, TranscriptMessage{EventID: "$3", Sender: "@carol:example.org", Body: "three"})

	recent := store.Recent(roomID, 10)
	if len(recent) != 2 {
		t.Fatalf("recent len = %d, want 2", len(recent))
	}
	if recent[0].EventID != "$2" || recent[1].EventID != "$3" {
		t.Fatalf("unexpected recent messages: %+v", recent)
	}
	if _, ok := store.SenderForEvent(context.Background(), roomID, "$1"); ok {
		t.Fatal("expected trimmed event to be removed from index")
	}
	sender, ok := store.SenderForEvent(context.Background(), roomID, "$3")
	if !ok || sender != "@carol:example.org" {
		t.Fatalf("sender lookup = (%q,%v), want (@carol:example.org,true)", sender, ok)
	}
}

func TestConversationStore_ClearRoom(t *testing.T) {
	t.Parallel()

	store := NewConversationStore(10)
	roomA := id.RoomID("!a:example.org")
	roomB := id.RoomID("!b:example.org")
	store.Add(roomA, TranscriptMessage{EventID: "$a1", Sender: "@alice:example.org", Body: "one"})
	store.Add(roomB, TranscriptMessage{EventID: "$b1", Sender: "@bob:example.org", Body: "two"})

	store.ClearRoom(roomA)
	if got := store.Recent(roomA, 10); len(got) != 0 {
		t.Fatalf("expected room A cleared, got %+v", got)
	}
	if _, ok := store.SenderForEvent(context.Background(), roomA, "$a1"); ok {
		t.Fatal("expected room A index cleared")
	}
	if got := store.Recent(roomB, 10); len(got) != 1 {
		t.Fatalf("expected room B untouched, got %+v", got)
	}
}

func TestCallOllama(t *testing.T) {
	t.Parallel()

	client := &fakeOllamaClient{
		resp: OllamaChatResponse{Message: ChatMessage{Content: "  hello world  "}},
	}
	content, err := CallOllama(context.Background(), client, LLMConfig{Model: "mistral-small3.1", SystemPrompt: "system"}, PromptContext{Current: TranscriptMessage{Body: "hi"}})
	if err != nil {
		t.Fatalf("CallOllama() error = %v", err)
	}
	if content != "hello world" {
		t.Fatalf("content = %q, want %q", content, "hello world")
	}
	if client.lastReq.Model != "mistral-small3.1" {
		t.Fatalf("unexpected request model: %q", client.lastReq.Model)
	}
}

func TestCallOllamaErrors(t *testing.T) {
	t.Parallel()

	_, err := CallOllama(context.Background(), nil, LLMConfig{Model: "mistral-small3.1"}, PromptContext{})
	if err == nil {
		t.Fatal("expected nil client error")
	}

	_, err = CallOllama(context.Background(), &fakeOllamaClient{}, LLMConfig{}, PromptContext{})
	if err == nil {
		t.Fatal("expected missing model error")
	}

	_, err = CallOllama(context.Background(), &fakeOllamaClient{err: errors.New("boom")}, LLMConfig{Model: "mistral-small3.1"}, PromptContext{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}

func TestTranscriptFromEvent(t *testing.T) {
	t.Parallel()

	rel := (&event.RelatesTo{}).SetReplyTo("$parent").SetThread("$root", "")
	evt := &event.Event{
		ID:        "$evt",
		RoomID:    "!room:example.org",
		Sender:    "@alice:example.org",
		Timestamp: 1700000000000,
		Content: event.Content{Parsed: &event.MessageEventContent{
			MsgType:   event.MsgText,
			Body:      " hello ",
			RelatesTo: rel,
		}},
	}
	msg := TranscriptFromEvent(evt, true, false)
	if msg.EventID != "$evt" || msg.RoomID != "!room:example.org" || msg.Sender != "@alice:example.org" {
		t.Fatalf("unexpected identifiers: %+v", msg)
	}
	if msg.Body != "hello" || msg.ReplyTo != "$parent" || msg.ThreadRoot != "$root" {
		t.Fatalf("unexpected content extraction: %+v", msg)
	}
	if !msg.MentionsBot || msg.IsBot {
		t.Fatalf("unexpected mention/bot flags: %+v", msg)
	}
}

type fakeOllamaClient struct {
	resp    OllamaChatResponse
	err     error
	lastReq OllamaChatRequest
}

func (f *fakeOllamaClient) Chat(_ context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return OllamaChatResponse{}, f.err
	}
	return f.resp, nil
}
