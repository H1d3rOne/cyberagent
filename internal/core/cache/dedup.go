package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

type Entry struct {
	Key       string
	Value     any
	CreatedAt time.Time
	TTL       time.Duration
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
	maxSize int
}

func New(maxSize int) *Cache {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &Cache{
		entries: make(map[string]Entry, maxSize),
		maxSize: maxSize,
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.TTL > 0 && time.Since(entry.CreatedAt) > entry.TTL {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return entry.Value, true
}

func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}
	c.entries[key] = Entry{Key: key, Value: value, CreatedAt: time.Now(), TTL: ttl}
}

func (c *Cache) Has(key string) bool {
	_, ok := c.Get(key)
	return ok
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]Entry, c.maxSize)
	c.mu.Unlock()
}

func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true
	for key, entry := range c.entries {
		if first || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
			first = false
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

type URLDedup struct {
	mu   sync.RWMutex
	seen map[string]bool
}

func NewURLDedup() *URLDedup {
	return &URLDedup{seen: make(map[string]bool)}
}

func (d *URLDedup) Seen(url string) bool {
	key := HashString(url)
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.seen[key]
}

func (d *URLDedup) Mark(url string) bool {
	key := HashString(url)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen[key] {
		return false
	}
	d.seen[key] = true
	return true
}

func (d *URLDedup) Count() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.seen)
}

func (d *URLDedup) Reset() {
	d.mu.Lock()
	d.seen = make(map[string]bool)
	d.mu.Unlock()
}

func HashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
