package main

import (
	"encoding/json"
	"os"
	"sync"
)

const maxRecent = 50

type recentEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// recentList is the most-recently-viewed stack behind the Ctrl-Tab switcher.
// Only user page views touch it; prefetches do not. It is persisted so the
// stack survives restarts.
type recentList struct {
	mu      sync.Mutex
	path    string
	entries []recentEntry
}

func loadRecent(path string) *recentList {
	r := &recentList{path: path}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &r.entries)
	}
	return r
}

// touch moves id to the front, updating its title.
func (r *recentList) touch(id, title string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recentEntry, 0, len(r.entries)+1)
	out = append(out, recentEntry{id, title})
	for _, e := range r.entries {
		if e.ID != id {
			out = append(out, e)
		}
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	r.entries = out
	if b, err := json.Marshal(out); err == nil {
		os.WriteFile(r.path, b, 0o600)
	}
}

func (r *recentList) list() []recentEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recentEntry(nil), r.entries...)
}
