// Package tinylfu is an implementation of the TinyLFU caching algorithm
/*
   http://arxiv.org/abs/1512.00727
*/
package tinylfu

import (
	"container/list"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

func NewItem[V any](key string, value V) *Item[V] {
	return &Item[V]{Key: key, Value: value}
}

func NewItemExpire[V any](key string, value V, expireAt time.Time) *Item[V] {
	return &Item[V]{Key: key, Value: value, ExpireAt: expireAt}
}

type Item[V any] struct {
	Key      string
	Value    V
	ExpireAt time.Time
	OnEvict  func()

	listid int
	keyh   uint64
}

func (self *Item[V]) WithOnEvict(fn func()) *Item[V] {
	self.OnEvict = fn
	return self
}

func (self *Item[V]) expired() bool {
	return !self.ExpireAt.IsZero() && time.Now().After(self.ExpireAt)
}

type T[V any] struct {
	w       int
	samples int

	countSketch *cm4
	bouncer     *doorkeeper

	data map[string]*list.Element

	lru  *lruCache[V]
	slru *slruCache[V]
}

func New[V any](size, samples int) *T[V] {
	const lruPct = 1

	lruSize := max(1, (lruPct*size)/100)
	slruSize := max(1, size-lruSize)
	slru20 := max(1, slruSize/5)

	data := make(map[string]*list.Element, size)

	return &T[V]{
		w:       0,
		samples: samples,

		countSketch: newCM4(size),
		bouncer:     newDoorkeeper(samples, 0.01),

		data: data,

		lru:  newLRU[V](lruSize, data),
		slru: newSLRU[V](slru20, slruSize-slru20, data),
	}
}

func (t *T[V]) onEvict(item *Item[V]) {
	if item.OnEvict != nil {
		item.OnEvict()
	}
}

func (t *T[V]) Get(key string) (value V, exists bool) {
	t.w++
	if t.w == t.samples {
		t.countSketch.reset()
		t.bouncer.reset()
		t.w = 0
	}

	keyh := xxhash.Sum64String(key)
	t.countSketch.add(keyh)

	val, ok := t.data[key]
	if !ok {
		return value, exists
	}

	item := val.Value.(*Item[V])
	if item.expired() {
		t.del(val)
		return value, exists
	}

	// Save the value since it is overwritten below.
	value, exists = item.Value, true
	if item.listid == 0 {
		t.lru.get(val)
	} else {
		t.slru.get(val)
	}
	return value, exists
}

func (t *T[V]) Set(newItem *Item[V]) {
	if e, ok := t.data[newItem.Key]; ok {
		// Key is already in our cache.
		// `Set` will act as a `Get` for list movements
		item := e.Value.(*Item[V])
		item.Value = newItem.Value
		t.countSketch.add(item.keyh)

		if item.listid == 0 {
			t.lru.get(e)
		} else {
			t.slru.get(e)
		}
		return
	}

	newItem.keyh = xxhash.Sum64String(newItem.Key)

	oldItem, evicted := t.lru.add(newItem)
	if !evicted {
		return
	}

	// estimate count of what will be evicted from slru
	victim := t.slru.victim()
	if victim == nil {
		t.slru.add(oldItem)
		return
	}

	if !t.bouncer.allow(oldItem.keyh) {
		t.onEvict(oldItem)
		return
	}

	victimCount := t.countSketch.estimate(victim.keyh)
	itemCount := t.countSketch.estimate(oldItem.keyh)

	if itemCount > victimCount {
		t.slru.add(oldItem)
	} else {
		t.onEvict(oldItem)
	}
}

func (t *T[V]) Del(key string) {
	if val, ok := t.data[key]; ok {
		t.del(val)
	}
}

func (t *T[V]) del(val *list.Element) {
	item := val.Value.(*Item[V])
	delete(t.data, item.Key)

	if item.listid == 0 {
		t.lru.Remove(val)
	} else {
		t.slru.Remove(val)
	}

	t.onEvict(item)
}

//------------------------------------------------------------------------------

type SyncT[V any] struct {
	mu sync.Mutex
	t  *T[V]
}

func NewSync[V any](size, samples int) *SyncT[V] {
	return &SyncT[V]{t: New[V](size, samples)}
}

func (t *SyncT[V]) Get(key string) (V, bool) {
	t.mu.Lock()
	val, ok := t.t.Get(key)
	t.mu.Unlock()
	return val, ok
}

func (t *SyncT[V]) Set(item *Item[V]) {
	t.mu.Lock()
	t.t.Set(item)
	t.mu.Unlock()
}

func (t *SyncT[V]) Del(key string) {
	t.mu.Lock()
	t.t.Del(key)
	t.mu.Unlock()
}
