package tinylfu

import "container/list"

// Cache is an LRU cache.  It is not safe for concurrent access.
type lruCache[V any] struct {
	data map[string]*list.Element
	cap  int
	ll   *list.List
}

func newLRU[V any](capacity int, data map[string]*list.Element) *lruCache[V] {
	return &lruCache[V]{
		data: data,
		cap:  capacity,
		ll:   list.New(),
	}
}

// Get returns a value from the cache
func (lru *lruCache[V]) get(v *list.Element) { lru.ll.MoveToFront(v) }

// Set sets a value in the cache
func (lru *lruCache[V]) add(newItem *Item[V]) (*Item[V], bool) {
	if lru.ll.Len() < lru.cap {
		lru.data[newItem.Key] = lru.ll.PushFront(newItem)
		return nil, false
	}

	// reuse the tail item
	val := lru.ll.Back()
	item := val.Value.(*Item[V])

	delete(lru.data, item.Key)

	oldItem := *item
	*item = *newItem

	lru.data[item.Key] = val
	lru.ll.MoveToFront(val)

	return &oldItem, true
}

// Len returns the total number of items in the cache
func (lru *lruCache[V]) Len() int { return len(lru.data) }

// Remove removes an item from the cache, returning the item and a boolean
// indicating if it was found.
func (lru *lruCache[V]) Remove(v *list.Element) { lru.ll.Remove(v) }
