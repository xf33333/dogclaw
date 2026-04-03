// Package sessionmemory provides session-level memory management.
// Translated from src/services/SessionMemory/
package sessionmemory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionMemoryEntry represents a session memory entry
type SessionMemoryEntry struct {
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// SessionMemory manages in-memory session state
type SessionMemory struct {
	mu      sync.RWMutex
	entries map[string]SessionMemoryEntry
	dir     string // persistent storage directory
}

// NewSessionMemory creates a new session memory manager
func NewSessionMemory(dir string) *SessionMemory {
	return &SessionMemory{
		entries: make(map[string]SessionMemoryEntry),
		dir:     dir,
	}
}

// Set stores a value in session memory
func (sm *SessionMemory) Set(key string, value any, ttl time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := SessionMemoryEntry{
		Key:       key,
		Value:     value,
		CreatedAt: time.Now(),
	}
	if ttl > 0 {
		entry.ExpiresAt = time.Now().Add(ttl)
	}

	sm.entries[key] = entry
}

// Get retrieves a value from session memory
func (sm *SessionMemory) Get(key string) (any, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	entry, ok := sm.entries[key]
	if !ok {
		return nil, false
	}

	// Check expiration
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Value, true
}

// Delete removes a value from session memory
func (sm *SessionMemory) Delete(key string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.entries, key)
}

// Clear removes all session memory entries
func (sm *SessionMemory) Clear() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.entries = make(map[string]SessionMemoryEntry)
}

// Keys returns all non-expired keys
func (sm *SessionMemory) Keys() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var keys []string
	now := time.Now()
	for k, entry := range sm.entries {
		if entry.ExpiresAt.IsZero() || now.Before(entry.ExpiresAt) {
			keys = append(keys, k)
		}
	}
	return keys
}

// Persist saves session memory to disk
func (sm *SessionMemory) Persist() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.dir == "" {
		return nil
	}

	if err := os.MkdirAll(sm.dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", sm.dir, err)
	}

	data, err := json.MarshalIndent(sm.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	return os.WriteFile(filepath.Join(sm.dir, "session.json"), data, 0644)
}

// Load restores session memory from disk
func (sm *SessionMemory) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.dir == "" {
		return nil
	}

	path := filepath.Join(sm.dir, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	return json.Unmarshal(data, &sm.entries)
}

// Compact removes expired entries from session memory
func (sm *SessionMemory) Compact() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	removed := 0
	for k, entry := range sm.entries {
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			delete(sm.entries, k)
			removed++
		}
	}
	return removed
}

// Query searches for entries by key prefix
func (sm *SessionMemory) Query(prefix string) []SessionMemoryEntry {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var results []SessionMemoryEntry
	now := time.Now()
	for k, entry := range sm.entries {
		if strings.HasPrefix(k, prefix) {
			if entry.ExpiresAt.IsZero() || now.Before(entry.ExpiresAt) {
				results = append(results, entry)
			}
		}
	}
	return results
}

// Len returns the number of non-expired entries
func (sm *SessionMemory) Len() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	count := 0
	now := time.Now()
	for _, entry := range sm.entries {
		if entry.ExpiresAt.IsZero() || now.Before(entry.ExpiresAt) {
			count++
		}
	}
	return count
}
