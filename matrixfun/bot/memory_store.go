package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"maunium.net/go/mautrix/id"
)

type RoomMemory struct {
	RoomID         string    `json:"room_id"`
	RollingSummary string    `json:"rolling_summary,omitempty"`
	DurableMemory  []string  `json:"durable_memory,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

type RoomMemoryStore struct {
	dir   string
	mu    sync.Mutex
	cache map[id.RoomID]RoomMemory
}

func NewRoomMemoryStore(dir string) (*RoomMemoryStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "data/memory"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create memory dir %q: %w", dir, err)
	}
	return &RoomMemoryStore{dir: dir, cache: make(map[id.RoomID]RoomMemory)}, nil
}

func (s *RoomMemoryStore) Load(roomID id.RoomID) (RoomMemory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cached, ok := s.cache[roomID]; ok {
		return cloneRoomMemory(cached), nil
	}

	path := s.roomPath(roomID)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			empty := RoomMemory{RoomID: string(roomID)}
			s.cache[roomID] = empty
			return cloneRoomMemory(empty), nil
		}
		return RoomMemory{}, fmt.Errorf("read room memory %q: %w", path, err)
	}

	var mem RoomMemory
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &mem); err != nil {
			return RoomMemory{}, fmt.Errorf("parse room memory %q: %w", path, err)
		}
	}
	if mem.RoomID == "" {
		mem.RoomID = string(roomID)
	}
	mem.DurableMemory = normalizeDurableMemory(mem.DurableMemory, 50)
	s.cache[roomID] = mem
	return cloneRoomMemory(mem), nil
}

func (s *RoomMemoryStore) Save(roomID id.RoomID, mem RoomMemory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mem.RoomID = string(roomID)
	mem.DurableMemory = normalizeDurableMemory(mem.DurableMemory, 50)
	mem.UpdatedAt = time.Now().UTC()

	raw, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal room memory for %s: %w", roomID, err)
	}
	raw = append(raw, '\n')

	path := s.roomPath(roomID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write temp room memory %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace room memory %q: %w", path, err)
	}

	s.cache[roomID] = mem
	return nil
}

func (s *RoomMemoryStore) roomPath(roomID id.RoomID) string {
	name := url.PathEscape(string(roomID))
	if name == "" {
		name = "unknown-room"
	}
	return filepath.Join(s.dir, name+".json")
}

func cloneRoomMemory(mem RoomMemory) RoomMemory {
	mem.DurableMemory = append([]string(nil), mem.DurableMemory...)
	return mem
}

func MergeDurableMemory(existing []string, candidates []string, maxItems int) []string {
	if maxItems <= 0 {
		maxItems = 50
	}
	merged := append(append([]string(nil), existing...), candidates...)
	return normalizeDurableMemory(merged, maxItems)
}

func ExtractDurableMemoryCandidates(messages ...string) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		for _, line := range splitSentences(msg) {
			if isDurableCandidate(line) {
				out = append(out, line)
			}
		}
	}
	return normalizeDurableMemory(out, 25)
}

func splitSentences(text string) []string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return nil
	}
	parts := strings.FieldsFunc(clean, func(r rune) bool {
		return r == '\n' || r == '.' || r == '!' || r == '?'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.TrimSpace(part)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func isDurableCandidate(line string) bool {
	text := strings.ToLower(strings.TrimSpace(line))
	if text == "" {
		return false
	}
	keywords := []string{
		"i prefer", "prefer ", "must ", "constraint", "deadline", "action item", "todo", "remember", "always", "never",
	}
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func normalizeDurableMemory(items []string, maxItems int) []string {
	if maxItems <= 0 {
		maxItems = 50
	}
	seen := make(map[string]string)
	order := make([]string, 0, len(items))
	for _, item := range items {
		norm := normalizeMemoryLine(item)
		if norm == "" {
			continue
		}
		key := strings.ToLower(norm)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = norm
		order = append(order, key)
	}
	if len(order) > maxItems {
		order = order[len(order)-maxItems:]
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, seen[key])
	}
	sort.Strings(out)
	return out
}

func normalizeMemoryLine(item string) string {
	line := strings.TrimSpace(item)
	if line == "" {
		return ""
	}
	line = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
	line = strings.Join(strings.Fields(line), " ")
	if len(line) > 180 {
		line = line[:180]
	}
	return strings.TrimSpace(line)
}
