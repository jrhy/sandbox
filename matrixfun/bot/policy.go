package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"maunium.net/go/mautrix/id"
)

type RoomPolicy struct {
	CustomSystemPrompt    string    `json:"custom_system_prompt,omitempty"`
	CustomElisionCriteria string    `json:"custom_elision_criteria,omitempty"`
	ElisionAggressiveness string    `json:"elision_aggressiveness,omitempty"`
	KeepKeywords          []string  `json:"keep_keywords,omitempty"`
	DropKeywords          []string  `json:"drop_keywords,omitempty"`
	UpdatedBy             string    `json:"updated_by,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
}

func normalizeRoomPolicy(p RoomPolicy) RoomPolicy {
	p.CustomSystemPrompt = strings.TrimSpace(p.CustomSystemPrompt)
	p.CustomElisionCriteria = strings.TrimSpace(p.CustomElisionCriteria)
	p.ElisionAggressiveness = normalizeElisionAggressiveness(p.ElisionAggressiveness)
	p.KeepKeywords = normalizeKeywordList(p.KeepKeywords)
	p.DropKeywords = normalizeKeywordList(p.DropKeywords)
	p.UpdatedBy = strings.TrimSpace(p.UpdatedBy)
	return p
}

func normalizeElisionAggressiveness(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func normalizeKeywordList(items []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		norm := strings.ToLower(strings.TrimSpace(item))
		if norm == "" {
			continue
		}
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		out = append(out, norm)
	}
	sort.Strings(out)
	return out
}

func parseKeywordCSV(v string) []string {
	parts := strings.Split(v, ",")
	return normalizeKeywordList(parts)
}

func formatRoomPolicy(p RoomPolicy) string {
	p = normalizeRoomPolicy(p)
	summary := []string{"Room policy:"}
	if p.CustomSystemPrompt == "" {
		summary = append(summary, "- custom_system_prompt: (empty)")
	} else {
		summary = append(summary, "- custom_system_prompt: set")
	}
	if p.CustomElisionCriteria == "" {
		summary = append(summary, "- custom_elision_criteria: (empty)")
	} else {
		summary = append(summary, "- custom_elision_criteria: set")
	}
	if p.ElisionAggressiveness == "" {
		summary = append(summary, "- elision_aggressiveness: (default)")
	} else {
		summary = append(summary, "- elision_aggressiveness: "+p.ElisionAggressiveness)
	}
	if len(p.KeepKeywords) == 0 {
		summary = append(summary, "- keep_keywords: (empty)")
	} else {
		summary = append(summary, "- keep_keywords: "+strings.Join(p.KeepKeywords, ", "))
	}
	if len(p.DropKeywords) == 0 {
		summary = append(summary, "- drop_keywords: (empty)")
	} else {
		summary = append(summary, "- drop_keywords: "+strings.Join(p.DropKeywords, ", "))
	}
	if p.UpdatedBy != "" {
		summary = append(summary, "- updated_by: "+p.UpdatedBy)
	}
	if !p.UpdatedAt.IsZero() {
		summary = append(summary, "- updated_at: "+p.UpdatedAt.UTC().Format(time.RFC3339))
	}
	return strings.Join(summary, "\n")
}

func composeSystemPrompt(basePrompt string, policy RoomPolicy) string {
	policy = normalizeRoomPolicy(policy)
	basePrompt = strings.TrimSpace(basePrompt)
	parts := []string{basePrompt}
	if policy.CustomSystemPrompt != "" {
		parts = append(parts, "Room custom prompt:\n"+policy.CustomSystemPrompt)
	}
	if policy.CustomElisionCriteria != "" {
		parts = append(parts, "Room custom elision criteria:\n"+policy.CustomElisionCriteria)
	}
	if policy.ElisionAggressiveness != "" {
		parts = append(parts, "Room elision aggressiveness: "+policy.ElisionAggressiveness)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func applyPolicyToContextConfig(cfg ContextConfig, policy RoomPolicy) ContextConfig {
	out := cfg
	policy = normalizeRoomPolicy(policy)
	out.KeepKeywords = append([]string(nil), policy.KeepKeywords...)
	out.DropKeywords = append([]string(nil), policy.DropKeywords...)

	switch policy.ElisionAggressiveness {
	case "high":
		out.MaxMessages = maxInt(8, int(float64(out.MaxMessages)*0.7))
		out.TokenBudget = maxInt(350, int(float64(out.TokenBudget)*0.7))
		out.SummaryMaxItems = maxInt(3, int(float64(out.SummaryMaxItems)*0.7))
	case "low":
		out.MaxMessages = int(float64(out.MaxMessages) * 1.2)
		out.TokenBudget = int(float64(out.TokenBudget) * 1.2)
		out.SummaryMaxItems = int(float64(out.SummaryMaxItems) * 1.2)
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func handlePolicyCommand(ctx context.Context, sender id.UserID, roomID id.RoomID, body string, store *RoomMemoryStore, cfg LLMConfig, ollama OllamaClient, trusted map[id.UserID]bool) (string, bool, error) {
	body = strings.TrimSpace(body)
	if !strings.HasPrefix(body, "!policy") {
		return "", false, nil
	}
	if store == nil {
		return "policy store not configured", true, nil
	}

	mem, err := store.Load(roomID)
	if err != nil {
		return "", true, fmt.Errorf("load room memory: %w", err)
	}
	policy := normalizeRoomPolicy(mem.Policy)
	isTrusted := trusted[sender]

	switch {
	case body == "!policy", body == "!policy show":
		return formatRoomPolicy(policy), true, nil
	case body == "!policy reset":
		policy = RoomPolicy{UpdatedBy: string(sender), UpdatedAt: time.Now().UTC()}
		mem.Policy = policy
		if err := store.Save(roomID, mem); err != nil {
			return "", true, fmt.Errorf("save room policy: %w", err)
		}
		return "Room policy reset.", true, nil
	case strings.HasPrefix(body, "!policy set "):
		keyAndValue := strings.TrimSpace(strings.TrimPrefix(body, "!policy set "))
		parts := strings.SplitN(keyAndValue, " ", 2)
		if len(parts) < 2 {
			return "Usage: !policy set <field> <value>", true, nil
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		switch key {
		case "elision":
			aggr := normalizeElisionAggressiveness(value)
			if aggr == "" {
				return "elision must be one of: low, medium, high", true, nil
			}
			policy.ElisionAggressiveness = aggr
		case "keep":
			policy.KeepKeywords = parseKeywordCSV(value)
		case "drop":
			policy.DropKeywords = parseKeywordCSV(value)
		case "prompt":
			if !isTrusted {
				return "Only trusted members can set raw prompt text.", true, nil
			}
			policy.CustomSystemPrompt = value
		case "criteria":
			if !isTrusted {
				return "Only trusted members can set raw elision criteria.", true, nil
			}
			policy.CustomElisionCriteria = value
		default:
			return "Unknown field. Supported: elision, keep, drop, prompt, criteria", true, nil
		}
		policy.UpdatedBy = string(sender)
		policy.UpdatedAt = time.Now().UTC()
		mem.Policy = normalizeRoomPolicy(policy)
		if err := store.Save(roomID, mem); err != nil {
			return "", true, fmt.Errorf("save room policy: %w", err)
		}
		return formatRoomPolicy(mem.Policy), true, nil
	case strings.HasPrefix(body, "!policy edit "):
		if !isTrusted {
			return "Only trusted members can ask the model to edit raw policy text.", true, nil
		}
		instruction := strings.TrimSpace(strings.TrimPrefix(body, "!policy edit "))
		if instruction == "" {
			return "Usage: !policy edit <natural-language instruction>", true, nil
		}
		updated, err := suggestPolicyEdit(ctx, cfg.Model, ollama, policy, instruction)
		if err != nil {
			return "", true, fmt.Errorf("policy edit failed: %w", err)
		}
		updated.UpdatedBy = string(sender)
		updated.UpdatedAt = time.Now().UTC()
		mem.Policy = normalizeRoomPolicy(updated)
		if err := store.Save(roomID, mem); err != nil {
			return "", true, fmt.Errorf("save room policy: %w", err)
		}
		return "Policy updated via model edit.\n" + formatRoomPolicy(mem.Policy), true, nil
	default:
		return "Unknown policy command. Use: !policy show | !policy set ... | !policy edit ... | !policy reset", true, nil
	}
}

type policyEditResponse struct {
	CustomSystemPrompt    string   `json:"custom_system_prompt"`
	CustomElisionCriteria string   `json:"custom_elision_criteria"`
	ElisionAggressiveness string   `json:"elision_aggressiveness"`
	KeepKeywords          []string `json:"keep_keywords"`
	DropKeywords          []string `json:"drop_keywords"`
}

func suggestPolicyEdit(ctx context.Context, model string, client OllamaClient, current RoomPolicy, instruction string) (RoomPolicy, error) {
	if client == nil {
		return RoomPolicy{}, fmt.Errorf("nil ollama client")
	}
	if strings.TrimSpace(model) == "" {
		return RoomPolicy{}, fmt.Errorf("missing model")
	}
	current = normalizeRoomPolicy(current)
	currentJSON, _ := json.Marshal(current)

	system := `You edit room policy JSON for a chat bot.
Return ONLY valid JSON with keys:
custom_system_prompt, custom_elision_criteria, elision_aggressiveness, keep_keywords, drop_keywords.
Do not include markdown.
Keep unchanged fields from current policy unless instruction requests changes.
elision_aggressiveness must be one of: low, medium, high, or empty string.`
	user := fmt.Sprintf("Current policy JSON:\n%s\n\nInstruction:\n%s", string(currentJSON), instruction)

	resp, err := client.Chat(ctx, OllamaChatRequest{
		Model: model,
		Messages: []ChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Stream: false,
	})
	if err != nil {
		return RoomPolicy{}, err
	}

	raw := strings.TrimSpace(resp.Message.Content)
	if raw == "" {
		return RoomPolicy{}, fmt.Errorf("empty policy edit response")
	}
	if start := strings.Index(raw, "{"); start >= 0 {
		if end := strings.LastIndex(raw, "}"); end > start {
			raw = raw[start : end+1]
		}
	}

	var parsed policyEditResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return RoomPolicy{}, fmt.Errorf("parse policy edit json: %w", err)
	}

	updated := RoomPolicy{
		CustomSystemPrompt:    parsed.CustomSystemPrompt,
		CustomElisionCriteria: parsed.CustomElisionCriteria,
		ElisionAggressiveness: parsed.ElisionAggressiveness,
		KeepKeywords:          parsed.KeepKeywords,
		DropKeywords:          parsed.DropKeywords,
	}
	updated = normalizeRoomPolicy(updated)
	if updated.CustomSystemPrompt == "" && updated.CustomElisionCriteria == "" && updated.ElisionAggressiveness == "" && len(updated.KeepKeywords) == 0 && len(updated.DropKeywords) == 0 {
		return RoomPolicy{}, fmt.Errorf("policy edit produced empty policy")
	}
	return updated, nil
}
