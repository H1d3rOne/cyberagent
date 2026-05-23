package cache

import (
	"sync"
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	c := New(100)
	c.Set("key1", "value1", 0)

	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("got %v, want value1", val)
	}
}

func TestCacheGetMissing(t *testing.T) {
	c := New(100)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected missing key to return false")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	c := New(100)
	c.Set("expire", "soon", 50*time.Millisecond)

	val, ok := c.Get("expire")
	if !ok || val != "soon" {
		t.Fatal("expected key to exist before TTL")
	}

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get("expire")
	if ok {
		t.Error("expected key to be expired after TTL")
	}
}

func TestCacheEviction(t *testing.T) {
	c := New(3)
	c.Set("a", 1, 0)
	time.Sleep(time.Millisecond)
	c.Set("b", 2, 0)
	time.Sleep(time.Millisecond)
	c.Set("c", 3, 0)
	time.Sleep(time.Millisecond)
	c.Set("d", 4, 0)

	if c.Len() > 3 {
		t.Errorf("expected max 3 entries, got %d", c.Len())
	}
	if c.Has("a") {
		t.Error("expected oldest entry 'a' to be evicted")
	}
	if !c.Has("d") {
		t.Error("expected newest entry 'd' to exist")
	}
}

func TestCacheDelete(t *testing.T) {
	c := New(100)
	c.Set("key", "val", 0)
	c.Delete("key")
	if c.Has("key") {
		t.Error("expected key to be deleted")
	}
}

func TestCacheClear(t *testing.T) {
	c := New(100)
	c.Set("a", 1, 0)
	c.Set("b", 2, 0)
	c.Clear()
	if c.Len() != 0 {
		t.Errorf("expected 0 entries after clear, got %d", c.Len())
	}
}

func TestCacheConcurrency(t *testing.T) {
	c := New(1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := HashString("key" + string(rune(n)))
			c.Set(key, n, time.Second)
			c.Get(key)
			c.Has(key)
		}(i)
	}
	wg.Wait()
}

func TestURLDedupMark(t *testing.T) {
	d := NewURLDedup()

	first := d.Mark("http://localhost/api/users")
	if !first {
		t.Error("first mark should return true")
	}

	second := d.Mark("http://localhost/api/users")
	if second {
		t.Error("second mark of same URL should return false")
	}
}

func TestURLDedupSeen(t *testing.T) {
	d := NewURLDedup()

	if d.Seen("http://example.com") {
		t.Error("unseen URL should return false")
	}

	d.Mark("http://example.com")
	if !d.Seen("http://example.com") {
		t.Error("marked URL should be seen")
	}
}

func TestURLDedupReset(t *testing.T) {
	d := NewURLDedup()
	d.Mark("http://a.com")
	d.Mark("http://b.com")
	d.Reset()
	if d.Count() != 0 {
		t.Errorf("expected 0 after reset, got %d", d.Count())
	}
}

func TestURLDedupConcurrency(t *testing.T) {
	d := NewURLDedup()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			url := "http://localhost/" + HashString(string(rune(n)))
			d.Mark(url)
			d.Seen(url)
		}(i)
	}
	wg.Wait()
}

func TestHashStringDeterministic(t *testing.T) {
	h1 := HashString("test")
	h2 := HashString("test")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	h3 := HashString("other")
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}
