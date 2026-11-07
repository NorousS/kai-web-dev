// Package cache isolates application-level caching from HTTP controllers.
package cache

import (
	"sync"
	"time"
)

type entry struct {
	payload any
	expiry  time.Time
}
type Dashboard struct {
	mu      sync.Mutex
	entries map[uint]entry
	ttl     time.Duration
}

func NewDashboard(ttl time.Duration) *Dashboard {
	return &Dashboard{entries: make(map[uint]entry), ttl: ttl}
}
func (c *Dashboard) Get(id uint) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	return e.payload, ok && time.Now().Before(e.expiry)
}
func (c *Dashboard) Put(id uint, p any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = entry{p, time.Now().Add(c.ttl)}
}
func (c *Dashboard) Invalidate() { c.mu.Lock(); defer c.mu.Unlock(); c.entries = make(map[uint]entry) }