package glsi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/user/glsi/internal/cache"
	"github.com/user/glsi/internal/engine"
	"github.com/user/glsi/internal/scraper"
	"github.com/user/glsi/internal/search"
)

// fakeGoogleHTML builds a Google-like SERP page pointing at the given URLs.
func fakeGoogleHTML(urls []string) string {
	html := `<!DOCTYPE html><html><body>`
	for i, u := range urls {
		html += fmt.Sprintf(`<div class="g"><a href="%s"><h3>Result %d</h3></a></div>`, u, i+1)
	}
	html += `</body></html>`
	return html
}

// fakeArticlePage returns an HTML page that go-readability can extract from.
func fakeArticlePage(title, body string) string {
	return `<!DOCTYPE html>
<html>
<head><title>` + title + `</title></head>
<body>
<article>
<h1>` + title + `</h1>
<p>` + body + `</p>
</article>
</body>
</html>`
}

// TestIntegrationSearchScrapeCache exercises the full pipeline:
//
//	search → scrape → cache → (cache-hit) → clear-cache → (cache-miss)
//
// Everything runs against httptest servers so no real network traffic occurs.
func TestIntegrationSearchScrapeCache(t *testing.T) {
	// ── 1. Set up a "content" server that hosts scrapeable article pages ──
	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/article/1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage(
			"Go Concurrency Patterns",
			"Goroutines and channels are the building blocks of concurrent Go programs. "+
				"This article explains common patterns for safe concurrent access.",
		)))
	})
	contentMux.HandleFunc("/article/2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage(
			"Advanced Go Testing",
			"Table-driven tests and subtests make Go testing expressive and maintainable. "+
				"This guide covers everything from httptest to benchmarks.",
		)))
	})
	contentSrv := httptest.NewServer(contentMux)
	defer contentSrv.Close()

	// ── 2. Set up a "search engine" server that returns fake SERP HTML ──
	searchMux := http.NewServeMux()
	searchMux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		// Return links pointing at the content server.
		urls := []string{
			contentSrv.URL + "/article/1",
			contentSrv.URL + "/article/2",
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML(urls)))
	})
	searchSrv := httptest.NewServer(searchMux)
	defer searchSrv.Close()

	// ── 3. Override the search and scraper HTTP clients ──
	restoreSearchClient := search.OverrideHTTPClient(searchSrv.Client())
	defer restoreSearchClient()

	restoreBaseURLs := search.OverrideBaseURLs(searchSrv.URL, searchSrv.URL)
	defer restoreBaseURLs()

	restoreScraperClient := scraper.OverrideHTTPClient(contentSrv.Client())
	defer restoreScraperClient()

	// ── 4. Create a real SQLite cache in a temp directory ──
	dbPath := filepath.Join(t.TempDir(), "integration_test.db")
	c, err := cache.New(dbPath)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	// ── 5. Build the engine ──
	eng := engine.New(c, engine.Config{
		SearchEngine: "google",
		RateLimit:    0, // no delay in tests
	})

	ctx := context.Background()

	// ── 6. First search → should be a cache MISS (fresh scrape) ──
	result, err := eng.Search(ctx, "golang concurrency", 5, false)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	if result.Content == "" {
		t.Fatal("first Search returned empty content")
	}
	if result.FromCache {
		t.Error("first Search should not be from cache")
	}
	if result.ResultCount == 0 {
		t.Error("first Search should have a non-zero result count")
	}
	// Verify the consolidated content contains both article URLs.
	if !strings.Contains(result.Content, "/article/1") {
		t.Error("content should reference /article/1")
	}
	if !strings.Contains(result.Content, "/article/2") {
		t.Error("content should reference /article/2")
	}
	// Verify some extracted text is present.
	if !strings.Contains(result.Content, "Goroutines") && !strings.Contains(result.Content, "concurrent") {
		t.Error("content should contain extracted article text")
	}

	// ── 7. Second search (same query) → should be a cache HIT ──
	// To prove it's a cache hit, shut down the content server. If the engine
	// tries to scrape, it will fail.
	contentSrv.Close()
	searchSrv.Close()

	result2, err := eng.Search(ctx, "golang concurrency", 5, false)
	if err != nil {
		t.Fatalf("second Search (cache hit): %v", err)
	}
	if result2.Content != result.Content {
		t.Error("second Search should return identical cached content")
	}
	if !result2.FromCache {
		t.Error("second Search should be from cache")
	}

	// ── 8. ClearCache for this specific query ──
	if err := eng.ClearCache("golang concurrency"); err != nil {
		t.Fatalf("ClearCache: %v", err)
	}

	// ── 9. After clearing, searching without servers should fail ──
	_, err = eng.Search(ctx, "golang concurrency", 5, false)
	if err == nil {
		t.Fatal("expected error after cache clear and servers down")
	}
}

// TestIntegrationForceBypassCache verifies that force=true skips the cache
// and performs a fresh scrape.
func TestIntegrationForceBypassCache(t *testing.T) {
	callCount := 0
	contentMux := http.NewServeMux()
	contentMux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage(
			"Dynamic Page",
			fmt.Sprintf("This is version %d of the page content with enough text.", callCount),
		)))
	})
	contentSrv := httptest.NewServer(contentMux)
	defer contentSrv.Close()

	searchMux := http.NewServeMux()
	searchMux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML([]string{contentSrv.URL + "/page"})))
	})
	searchSrv := httptest.NewServer(searchMux)
	defer searchSrv.Close()

	restoreSearchClient := search.OverrideHTTPClient(searchSrv.Client())
	defer restoreSearchClient()
	restoreBaseURLs := search.OverrideBaseURLs(searchSrv.URL, searchSrv.URL)
	defer restoreBaseURLs()
	restoreScraperClient := scraper.OverrideHTTPClient(contentSrv.Client())
	defer restoreScraperClient()

	dbPath := filepath.Join(t.TempDir(), "force_test.db")
	c, err := cache.New(dbPath)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	eng := engine.New(c, engine.Config{
		SearchEngine: "google",
		RateLimit:    0,
	})

	ctx := context.Background()

	// First search — cache miss.
	result1, err := eng.Search(ctx, "dynamic", 5, false)
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}
	if !strings.Contains(result1.Content, "version 1") {
		t.Errorf("expected version 1 content, got: %s", result1.Content)
	}

	// Second search with force=true — should bypass cache and get version 2.
	result2, err := eng.Search(ctx, "dynamic", 5, true)
	if err != nil {
		t.Fatalf("forced Search: %v", err)
	}
	if !strings.Contains(result2.Content, "version 2") {
		t.Errorf("expected version 2 (force bypass), got: %s", result2.Content)
	}
	if result1.Content == result2.Content {
		t.Error("forced search should produce different content than cached")
	}
}

// TestIntegrationClearAllCache verifies that ClearCache("") flushes everything.
func TestIntegrationClearAllCache(t *testing.T) {
	contentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("Any Page", "Generic content for cache test.")))
	}))
	defer contentSrv.Close()

	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML([]string{contentSrv.URL + "/p"})))
	}))
	defer searchSrv.Close()

	restoreSearchClient := search.OverrideHTTPClient(searchSrv.Client())
	defer restoreSearchClient()
	restoreBaseURLs := search.OverrideBaseURLs(searchSrv.URL, searchSrv.URL)
	defer restoreBaseURLs()
	restoreScraperClient := scraper.OverrideHTTPClient(contentSrv.Client())
	defer restoreScraperClient()

	dbPath := filepath.Join(t.TempDir(), "clearall_test.db")
	c, err := cache.New(dbPath)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	eng := engine.New(c, engine.Config{SearchEngine: "google", RateLimit: 0})
	ctx := context.Background()

	// Populate cache with two different queries.
	if _, err := eng.Search(ctx, "query one", 5, false); err != nil {
		t.Fatalf("Search query one: %v", err)
	}
	if _, err := eng.Search(ctx, "query two", 5, false); err != nil {
		t.Fatalf("Search query two: %v", err)
	}

	// Flush all.
	if err := eng.ClearCache(""); err != nil {
		t.Fatalf("ClearCache all: %v", err)
	}

	// Now shut down servers — cache should be empty, so searches should fail.
	contentSrv.Close()
	searchSrv.Close()

	_, err = eng.Search(ctx, "query one", 5, false)
	if err == nil {
		t.Error("expected error after flushing cache for 'query one'")
	}
	_, err = eng.Search(ctx, "query two", 5, false)
	if err == nil {
		t.Error("expected error after flushing cache for 'query two'")
	}
}

// TestIntegrationRateLimit ensures the engine respects the rate limit.
func TestIntegrationRateLimit(t *testing.T) {
	contentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("Rate Limited", "Testing rate limit delay.")))
	}))
	defer contentSrv.Close()

	searchSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML([]string{contentSrv.URL + "/p"})))
	}))
	defer searchSrv.Close()

	restoreSearchClient := search.OverrideHTTPClient(searchSrv.Client())
	defer restoreSearchClient()
	restoreBaseURLs := search.OverrideBaseURLs(searchSrv.URL, searchSrv.URL)
	defer restoreBaseURLs()
	restoreScraperClient := scraper.OverrideHTTPClient(contentSrv.Client())
	defer restoreScraperClient()

	dbPath := filepath.Join(t.TempDir(), "ratelimit_test.db")
	c, err := cache.New(dbPath)
	if err != nil {
		t.Fatalf("cache.New: %v", err)
	}
	defer c.Close()

	rateLimit := 200 * time.Millisecond
	eng := engine.New(c, engine.Config{
		SearchEngine: "google",
		RateLimit:    rateLimit,
	})

	start := time.Now()
	_, err = eng.Search(context.Background(), "rate limit test", 5, false)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// The engine should have waited at least the rate limit duration.
	if elapsed < rateLimit {
		t.Errorf("search completed in %v, expected at least %v (rate limit)", elapsed, rateLimit)
	}
}
