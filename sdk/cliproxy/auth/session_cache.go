package auth

import (
	"sync"
	"time"
)

// sessionEntry stores auth binding with expiration.
type sessionEntry struct {
	authID    string
	provider  string
	expiresAt time.Time
}

// SessionCache provides TTL-based session to auth mapping with automatic cleanup.
type SessionCache struct {
	mu      sync.RWMutex
	entries map[string]sessionEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

// NewSessionCache creates a cache with the specified TTL.
// A background goroutine periodically cleans expired entries.
func NewSessionCache(ttl time.Duration) *SessionCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	c := &SessionCache{
		entries: make(map[string]sessionEntry),
		ttl:     ttl,
		stopCh:  make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves the auth ID bound to a session, if still valid.
// Does NOT refresh the TTL on access.
func (c *SessionCache) Get(sessionID string) (string, bool) {
	entry, ok := c.GetEntry(sessionID)
	if !ok {
		return "", false
	}
	return entry.authID, true
}

// GetEntry retrieves the cached session entry, if still valid.
// Does NOT refresh the TTL on access.
func (c *SessionCache) GetEntry(sessionID string) (sessionEntry, bool) {
	if sessionID == "" {
		return sessionEntry{}, false
	}
	c.mu.RLock()
	entry, ok := c.entries[sessionID]
	c.mu.RUnlock()
	if !ok {
		return sessionEntry{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, sessionID)
		c.mu.Unlock()
		return sessionEntry{}, false
	}
	return entry, true
}

// GetAndRefresh retrieves the auth ID bound to a session and refreshes TTL on hit.
// This extends the binding lifetime for active sessions.
func (c *SessionCache) GetAndRefresh(sessionID string) (string, bool) {
	entry, ok := c.GetEntryAndRefresh(sessionID)
	if !ok {
		return "", false
	}
	return entry.authID, true
}

// GetEntryAndRefresh retrieves the cached session entry and refreshes TTL on hit.
// This extends the binding lifetime for active sessions.
func (c *SessionCache) GetEntryAndRefresh(sessionID string) (sessionEntry, bool) {
	if sessionID == "" {
		return sessionEntry{}, false
	}
	now := time.Now()
	c.mu.Lock()
	entry, ok := c.entries[sessionID]
	if !ok {
		c.mu.Unlock()
		return sessionEntry{}, false
	}
	if now.After(entry.expiresAt) {
		delete(c.entries, sessionID)
		c.mu.Unlock()
		return sessionEntry{}, false
	}
	// Refresh TTL on successful access
	entry.expiresAt = now.Add(c.ttl)
	c.entries[sessionID] = entry
	c.mu.Unlock()
	return entry, true
}

// Set binds a session to an auth ID with TTL refresh.
func (c *SessionCache) Set(sessionID, authID string) {
	c.SetWithProvider(sessionID, authID, "")
}

// SetWithProvider binds a session to an auth ID and provider with TTL refresh.
func (c *SessionCache) SetWithProvider(sessionID, authID, provider string) {
	if sessionID == "" || authID == "" {
		return
	}
	c.mu.Lock()
	c.entries[sessionID] = sessionEntry{
		authID:    authID,
		provider:  provider,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

func (c *SessionCache) snapshotEntriesForTransfer(destTTL time.Duration) map[string]sessionEntry {
	if c == nil {
		return nil
	}
	now := time.Now()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.entries) == 0 {
		return nil
	}
	snapshot := make(map[string]sessionEntry, len(c.entries))
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			continue
		}
		if destTTL > 0 {
			entry.expiresAt = now.Add(destTTL)
		}
		snapshot[key] = entry
	}
	return snapshot
}

// CopyFrom clones live session bindings from another cache into this cache.
func (c *SessionCache) CopyFrom(other *SessionCache) {
	if c == nil || other == nil || c == other {
		return
	}
	snapshot := other.snapshotEntriesForTransfer(c.ttl)
	if len(snapshot) == 0 {
		return
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]sessionEntry, len(snapshot))
	}
	for key, entry := range snapshot {
		c.entries[key] = entry
	}
	c.mu.Unlock()
}

// Invalidate removes a specific session binding.
func (c *SessionCache) Invalidate(sessionID string) {
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, sessionID)
	c.mu.Unlock()
}

// InvalidateAuth removes all sessions bound to a specific auth ID.
// Used when an auth becomes unavailable.
func (c *SessionCache) InvalidateAuth(authID string) {
	if authID == "" {
		return
	}
	c.mu.Lock()
	for sid, entry := range c.entries {
		if entry.authID == authID {
			delete(c.entries, sid)
		}
	}
	c.mu.Unlock()
}

// Stop terminates the background cleanup goroutine.
func (c *SessionCache) Stop() {
	select {
	case <-c.stopCh:
	default:
		close(c.stopCh)
	}
}

func (c *SessionCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

func (c *SessionCache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	for sid, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, sid)
		}
	}
	c.mu.Unlock()
}
