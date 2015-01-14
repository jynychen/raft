package raft

import (
	"sync"
)

// LogCache wraps a logstore with a ring buffer providing fast access to the
// last n raft log entries.
type LogCache struct {
	store      LogStore
	cache      []*Log
	current    int
	lastlogidx uint64
	l          sync.RWMutex
}

func NewLogCache(capacity int, logstore LogStore) *LogCache {
	return &LogCache{
		cache: make([]*Log, capacity),
		store: logstore,
	}
}

func (c *LogCache) getLogFromCache(logidx uint64) (*Log, bool) {
	c.l.RLock()
	defer c.l.RUnlock()

	// 'last' is the index of the element we cached last,
	// its raft log index is 'lastlogidx'
	last := (c.current - 1)
	m := last - int(c.lastlogidx-logidx)

	// See https://golang.org/issue/448 for why (m % n) is not enough.
	n := len(c.cache)
	log := c.cache[((m%n)+n)%n]
	if log == nil {
		return nil, false
	}
	// If the index does not match, cacheLog’s expected access pattern was
	// violated and we need to fall back to reading from the LogStore.
	return log, log.Index == logidx
}

// cacheLogs should be called with strictly monotonically increasing logidx
// values, otherwise the cache will not be effective.
func (c *LogCache) cacheLogs(logs []*Log) {
	c.l.Lock()
	defer c.l.Unlock()

	for _, l := range logs {
		c.cache[c.current] = l
		c.lastlogidx = l.Index
		c.current = (c.current + 1) % len(c.cache)
	}
}

func (c *LogCache) GetLog(logidx uint64, log *Log) error {
	if cached, ok := c.getLogFromCache(logidx); ok {
		*log = *cached
		return nil
	}
	return c.store.GetLog(logidx, log)
}

func (c *LogCache) StoreLog(log *Log) error {
	return c.StoreLogs([]*Log{log})
}

func (c *LogCache) StoreLogs(logs []*Log) error {
	c.cacheLogs(logs)
	return c.store.StoreLogs(logs)
}

func (c *LogCache) FirstIndex() (uint64, error) {
	return c.store.FirstIndex()
}

func (c *LogCache) LastIndex() (uint64, error) {
	return c.store.LastIndex()
}

func (c *LogCache) DeleteRange(min, max uint64) error {
	c.l.Lock()
	defer c.l.Unlock()

	c.lastlogidx = 0
	c.current = 0
	c.cache = make([]*Log, len(c.cache))

	return c.store.DeleteRange(min, max)
}