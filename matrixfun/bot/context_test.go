package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildPromptContext_PrioritizesRelevantMessages(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)
	current := TranscriptMessage{
		EventID:    "$current",
		Sender:     "@alice:example.org",
		Body:       "Can we decide the retry policy?",
		Timestamp:  now,
		ThreadRoot: "$thread-1",
		ReplyTo:    "$bot-msg",
	}

	candidates := []TranscriptMessage{
		{EventID: "$noise-1", Sender: "@bob:example.org", Body: "lol", Timestamp: now.Add(-5 * time.Minute)},
		{EventID: "$bot-msg", Sender: "@bot:example.org", Body: "We should use exponential backoff", Timestamp: now.Add(-4 * time.Minute), ThreadRoot: "$thread-1", IsBot: true},
		{EventID: "$decision", Sender: "@alice:example.org", Body: "Decision: cap retries at 3", Timestamp: now.Add(-3 * time.Minute), ThreadRoot: "$thread-1"},
		{EventID: "$other-thread", Sender: "@carol:example.org", Body: "Question about vacation calendar", Timestamp: now.Add(-2 * time.Minute), ThreadRoot: "$thread-2"},
	}

	cfg := ContextConfig{MaxMessages: 3, TokenBudget: 80, SummaryMaxItems: 3}
	ctx := BuildPromptContext(cfg, current, candidates, "Earlier summary", []string{"User prefers concise answers"})

	if len(ctx.Included) != 3 {
		t.Fatalf("included len = %d, want 3", len(ctx.Included))
	}
	if ctx.Included[0].EventID != "$bot-msg" || ctx.Included[1].EventID != "$decision" {
		t.Fatalf("unexpected included ordering: %+v", ctx.Included)
	}
	if !strings.Contains(ctx.RollingSummary, "Earlier summary") {
		t.Fatalf("expected existing summary to be preserved: %q", ctx.RollingSummary)
	}
	if len(ctx.DurableMemory) != 1 || ctx.DurableMemory[0] != "User prefers concise answers" {
		t.Fatalf("unexpected durable memory: %+v", ctx.DurableMemory)
	}
}

func TestBuildPromptContext_RespectsTokenBudget(t *testing.T) {
	t.Parallel()

	current := TranscriptMessage{EventID: "$current", Body: "hello"}
	candidates := []TranscriptMessage{
		{EventID: "$a", Body: strings.Repeat("word ", 50), Sender: "@alice:example.org"},
		{EventID: "$b", Body: "Decision: keep short message", Sender: "@bob:example.org"},
	}

	cfg := ContextConfig{MaxMessages: 5, TokenBudget: 20, SummaryMaxItems: 2}
	ctx := BuildPromptContext(cfg, current, candidates, "", nil)

	if len(ctx.Included) != 1 {
		t.Fatalf("included len = %d, want 1", len(ctx.Included))
	}
	if ctx.Included[0].EventID != "$b" {
		t.Fatalf("expected smaller high-value message to survive, got %+v", ctx.Included)
	}
	if len(ctx.Elided) != 1 || ctx.Elided[0].EventID != "$a" {
		t.Fatalf("unexpected elided set: %+v", ctx.Elided)
	}
}

func TestSummarizeElided(t *testing.T) {
	t.Parallel()

	elided := []TranscriptMessage{
		{Sender: "@alice:example.org", Body: "lol"},
		{Sender: "@bob:example.org", Body: "Decision: use ambient mode only in test room"},
		{Sender: "@carol:example.org", Body: "How should we handle thread context?"},
	}

	summary := summarizeElided(elided, 2)
	if !strings.Contains(summary, "Elided context highlights") {
		t.Fatalf("summary missing heading: %q", summary)
	}
	if strings.Contains(summary, "lol") {
		t.Fatalf("summary should skip low-value chatter: %q", summary)
	}
	if !strings.Contains(summary, "Decision: use ambient mode") {
		t.Fatalf("summary missing high value line: %q", summary)
	}
}

func TestPruneAndSummarize(t *testing.T) {
	t.Parallel()

	current := TranscriptMessage{EventID: "$current", Body: "Need a plan", Sender: "@alice:example.org", ThreadRoot: "$t1"}
	history := []TranscriptMessage{
		{EventID: "$1", Body: "lol", Sender: "@bob:example.org", ThreadRoot: "$t2"},
		{EventID: "$2", Body: "Action item: wire reply detection", Sender: "@alice:example.org", ThreadRoot: "$t1"},
		{EventID: "$3", Body: "Question: should ambient mode default to false?", Sender: "@carol:example.org", ThreadRoot: "$t1"},
	}

	included, summary := PruneAndSummarize(ContextConfig{MaxMessages: 1, TokenBudget: 40, SummaryMaxItems: 2}, current, history, "")
	if len(included) != 1 || included[0].EventID != "$2" {
		t.Fatalf("included = %+v, want event $2", included)
	}
	if summary == "" {
		t.Fatal("expected summary to include elided highlights")
	}
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()

	if got := estimateTokens(""); got != 1 {
		t.Fatalf("estimateTokens(empty) = %d, want 1", got)
	}
	if got := estimateTokens("one two three"); got != 7 {
		t.Fatalf("estimateTokens = %d, want 7", got)
	}
}
