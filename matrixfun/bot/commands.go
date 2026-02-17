package main

import (
	"fmt"
	"strings"

	"maunium.net/go/mautrix/id"
)

func handleCommand(body string, roomID id.RoomID, memoryStore *RoomMemoryStore, conversation *ConversationStore) (reply string, handled bool) {
	body = strings.TrimSpace(body)

	switch {
	case body == "!ping":
		return "pong", true
	case strings.HasPrefix(body, "!echo "):
		return strings.TrimSpace(strings.TrimPrefix(body, "!echo ")), true
	case body == "!help":
		return "Commands: !ping, !echo <text>, !help, !memory, !memory forget <text>, !memory clear, !policy ...", true
	case body == "!memory":
		if memoryStore == nil {
			return "Memory store not configured.", true
		}
		mem, err := memoryStore.Load(roomID)
		if err != nil {
			return fmt.Sprintf("Failed to load memory: %v", err), true
		}
		return formatRoomMemory(mem), true
	case body == "!memory clear":
		if memoryStore == nil {
			return "Memory store not configured.", true
		}
		cleared := RoomMemory{RoomID: string(roomID)}
		if err := memoryStore.Save(roomID, cleared); err != nil {
			return fmt.Sprintf("Failed to clear memory: %v", err), true
		}
		if conversation != nil {
			conversation.ClearRoom(roomID)
		}
		return "Memory cleared for this room.", true
	case strings.HasPrefix(body, "!memory forget "):
		if memoryStore == nil {
			return "Memory store not configured.", true
		}
		query := strings.TrimSpace(strings.TrimPrefix(body, "!memory forget "))
		if query == "" {
			return "Usage: !memory forget <text>", true
		}
		mem, err := memoryStore.Load(roomID)
		if err != nil {
			return fmt.Sprintf("Failed to load memory: %v", err), true
		}
		updated, removed := forgetDurableMemory(mem.DurableMemory, query)
		if removed == 0 {
			return "No durable memory entries matched that text.", true
		}
		mem.DurableMemory = updated
		if err := memoryStore.Save(roomID, mem); err != nil {
			return fmt.Sprintf("Failed to save memory: %v", err), true
		}
		return fmt.Sprintf("Removed %d durable memory entr%s.", removed, pluralSuffix(removed)), true
	default:
		return "", false
	}
}

func forgetDurableMemory(items []string, query string) ([]string, int) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]string(nil), items...), 0
	}
	out := make([]string, 0, len(items))
	removed := 0
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), query) {
			removed++
			continue
		}
		out = append(out, item)
	}
	return out, removed
}

func pluralSuffix(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func formatRoomMemory(mem RoomMemory) string {
	summary := strings.TrimSpace(mem.RollingSummary)
	if summary == "" {
		summary = "(empty)"
	}
	lines := []string{
		"Room memory:",
		"Summary: " + summary,
	}
	if len(mem.DurableMemory) == 0 {
		lines = append(lines, "Durable memory: (empty)")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Durable memory:")
	for i, item := range mem.DurableMemory {
		if i >= 10 {
			lines = append(lines, fmt.Sprintf("- ... (%d more)", len(mem.DurableMemory)-10))
			break
		}
		lines = append(lines, "- "+item)
	}
	return strings.Join(lines, "\n")
}
