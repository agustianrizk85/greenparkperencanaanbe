package repository

import (
	"fmt"
	"sync"
)

// collection is a generic, concurrency-safe in-memory CRUD store for a slice of
// records identified by a string ID. IDs are accessed via the supplied
// getID/setID functions, so the record type itself need not implement any
// interface. New records receive an auto-generated "<prefix>-<n>" ID.
type collection[T any] struct {
	mu     sync.RWMutex
	items  []T
	prefix string
	seq    int
	getID  func(*T) string
	setID  func(*T, string)
}

func newCollection[T any](prefix string, items []T, getID func(*T) string, setID func(*T, string)) *collection[T] {
	return &collection[T]{items: items, prefix: prefix, seq: len(items), getID: getID, setID: setID}
}

// list returns a copy of all records (safe for the caller to read/serialise).
func (c *collection[T]) list() []T {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]T, len(c.items))
	copy(out, c.items)
	return out
}

func (c *collection[T]) get(id string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.items {
		v := c.items[i]
		if c.getID(&v) == id {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// create appends a record, assigning a generated ID when none is provided.
func (c *collection[T]) create(v T) T {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getID(&v) == "" {
		c.seq++
		c.setID(&v, fmt.Sprintf("%s-%d", c.prefix, c.seq))
	}
	c.items = append(c.items, v)
	return v
}

// update replaces the record with the given ID, preserving its ID.
func (c *collection[T]) update(id string, v T) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.items {
		existing := c.items[i]
		if c.getID(&existing) == id {
			c.setID(&v, id)
			c.items[i] = v
			return v, true
		}
	}
	var zero T
	return zero, false
}

func (c *collection[T]) delete(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.items {
		existing := c.items[i]
		if c.getID(&existing) == id {
			c.items = append(c.items[:i], c.items[i+1:]...)
			return true
		}
	}
	return false
}
