package main

import (
	"context"
	"strings"
	"testing"

	"maunium.net/go/mautrix/id"
)

func TestApplyPolicyToContextConfig(t *testing.T) {
	t.Parallel()

	base := ContextConfig{MaxMessages: 20, TokenBudget: 1000, SummaryMaxItems: 6}
	policy := RoomPolicy{ElisionAggressiveness: "high", KeepKeywords: []string{"decision"}, DropKeywords: []string{"lol"}}
	cfg := applyPolicyToContextConfig(base, policy)
	if cfg.MaxMessages >= base.MaxMessages || cfg.TokenBudget >= base.TokenBudget {
		t.Fatalf("expected high aggressiveness to reduce context, got %+v", cfg)
	}
	if len(cfg.KeepKeywords) != 1 || cfg.KeepKeywords[0] != "decision" {
		t.Fatalf("unexpected keep keywords: %+v", cfg.KeepKeywords)
	}
	if len(cfg.DropKeywords) != 1 || cfg.DropKeywords[0] != "lol" {
		t.Fatalf("unexpected drop keywords: %+v", cfg.DropKeywords)
	}
}

func TestComposeSystemPrompt(t *testing.T) {
	t.Parallel()

	prompt := composeSystemPrompt("Base", RoomPolicy{CustomSystemPrompt: "Speak like a pirate", CustomElisionCriteria: "Drop memes"})
	if !strings.Contains(prompt, "Base") || !strings.Contains(prompt, "Speak like a pirate") || !strings.Contains(prompt, "Drop memes") {
		t.Fatalf("unexpected composed prompt: %q", prompt)
	}
}

func TestHandlePolicyCommand_SetShowReset(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	trusted := map[id.UserID]bool{"@admin:example.org": true}
	roomID := id.RoomID("!room:example.org")

	reply, handled, err := handlePolicyCommand(context.Background(), "@admin:example.org", roomID, "!policy set elision high", store, LLMConfig{}, &fakeOllamaClient{}, trusted)
	if err != nil || !handled {
		t.Fatalf("set elision failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(reply, "elision_aggressiveness: high") {
		t.Fatalf("unexpected policy reply: %q", reply)
	}

	reply, handled, err = handlePolicyCommand(context.Background(), "@admin:example.org", roomID, "!policy show", store, LLMConfig{}, &fakeOllamaClient{}, trusted)
	if err != nil || !handled {
		t.Fatalf("policy show failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(reply, "Room policy") {
		t.Fatalf("unexpected show reply: %q", reply)
	}

	reply, handled, err = handlePolicyCommand(context.Background(), "@admin:example.org", roomID, "!policy reset", store, LLMConfig{}, &fakeOllamaClient{}, trusted)
	if err != nil || !handled {
		t.Fatalf("policy reset failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(reply, "reset") {
		t.Fatalf("unexpected reset reply: %q", reply)
	}
}

func TestHandlePolicyCommand_Trust(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	roomID := id.RoomID("!room:example.org")

	reply, handled, err := handlePolicyCommand(context.Background(), "@user:example.org", roomID, "!policy set prompt do things", store, LLMConfig{}, &fakeOllamaClient{}, nil)
	if err != nil || !handled {
		t.Fatalf("unexpected handling: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(reply, "Only trusted members") {
		t.Fatalf("expected trust message, got %q", reply)
	}
}

func TestHandlePolicyCommand_Edit(t *testing.T) {
	t.Parallel()

	store, err := NewRoomMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewRoomMemoryStore() error = %v", err)
	}
	trusted := map[id.UserID]bool{"@admin:example.org": true}
	roomID := id.RoomID("!room:example.org")
	client := &fakeOllamaClient{resp: OllamaChatResponse{Message: ChatMessage{Content: `{"custom_system_prompt":"Be terse","custom_elision_criteria":"Drop jokes","elision_aggressiveness":"medium","keep_keywords":["decision"],"drop_keywords":["meme"]}`}}}

	reply, handled, err := handlePolicyCommand(context.Background(), "@admin:example.org", roomID, "!policy edit make responses terse", store, LLMConfig{Model: "mistral-small3.1"}, client, trusted)
	if err != nil || !handled {
		t.Fatalf("policy edit failed: handled=%v err=%v", handled, err)
	}
	if !strings.Contains(reply, "Policy updated via model edit") {
		t.Fatalf("unexpected edit reply: %q", reply)
	}
	mem, err := store.Load(roomID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if mem.Policy.CustomSystemPrompt != "Be terse" || mem.Policy.CustomElisionCriteria != "Drop jokes" {
		t.Fatalf("unexpected stored policy: %+v", mem.Policy)
	}
}
