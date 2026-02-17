package scraper

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	readability "github.com/go-shiori/go-readability"
)

const perURLTimeout = 3 * time.Second

// httpClient is the HTTP client used for scraping. Tests can override it.
var httpClient = &http.Client{}

// OverrideHTTPClient replaces the HTTP client used by the scraper
// package and returns a function to restore the original.
// Intended for testing only.
func OverrideHTTPClient(c *http.Client) (restore func()) {
	orig := httpClient
	httpClient = c
	return func() { httpClient = orig }
}

// ScrapedPage holds the result of scraping a single URL.
type ScrapedPage struct {
	URL     string
	Content string
	Err     error
}

// Scrape concurrently fetches each URL, extracts readable text via
// go-readability, and returns results for every URL (including per-URL errors).
func Scrape(ctx context.Context, urls []string) []ScrapedPage {
	results := make([]ScrapedPage, len(urls))
	var wg sync.WaitGroup

	for i, u := range urls {
		wg.Add(1)
		go func(idx int, rawURL string) {
			defer wg.Done()
			content, err := scrapeSingle(ctx, rawURL)
			results[idx] = ScrapedPage{
				URL:     rawURL,
				Content: content,
				Err:     err,
			}
		}(i, u)
	}

	wg.Wait()
	return results
}

func scrapeSingle(ctx context.Context, rawURL string) (string, error) {
	// Derive a per-URL context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(ctx, perURLTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := *httpClient
	client.Timeout = perURLTimeout
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}

	article, err := readability.FromReader(resp.Body, nil)
	if err != nil {
		return "", fmt.Errorf("readability parse %s: %w", rawURL, err)
	}

	return article.TextContent, nil
}
