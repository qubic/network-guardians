package cache

import (
	"sync"
	"time"
)

// Item represents a cached item with expiration
type Item struct {
	Value      interface{}
	Expiration int64
}

// Expired returns true if the item has expired
func (i Item) Expired() bool {
	if i.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > i.Expiration
}

// thread-safe in-memory cache with TTL support
type Cache struct {
	items map[string]Item
	mu    sync.RWMutex
	ttl   time.Duration
}

// New creates a new cache with the given default TTL
func New(ttl time.Duration) *Cache {
	c := &Cache{
		items: make(map[string]Item),
		ttl:   ttl,
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.ttl)
}

func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	var expiration int64
	if ttl > 0 {
		expiration = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	c.items[key] = Item{
		Value:      value,
		Expiration: expiration,
	}
	c.mu.Unlock()
}

func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	item, found := c.items[key]
	c.mu.RUnlock()

	if !found {
		return nil, false
	}

	if item.Expired() {
		c.Delete(key)
		return nil, false
	}

	return item.Value, true
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]Item)
	c.mu.Unlock()
}

// cleanupLoop periodically removes expired items
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.deleteExpired()
	}
}

// deleteExpired removes all expired items
func (c *Cache) deleteExpired() {
	now := time.Now().UnixNano()

	c.mu.Lock()
	for key, item := range c.items {
		if item.Expiration > 0 && now > item.Expiration {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}
