package cache2go

import (
	"container/list"
	"sync"
	"time"
)

// Cache is an LRU cache.
type Cache struct {
	sync.RWMutex
	// MaxEntries is the maximum number of cache entries before
	// an item is evicted. Zero means no limit.
	maxEntries int

	lruIndex   *list.List
	ttlIndex   []*list.Element
	cache      map[string]*list.Element
	expiration time.Duration
}

type entry struct {
	key       string
	value     interface{}
	timestamp time.Time
}

// New creates a new Cache.
// If maxEntries is zero, the cache has no limit and it's assumed
// that eviction is done by the caller.
func New(maxEntries int, expire time.Duration) *Cache {
	c := &Cache{
		maxEntries: maxEntries,
		expiration: expire,
		lruIndex:   list.New(),
		cache:      make(map[string]*list.Element),
	}
	if c.expiration > 0 {
		c.ttlIndex = make([]*list.Element, 0)
		go c.cleanExpired()
	}
	return c
}

// cleans expired entries performing minimal checks
func (c *Cache) cleanExpired() {
	for {
		c.RLock()
		if len(c.ttlIndex) == 0 {
			c.RUnlock()
			time.Sleep(c.expiration)
			continue
		}
		e := c.ttlIndex[0]

		en := e.Value.(*entry)
		exp := en.timestamp.Add(c.expiration)
		c.RUnlock()
		if time.Now().After(exp) {
			c.Lock()
			c.removeElement(e)
			c.Unlock()
		} else {
			time.Sleep(time.Now().Sub(exp))
		}
	}
}

// Add adds a value to the cache
func (c *Cache) Set(key string, value interface{}) {
	c.Lock()
	if c.cache == nil {
		c.cache = make(map[string]*list.Element)
		c.lruIndex = list.New()
		if c.expiration > 0 {
			c.ttlIndex = make([]*list.Element, 0)
		}
	}

	if e, ok := c.cache[key]; ok {
		c.lruIndex.MoveToFront(e)

		en := e.Value.(*entry)
		en.value = value
		en.timestamp = time.Now()

		c.Unlock()
		return
	}
	e := c.lruIndex.PushFront(&entry{key: key, value: value, timestamp: time.Now()})
	if c.expiration > 0 {
		c.ttlIndex = append(c.ttlIndex, e)
	}
	c.cache[key] = e

	if c.maxEntries != 0 && c.lruIndex.Len() > c.maxEntries {
		c.removeOldest()
	}
	c.Unlock()
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(key string) (value interface{}, ok bool) {
	c.Lock()
	defer c.Unlock()
	if c.cache == nil {
		return
	}
	if e, hit := c.cache[key]; hit {
		c.lruIndex.MoveToFront(e)
		return e.Value.(*entry).value, true
	}
	return
}

// Remove removes the provided key from the cache.
func (c *Cache) Delete(key string) {
	c.Lock()
	defer c.Unlock()
	if c.cache == nil {
		return
	}
	if e, hit := c.cache[key]; hit {
		c.removeElement(e)
	}
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache) removeOldest() {
	if c.cache == nil {
		return
	}
	e := c.lruIndex.Back()
	if e != nil {
		c.removeElement(e)
	}
}

func (c *Cache) removeElement(e *list.Element) {
	c.lruIndex.Remove(e)
	if c.expiration > 0 {
		for i, se := range c.ttlIndex {
			if se == e {
				//delete
				copy(c.ttlIndex[i:], c.ttlIndex[i+1:])
				c.ttlIndex[len(c.ttlIndex)-1] = nil
				c.ttlIndex = c.ttlIndex[:len(c.ttlIndex)-1]
				break
			}
		}
	}
	if e.Value != nil {
		kv := e.Value.(*entry)
		delete(c.cache, kv.key)
	}
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	c.RLock()
	defer c.RUnlock()
	if c.cache == nil {
		return 0
	}
	return c.lruIndex.Len()
}

// empties the whole cache
func (c *Cache) Flush() {
	c.Lock()
	defer c.Unlock()
	c.lruIndex = list.New()
	if c.expiration > 0 {
		c.ttlIndex = make([]*list.Element, 0)
	}
	c.cache = make(map[string]*list.Element)
}
