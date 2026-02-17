package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Result holds a single search-engine result.
type Result struct {
	URL   string
	Title string
}

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Package-level variables for testability. Tests can override these.
var (
	httpClient        = http.DefaultClient
	baseURLGoogle     = "https://www.google.com"
	baseURLDuckDuckGo = "https://html.duckduckgo.com"
)

// OverrideHTTPClient replaces the HTTP client used by the search
// package and returns a function to restore the original.
// Intended for testing only.
func OverrideHTTPClient(c *http.Client) (restore func()) {
	orig := httpClient
	httpClient = c
	return func() { httpClient = orig }
}

// OverrideBaseURLs replaces the base URLs used for Google and DuckDuckGo
// search and returns a function to restore the originals.
// Intended for testing only.
func OverrideBaseURLs(google, ddg string) (restore func()) {
	origG, origD := baseURLGoogle, baseURLDuckDuckGo
	baseURLGoogle = google
	baseURLDuckDuckGo = ddg
	return func() { baseURLGoogle = origG; baseURLDuckDuckGo = origD }
}

// Search scrapes a search engine results page and returns up to count results.
// Supported engines: "google" (default), "duckduckgo".
func Search(ctx context.Context, query string, count int, engine string) ([]Result, error) {
	switch strings.ToLower(engine) {
	case "duckduckgo", "ddg":
		return searchDuckDuckGo(ctx, query, count)
	default: // google
		return searchGoogle(ctx, query, count)
	}
}

func searchGoogle(ctx context.Context, query string, count int) ([]Result, error) {
	u := fmt.Sprintf("%s/search?q=%s&num=%d",
		baseURLGoogle, url.QueryEscape(query), count)

	doc, err := fetchDocument(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("search google: %w", err)
	}

	var results []Result
	// Google wraps organic results in divs with class "g".
	doc.Find("div.g").Each(func(_ int, s *goquery.Selection) {
		if len(results) >= count {
			return
		}
		link := s.Find("a").First()
		href, exists := link.Attr("href")
		if !exists || href == "" {
			return
		}
		// Skip Google's own links, ads, etc.
		if strings.HasPrefix(href, "/") || strings.Contains(href, "google.com") {
			return
		}
		title := s.Find("h3").First().Text()
		if title == "" {
			title = link.Text()
		}
		results = append(results, Result{URL: href, Title: strings.TrimSpace(title)})
	})

	if len(results) == 0 {
		// Fallback: try extracting all anchor tags with absolute URLs.
		doc.Find("a").Each(func(_ int, s *goquery.Selection) {
			if len(results) >= count {
				return
			}
			href, exists := s.Attr("href")
			if !exists {
				return
			}
			// Extract URL from Google redirect links: /url?q=...&sa=...
			if strings.HasPrefix(href, "/url?") {
				if parsed, err := url.Parse(href); err == nil {
					href = parsed.Query().Get("q")
				}
			}
			if href == "" || !strings.HasPrefix(href, "http") {
				return
			}
			if strings.Contains(href, "google.com") || strings.Contains(href, "youtube.com") {
				return
			}
			title := strings.TrimSpace(s.Text())
			if title == "" || len(title) > 200 {
				return
			}
			results = append(results, Result{URL: href, Title: title})
		})
	}

	return results, nil
}

func searchDuckDuckGo(ctx context.Context, query string, count int) ([]Result, error) {
	u := fmt.Sprintf("%s/html/?q=%s", baseURLDuckDuckGo, url.QueryEscape(query))

	doc, err := fetchDocument(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("search duckduckgo: %w", err)
	}

	var results []Result
	doc.Find("a.result__a").Each(func(_ int, s *goquery.Selection) {
		if len(results) >= count {
			return
		}
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}
		// DuckDuckGo sometimes wraps URLs in a redirect.
		if strings.Contains(href, "duckduckgo.com/l/?") {
			if parsed, err := url.Parse(href); err == nil {
				if uddg := parsed.Query().Get("uddg"); uddg != "" {
					href = uddg
				}
			}
		}
		title := strings.TrimSpace(s.Text())
		results = append(results, Result{URL: href, Title: title})
	})

	return results, nil
}

func fetchDocument(ctx context.Context, rawURL string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	return doc, nil
}
