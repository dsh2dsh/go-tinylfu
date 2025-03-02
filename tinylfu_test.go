package tinylfu_test

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dsh2dsh/go-tinylfu"
)

func TestCache(t *testing.T) {
	cache := tinylfu.New[string](1e3, 10e3)
	require.NotNil(t, cache)
	keys := []string{"one", "two", "three"}

	var evicted bool
	onEvict := func() { evicted = true }

	for _, key := range keys {
		item := tinylfu.NewItem(key, key).WithOnEvict(onEvict)
		cache.Set(item)
		require.False(t, evicted, "key: %q", key)

		got, ok := cache.Get(key)
		require.True(t, ok, "key: %q, got: %q", key, got)
		require.Equal(t, key, got)
	}

	for _, key := range keys {
		got, ok := cache.Get(key)
		require.True(t, ok, "key: %q, got: %q", key, got)
		require.Equal(t, key, got)

		cache.Set(tinylfu.NewItem(key, key+key))
	}

	for _, key := range keys {
		got, ok := cache.Get(key)
		require.True(t, ok, "key: %q, got: %q", key, got)
		require.Equal(t, key+key, got)
	}

	for _, key := range keys {
		cache.Del(key)
	}

	for _, key := range keys {
		_, ok := cache.Get(key)
		require.False(t, ok, "key: %q", key)
	}
}

func TestOOM(t *testing.T) {
	keys := make([]string, 10000)
	for i := range keys {
		keys[i] = randWord()
	}

	cache := tinylfu.New[string](1e3, 10e3)

	for i := range int(5e6) {
		key := keys[i%len(keys)]
		cache.Set(tinylfu.NewItem(key, key))
	}
}

func TestCorruptionOnExpiry(t *testing.T) {
	const size = 50000

	strFor := func(i int) string { return fmt.Sprintf("a string %d", i) }
	keyName := func(i int) string { return fmt.Sprintf("key-%05d", i) }

	mycache := tinylfu.New[[]byte](1000, 10000)
	// Put a bunch of stuff in the cache with a TTL of 1 second
	expireAt := time.Now().Add(time.Second)
	for i := range size {
		mycache.Set(tinylfu.NewItemExpire(keyName(i), []byte(strFor(i)), expireAt))
	}

	// Read stuff for a bit longer than the TTL - that's when the corruption
	// occurs.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

loop:
	for {
		select {
		case <-ctx.Done():
			// this is expected
			break loop
		default:
			i := rand.Intn(size) //nolint:gosec // weak random is OK for tests
			key := keyName(i)

			b, ok := mycache.Get(key)
			if !ok {
				continue loop
			}
			require.Equal(t, strFor(i), string(b), "key=%q", key)
		}
	}
}

func randWord() string {
	buf := make([]byte, 64)
	io.ReadFull(cryptorand.Reader, buf)
	return string(buf)
}

func TestAddAlreadyInCache(t *testing.T) {
	c := tinylfu.New[string](100, 10000)

	c.Set(tinylfu.NewItem("foo", "bar"))

	val, _ := c.Get("foo")
	if val != "bar" {
		t.Errorf("c.Get(foo)=%q, want %q", val, "bar")
	}

	c.Set(tinylfu.NewItem("foo", "baz"))

	val, _ = c.Get("foo")
	if val != "baz" {
		t.Errorf("c.Get(foo)=%q, want %q", val, "baz")
	}
}

func BenchmarkGet(b *testing.B) {
	c := tinylfu.New[string](64, 640)
	key := "some arbitrary key"
	c.Set(tinylfu.NewItem(key, "some arbitrary value"))
	for b.Loop() {
		c.Get(key)
	}
}
