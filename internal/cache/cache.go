package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultDBDir  = ".glsi"
	defaultDBFile = "cache.db"
	cacheTTL      = 24 * time.Hour
)

// Cache provides a SQLite-backed key–value cache with TTL support.
type Cache struct {
	db *sql.DB
}

// New opens (or creates) a SQLite cache database at dbPath.
// If dbPath is empty, it defaults to ~/.glsi/cache.db.
func New(dbPath string) (*Cache, error) {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cache: user home dir: %w", err)
		}
		dir := filepath.Join(home, defaultDBDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("cache: create dir %s: %w", dir, err)
		}
		dbPath = filepath.Join(dir, defaultDBFile)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cache: open db: %w", err)
	}

	// Create the cache table if it does not exist.
	const createSQL = `
		CREATE TABLE IF NOT EXISTS cache (
			query_hash TEXT PRIMARY KEY,
			content    TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
	if _, err := db.Exec(createSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: create table: %w", err)
	}

	return &Cache{db: db}, nil
}

// Get retrieves cached content for the given query hash.
// It returns the content, whether the cache was hit (i.e. entry exists and is
// not older than 24 hours), and any error.
func (c *Cache) Get(queryHash string) (string, bool, error) {
	var content string
	var updatedAt time.Time

	err := c.db.QueryRow(
		"SELECT content, updated_at FROM cache WHERE query_hash = ?",
		queryHash,
	).Scan(&content, &updatedAt)

	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cache: get %q: %w", queryHash, err)
	}

	// Stale if older than TTL.
	if time.Since(updatedAt) > cacheTTL {
		return "", false, nil
	}

	return content, true, nil
}

// Set upserts content for the given query hash.
func (c *Cache) Set(queryHash, content string) error {
	const upsertSQL = `
		INSERT INTO cache (query_hash, content, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(query_hash) DO UPDATE SET
			content    = excluded.content,
			updated_at = excluded.updated_at;`

	if _, err := c.db.Exec(upsertSQL, queryHash, content); err != nil {
		return fmt.Errorf("cache: set %q: %w", queryHash, err)
	}
	return nil
}

// Clear removes cached entries.
// If queryHash is empty, all entries are flushed.
// Otherwise, only the entry matching the hash is deleted.
func (c *Cache) Clear(queryHash string) error {
	var err error
	if queryHash == "" {
		_, err = c.db.Exec("DELETE FROM cache")
	} else {
		_, err = c.db.Exec("DELETE FROM cache WHERE query_hash = ?", queryHash)
	}
	if err != nil {
		return fmt.Errorf("cache: clear: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}
