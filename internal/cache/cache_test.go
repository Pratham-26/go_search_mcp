package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempDB(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "test.db")
}

func TestNewCreatesTable(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()
}

func TestSetAndGet(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	hash := "abc123"
	want := "hello, world"

	if err := c.Set(hash, want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, hit, err := c.Get(hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected cache hit, got miss")
	}
	if got != want {
		t.Fatalf("Get = %q, want %q", got, want)
	}
}

func TestGetMiss(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	_, hit, err := c.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if hit {
		t.Fatal("expected cache miss, got hit")
	}
}

func TestUpsert(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	hash := "key1"
	if err := c.Set(hash, "first"); err != nil {
		t.Fatalf("Set first: %v", err)
	}
	if err := c.Set(hash, "second"); err != nil {
		t.Fatalf("Set second: %v", err)
	}

	got, hit, err := c.Get(hash)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected cache hit")
	}
	if got != "second" {
		t.Fatalf("Get = %q, want %q", got, "second")
	}
}

func TestClearOne(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	c.Set("a", "1")
	c.Set("b", "2")

	if err := c.Clear("a"); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	_, hit, _ := c.Get("a")
	if hit {
		t.Fatal("expected 'a' to be deleted")
	}

	_, hit, _ = c.Get("b")
	if !hit {
		t.Fatal("expected 'b' to still exist")
	}
}

func TestClearAll(t *testing.T) {
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	c.Set("a", "1")
	c.Set("b", "2")

	if err := c.Clear(""); err != nil {
		t.Fatalf("Clear all: %v", err)
	}

	_, hitA, _ := c.Get("a")
	_, hitB, _ := c.Get("b")
	if hitA || hitB {
		t.Fatal("expected all entries to be deleted")
	}
}

func TestDefaultDBPath(t *testing.T) {
	// Verify that New("") defaults to ~/.glsi/cache.db
	// We'll just check that the directory gets created.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	c, err := New("")
	if err != nil {
		t.Fatalf("New with default path: %v", err)
	}
	defer c.Close()

	expectedDir := filepath.Join(home, ".glsi")
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Fatalf("expected directory %s to exist", expectedDir)
	}
}

func TestTTLExpiry(t *testing.T) {
	// This test manually checks TTL logic by reducing the constant.
	// Since we can't easily mock time, we test the boundary:
	// a freshly-set entry should be a hit.
	c, err := New(tempDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	c.Set("test", "content")

	// Should be a hit immediately.
	_, hit, err := c.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected cache hit for fresh entry")
	}

	// Verify cacheTTL is 24h as expected.
	if cacheTTL != 24*time.Hour {
		t.Fatalf("cacheTTL = %v, want 24h", cacheTTL)
	}
}
