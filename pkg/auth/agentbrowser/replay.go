package agentbrowser

import (
	"strings"
	"sync"
	"time"
)

type ReplayPolicy struct {
	MaxUses int
}

type ReplayCache interface {
	Use(jti string, expiresAt time.Time, policy ReplayPolicy) bool
}

type MemoryReplayCache struct {
	mu   sync.Mutex
	used map[string]replayEntry
	now  func() time.Time
}

type replayEntry struct {
	expiresAt time.Time
	uses      int
}

func NewMemoryReplayCache() *MemoryReplayCache {
	return &MemoryReplayCache{used: map[string]replayEntry{}, now: time.Now}
}

func (c *MemoryReplayCache) Use(jti string, expiresAt time.Time, policy ReplayPolicy) bool {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return false
	}
	if policy.MaxUses <= 0 {
		policy.MaxUses = 1
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for key, entry := range c.used {
		if !entry.expiresAt.After(now) {
			delete(c.used, key)
		}
	}

	entry := c.used[jti]
	if entry.uses >= policy.MaxUses {
		return false
	}
	entry.expiresAt = expiresAt
	entry.uses++
	c.used[jti] = entry
	return true
}
