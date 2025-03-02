package tinylfu

import "container/list"

// Cache is an LRU cache.  It is not safe for concurrent access.
type slruCache[V any] struct {
	data           map[string]*list.Element
	onecap, twocap int
	one, two       *list.List
}

func newSLRU[V any](onecap, twocap int, data map[string]*list.Element,
) *slruCache[V] {
	return &slruCache[V]{
		data:   data,
		onecap: onecap,
		one:    list.New(),
		twocap: twocap,
		two:    list.New(),
	}
}

// get updates the cache data structures for a get
func (slru *slruCache[V]) get(v *list.Element) {
	item := v.Value.(*Item[V])

	// already on list two?
	if item.listid == 2 {
		slru.two.MoveToFront(v)
		return
	}

	// must be list one

	// is there space on the next list?
	if slru.two.Len() < slru.twocap {
		// just do the remove/add
		slru.one.Remove(v)
		item.listid = 2
		slru.data[item.Key] = slru.two.PushFront(item)
		return
	}

	back := slru.two.Back()
	bitem := back.Value.(*Item[V])

	// swap the key/values
	*bitem, *item = *item, *bitem

	bitem.listid = 2
	item.listid = 1

	// update pointers in the map
	slru.data[item.Key] = v
	slru.data[bitem.Key] = back

	// move the elements to the front of their lists
	slru.one.MoveToFront(v)
	slru.two.MoveToFront(back)
}

// Set sets a value in the cache
func (slru *slruCache[V]) add(newItem *Item[V]) {
	newItem.listid = 1

	if slru.one.Len() < slru.onecap || (slru.Len() < slru.onecap+slru.twocap) {
		slru.data[newItem.Key] = slru.one.PushFront(newItem)
		return
	}

	// reuse the tail item
	e := slru.one.Back()
	item := e.Value.(*Item[V])

	delete(slru.data, item.Key)

	*item = *newItem

	slru.data[item.Key] = e
	slru.one.MoveToFront(e)
}

func (slru *slruCache[V]) victim() *Item[V] {
	if slru.Len() < slru.onecap+slru.twocap {
		return nil
	}

	v := slru.one.Back()

	return v.Value.(*Item[V])
}

// Len returns the total number of items in the cache
func (slru *slruCache[V]) Len() int { return slru.one.Len() + slru.two.Len() }

// Remove removes an item from the cache, returning the item and a boolean
// indicating if it was found.
func (slru *slruCache[V]) Remove(v *list.Element) {
	item := v.Value.(*Item[V])
	if item.listid == 2 {
		slru.two.Remove(v)
	} else {
		slru.one.Remove(v)
	}
}
