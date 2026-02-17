package main

import (
	"sort"
	"strings"
	"time"
)

type TranscriptMessage struct {
	EventID     string
	RoomID      string
	Sender      string
	Body        string
	Timestamp   time.Time
	ReplyTo     string
	ThreadRoot  string
	MentionsBot bool
	IsBot       bool
}

type ContextConfig struct {
	MaxMessages     int
	TokenBudget     int
	SummaryMaxItems int
}

type PromptContext struct {
	Current        TranscriptMessage
	Included       []TranscriptMessage
	Elided         []TranscriptMessage
	RollingSummary string
	DurableMemory  []string
}

type scoredMessage struct {
	msg   TranscriptMessage
	score int
	idx   int
}

func BuildPromptContext(cfg ContextConfig, current TranscriptMessage, candidates []TranscriptMessage, rollingSummary string, durableMemory []string) PromptContext {
	cfg = normalizeContextConfig(cfg)
	scored := make([]scoredMessage, 0, len(candidates))
	for i, msg := range candidates {
		scored = append(scored, scoredMessage{msg: msg, score: scoreMessage(current, msg), idx: i})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if !scored[i].msg.Timestamp.Equal(scored[j].msg.Timestamp) {
			return scored[i].msg.Timestamp.After(scored[j].msg.Timestamp)
		}
		return scored[i].idx < scored[j].idx
	})

	included := make([]TranscriptMessage, 0, cfg.MaxMessages)
	elided := make([]TranscriptMessage, 0, len(scored))
	tokensUsed := estimateTokens(current.Body)

	for _, item := range scored {
		if len(included) >= cfg.MaxMessages {
			elided = append(elided, item.msg)
			continue
		}
		msgTokens := estimateTokens(item.msg.Body)
		if tokensUsed+msgTokens > cfg.TokenBudget {
			elided = append(elided, item.msg)
			continue
		}
		included = append(included, item.msg)
		tokensUsed += msgTokens
	}

	sort.SliceStable(included, func(i, j int) bool {
		if included[i].Timestamp.Equal(included[j].Timestamp) {
			return included[i].EventID < included[j].EventID
		}
		return included[i].Timestamp.Before(included[j].Timestamp)
	})

	summary := rollingSummary
	elision := summarizeElided(elided, cfg.SummaryMaxItems)
	if elision != "" {
		if summary == "" {
			summary = elision
		} else {
			summary = summary + "\n" + elision
		}
	}

	return PromptContext{
		Current:        current,
		Included:       included,
		Elided:         elided,
		RollingSummary: summary,
		DurableMemory:  append([]string(nil), durableMemory...),
	}
}

func normalizeContextConfig(cfg ContextConfig) ContextConfig {
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = 24
	}
	if cfg.TokenBudget <= 0 {
		cfg.TokenBudget = 1400
	}
	if cfg.SummaryMaxItems <= 0 {
		cfg.SummaryMaxItems = 6
	}
	return cfg
}

func scoreMessage(current, candidate TranscriptMessage) int {
	score := 0
	if candidate.ThreadRoot != "" && current.ThreadRoot != "" && candidate.ThreadRoot == current.ThreadRoot {
		score += 5
	}
	if candidate.EventID == current.ReplyTo || candidate.ReplyTo == current.EventID {
		score += 5
	}
	if candidate.Sender != "" && candidate.Sender == current.Sender {
		score += 3
	}
	if candidate.MentionsBot || candidate.IsBot {
		score += 3
	}
	if hasHighValueContent(candidate.Body) {
		score += 2
	}
	if isLowValueChatter(candidate.Body) {
		score -= 3
	}
	if candidate.ThreadRoot != "" && current.ThreadRoot != "" && candidate.ThreadRoot != current.ThreadRoot {
		score -= 4
	}
	return score
}

func hasHighValueContent(body string) bool {
	text := strings.ToLower(strings.TrimSpace(body))
	if text == "" {
		return false
	}
	keywords := []string{
		"decision", "decide", "action", "todo", "task", "ship", "constraint", "must", "deadline", "plan", "question", "why", "how",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return strings.HasSuffix(text, "?")
}

func isLowValueChatter(body string) bool {
	text := strings.ToLower(strings.TrimSpace(body))
	if text == "" {
		return true
	}
	lowValue := map[string]struct{}{
		"ok": {}, "okay": {}, "k": {}, "thanks": {}, "thx": {}, "lol": {}, "lmao": {}, "nice": {}, "cool": {}, "sgtm": {}, "sounds good": {}, "👍": {},
	}
	_, ok := lowValue[text]
	return ok
}

func summarizeElided(elided []TranscriptMessage, maxItems int) string {
	if maxItems <= 0 {
		return ""
	}
	items := make([]string, 0, maxItems)
	for _, msg := range elided {
		if len(items) >= maxItems {
			break
		}
		if !hasHighValueContent(msg.Body) {
			continue
		}
		items = append(items, compactSummaryLine(msg))
	}
	if len(items) == 0 {
		return ""
	}
	return "Elided context highlights:\n- " + strings.Join(items, "\n- ")
}

func compactSummaryLine(msg TranscriptMessage) string {
	text := strings.TrimSpace(msg.Body)
	if len(text) > 100 {
		text = text[:100] + "..."
	}
	if msg.Sender == "" {
		return text
	}
	return msg.Sender + ": " + text
}

func estimateTokens(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return 1
	}
	return len(fields) + 4
}
