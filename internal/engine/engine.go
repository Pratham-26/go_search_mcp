package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/user/glsi/internal/cache"
	"github.com/user/glsi/internal/scraper"
	"github.com/user/glsi/internal/search"
)

// Config holds engine-level configuration.
type Config struct {
	SearchEngine string        // "google" or "duckduckgo"
	RateLimit    time.Duration // delay between outgoing requests
}

// SearchResult holds the output of a search pipeline run.
type SearchResult struct {
	Content     string // consolidated text from scraped pages
	ResultCount int    // number of pages successfully scraped
	FromCache   bool   // true if the result was served from cache
}

// Engine orchestrates the search → scrape → cache pipeline.
type Engine struct {
	cache  *cache.Cache
	config Config
}

// New creates a new Engine with the given cache and configuration.
func New(c *cache.Cache, cfg Config) *Engine {
	return &Engine{cache: c, config: cfg}
}

// Search executes the full pipeline: hash → cache check → search → scrape →
// consolidate → upsert → return.
//
// If force is true the cache is bypassed and a fresh scrape is performed.
func (e *Engine) Search(ctx context.Context, query string, count int, force bool) (SearchResult, error) {
	hash := queryHash(query)

	// 1. Cache check (skip when force is set).
	if !force {
		content, hit, err := e.cache.Get(hash)
		if err != nil {
			return SearchResult{}, fmt.Errorf("engine: cache get: %w", err)
		}
		if hit {
			return SearchResult{
				Content:     content,
				ResultCount: countSections(content),
				FromCache:   true,
			}, nil
		}
	}

	// 2. Search — scrape search-engine results page.
	results, err := search.Search(ctx, query, count, e.config.SearchEngine)
	if err != nil {
		return SearchResult{}, fmt.Errorf("engine: search: %w", err)
	}
	if len(results) == 0 {
		return SearchResult{}, fmt.Errorf("engine: no search results for %q", query)
	}

	// Rate-limit between the search request and the page scrapes.
	if e.config.RateLimit > 0 {
		time.Sleep(e.config.RateLimit)
	}

	// 3. Scrape all result URLs concurrently.
	urls := make([]string, len(results))
	for i, r := range results {
		urls[i] = r.URL
	}
	pages := scraper.Scrape(ctx, urls)

	// 4. Consolidate into a single text block.
	content, resultCount := consolidate(pages)
	if content == "" {
		return SearchResult{}, fmt.Errorf("engine: all pages failed to scrape for %q", query)
	}

	// 5. Upsert into cache.
	if err := e.cache.Set(hash, content); err != nil {
		return SearchResult{}, fmt.Errorf("engine: cache set: %w", err)
	}

	return SearchResult{
		Content:     content,
		ResultCount: resultCount,
		FromCache:   false,
	}, nil
}

// ClearCache removes cached entries.
// If query is empty, all entries are flushed; otherwise only the matching
// entry is deleted.
func (e *Engine) ClearCache(query string) error {
	hash := ""
	if query != "" {
		hash = queryHash(query)
	}
	if err := e.cache.Clear(hash); err != nil {
		return fmt.Errorf("engine: clear cache: %w", err)
	}
	return nil
}

// queryHash produces a deterministic SHA-256 hex string for a query.
func queryHash(query string) string {
	normalized := strings.TrimSpace(strings.ToLower(query))
	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h)
}

// consolidate joins scraped page texts, each headed by its source URL.
// It returns the consolidated text and the number of pages successfully included.
func consolidate(pages []scraper.ScrapedPage) (string, int) {
	var b strings.Builder
	count := 0
	for _, p := range pages {
		if p.Err != nil || strings.TrimSpace(p.Content) == "" {
			continue
		}
		if count > 0 {
			b.WriteString("\n\n---\n\n")
		}
		count++
		fmt.Fprintf(&b, "## %s\n\n%s", p.URL, strings.TrimSpace(p.Content))
	}
	return b.String(), count
}

// countSections counts the number of "## " section headers in cached content.
// This is used to derive a result count from previously cached responses.
func countSections(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "## ") {
			count++
		}
	}
	return count
}
