package main

import (
	"context"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

func TestShouldRespond_Table(t *testing.T) {
	t.Parallel()

	cfg := RoutingConfig{BotUserID: "@bot:example.org", CommandPrefix: "!", AmbientMode: false}
	tests := []struct {
		name    string
		cfg     RoutingConfig
		signals MessageSignals
		want    RoutingDecision
	}{
		{name: "non text ignored", cfg: cfg, signals: MessageSignals{}, want: RoutingDecision{Respond: false, Reason: "not_text"}},
		{name: "dm responds", cfg: cfg, signals: MessageSignals{IsText: true, IsDM: true}, want: RoutingDecision{Respond: true, Reason: "dm"}},
		{name: "command responds", cfg: cfg, signals: MessageSignals{IsText: true, StartsWithCommand: true}, want: RoutingDecision{Respond: true, Reason: "command"}},
		{name: "mention responds", cfg: cfg, signals: MessageSignals{IsText: true, MentionsBot: true}, want: RoutingDecision{Respond: true, Reason: "mention"}},
		{name: "reply to bot responds", cfg: cfg, signals: MessageSignals{IsText: true, ReplyTargetsBot: true}, want: RoutingDecision{Respond: true, Reason: "reply_to_bot"}},
		{name: "bot thread responds", cfg: cfg, signals: MessageSignals{IsText: true, ThreadParentIsBot: true}, want: RoutingDecision{Respond: true, Reason: "bot_thread"}},
		{name: "directed to other user ignored", cfg: cfg, signals: MessageSignals{IsText: true, DirectedToOtherUser: true}, want: RoutingDecision{Respond: false, Reason: "directed_to_other_user"}},
		{name: "ambient fallback responds", cfg: RoutingConfig{AmbientMode: true}, signals: MessageSignals{IsText: true}, want: RoutingDecision{Respond: true, Reason: "ambient"}},
		{name: "group default ignored", cfg: cfg, signals: MessageSignals{IsText: true}, want: RoutingDecision{Respond: false, Reason: "not_addressed"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldRespond(tc.cfg, tc.signals)
			if got != tc.want {
				t.Fatalf("ShouldRespond() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestExtractSignals(t *testing.T) {
	t.Parallel()

	botUser := id.UserID("@bot:example.org")
	cfg := RoutingConfig{BotUserID: botUser, CommandPrefix: "!"}

	t.Run("mentions and command", func(t *testing.T) {
		evt := testTextEvent("@alice:example.org !echo hi", &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    "!echo hi",
			Mentions: &event.Mentions{
				UserIDs: []id.UserID{botUser, "@alice:example.org"},
			},
		})

		s := ExtractSignals(cfg, evt, false)
		if !s.IsText || !s.StartsWithCommand || !s.MentionsBot || !s.MentionsOtherUsers {
			t.Fatalf("unexpected signals: %+v", s)
		}
		if s.DirectedToOtherUser {
			t.Fatalf("expected not directed to other user when bot is also mentioned: %+v", s)
		}
	})

	t.Run("leading body target fallback", func(t *testing.T) {
		evt := testTextEvent("@alice:example.org can you check this", &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    "@alice:example.org can you check this",
		})
		s := ExtractSignals(cfg, evt, false)
		if !s.DirectedToOtherUser {
			t.Fatalf("expected directed to other user: %+v", s)
		}
	})

	t.Run("leading body target for bot marks mention", func(t *testing.T) {
		evt := testTextEvent("@alice:example.org", &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    "@bot can you help?",
		})
		s := ExtractSignals(cfg, evt, false)
		if !s.MentionsBot {
			t.Fatalf("expected bot mention via fallback target: %+v", s)
		}
		if s.DirectedToOtherUser {
			t.Fatalf("unexpected directed-to-other when addressing bot: %+v", s)
		}
	})

	t.Run("reply and thread ids", func(t *testing.T) {
		evt := testTextEvent("reply", &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    "reply",
			RelatesTo: (&event.RelatesTo{}).
				SetReplyTo("$reply-parent").
				SetThread("$thread-parent", "$thread-fallback"),
		})
		s := ExtractSignals(cfg, evt, false)
		if s.ReplyToEventID != "$reply-parent" {
			t.Fatalf("reply id = %q, want %q", s.ReplyToEventID, "$reply-parent")
		}
		if s.ThreadParentEventID != "$thread-parent" {
			t.Fatalf("thread parent = %q, want %q", s.ThreadParentEventID, "$thread-parent")
		}
	})
}

func TestEvaluateRouting_UsesLookup(t *testing.T) {
	t.Parallel()

	cfg := RoutingConfig{BotUserID: "@bot:example.org", CommandPrefix: "!"}
	lookup := &testLookup{senders: map[id.EventID]id.UserID{
		"$reply-parent":  "@bot:example.org",
		"$thread-parent": "@bot:example.org",
	}}
	evt := testTextEvent("follow up", &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    "follow up",
		RelatesTo: (&event.RelatesTo{}).
			SetReplyTo("$reply-parent").
			SetThread("$thread-parent", ""),
	})

	decision, signals := EvaluateRouting(context.Background(), cfg, evt, false, lookup)
	if !signals.ReplyTargetsBot || !signals.ThreadParentIsBot {
		t.Fatalf("expected lookup-derived bot targets, got %+v", signals)
	}
	if !decision.Respond || decision.Reason != "reply_to_bot" {
		t.Fatalf("decision = %+v, want reply_to_bot", decision)
	}
}

func TestLooksLikeBotMention(t *testing.T) {
	t.Parallel()

	if !looksLikeBotMention("@bot", "@bot:example.org") {
		t.Fatal("expected @bot shorthand to match")
	}
	if !looksLikeBotMention("@bot:example.org", "@bot:example.org") {
		t.Fatal("expected full user id to match")
	}
	if looksLikeBotMention("@alice", "@bot:example.org") {
		t.Fatal("unexpected non-bot mention match")
	}
}

type testLookup struct {
	senders map[id.EventID]id.UserID
}

func (l *testLookup) SenderForEvent(_ context.Context, _ id.RoomID, eventID id.EventID) (id.UserID, bool) {
	sender, ok := l.senders[eventID]
	return sender, ok
}

func testTextEvent(sender string, msg *event.MessageEventContent) *event.Event {
	return &event.Event{
		Sender: id.UserID(sender),
		RoomID: id.RoomID("!room:example.org"),
		ID:     id.EventID("$evt"),
		Type:   event.EventMessage,
		Content: event.Content{
			Parsed: msg,
		},
	}
}
