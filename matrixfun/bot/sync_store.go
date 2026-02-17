package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"maunium.net/go/mautrix/id"
)

type fileSyncStore struct {
	path string
	mu   sync.Mutex
	data syncStoreData
}

type syncStoreData struct {
	Users map[string]syncStoreUser `json:"users"`
}

type syncStoreUser struct {
	FilterID  string `json:"filter_id,omitempty"`
	NextBatch string `json:"next_batch,omitempty"`
}

func newFileSyncStore(path string) (*fileSyncStore, error) {
	store := &fileSyncStore{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *fileSyncStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = syncStoreData{Users: map[string]syncStoreUser{}}
			return nil
		}
		return fmt.Errorf("read sync store %q: %w", s.path, err)
	}

	if len(raw) == 0 {
		s.data = syncStoreData{Users: map[string]syncStoreUser{}}
		return nil
	}

	if err := json.Unmarshal(raw, &s.data); err != nil {
		return fmt.Errorf("parse sync store %q: %w", s.path, err)
	}
	if s.data.Users == nil {
		s.data.Users = map[string]syncStoreUser{}
	}
	return nil
}

func (s *fileSyncStore) SaveFilterID(_ context.Context, userID id.UserID, filterID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.data.Users[string(userID)]
	u.FilterID = filterID
	s.data.Users[string(userID)] = u
	return s.flushLocked()
}

func (s *fileSyncStore) LoadFilterID(_ context.Context, userID id.UserID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.data.Users[string(userID)].FilterID, nil
}

func (s *fileSyncStore) SaveNextBatch(_ context.Context, userID id.UserID, nextBatchToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	u := s.data.Users[string(userID)]
	u.NextBatch = nextBatchToken
	s.data.Users[string(userID)] = u
	return s.flushLocked()
}

func (s *fileSyncStore) LoadNextBatch(_ context.Context, userID id.UserID) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.data.Users[string(userID)].NextBatch, nil
}

func (s *fileSyncStore) flushLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create sync store dir for %q: %w", s.path, err)
	}

	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync store: %w", err)
	}
	raw = append(raw, '\n')

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write temp sync store %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace sync store %q: %w", s.path, err)
	}
	return nil
}
