package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeGoogleHTML returns a minimal Google-like SERP page with div.g results.
func fakeGoogleHTML(links []struct{ URL, Title string }) string {
	html := `<!DOCTYPE html><html><body>`
	for _, l := range links {
		html += `<div class="g"><a href="` + l.URL + `"><h3>` + l.Title + `</h3></a></div>`
	}
	html += `</body></html>`
	return html
}

// fakeGoogleFallbackHTML returns a page with /url?q= redirect-style links
// (no div.g), triggering the fallback parser.
func fakeGoogleFallbackHTML(links []struct{ URL, Title string }) string {
	html := `<!DOCTYPE html><html><body>`
	for _, l := range links {
		html += `<a href="/url?q=` + l.URL + `&sa=U">` + l.Title + `</a>`
	}
	html += `</body></html>`
	return html
}

// fakeDuckDuckGoHTML returns a minimal DDG-like results page.
func fakeDuckDuckGoHTML(links []struct{ URL, Title string }) string {
	html := `<!DOCTYPE html><html><body>`
	for _, l := range links {
		html += `<a class="result__a" href="` + l.URL + `">` + l.Title + `</a>`
	}
	html += `</body></html>`
	return html
}

// setupTestServer starts an httptest.Server and overrides the package-level
// variables to point at it. It returns a cleanup function that restores
// the original values.
func setupTestServer(t *testing.T, handler http.Handler) (cleanup func()) {
	t.Helper()

	srv := httptest.NewServer(handler)

	origClient := httpClient
	origGoogle := baseURLGoogle
	origDDG := baseURLDuckDuckGo

	httpClient = srv.Client()
	baseURLGoogle = srv.URL
	baseURLDuckDuckGo = srv.URL

	return func() {
		srv.Close()
		httpClient = origClient
		baseURLGoogle = origGoogle
		baseURLDuckDuckGo = origDDG
	}
}

func TestSearchGoogle(t *testing.T) {
	links := []struct{ URL, Title string }{
		{"https://example.com/page1", "Example Page 1"},
		{"https://example.com/page2", "Example Page 2"},
		{"https://example.com/page3", "Example Page 3"},
	}

	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML(links)))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "test query", 5, "google")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	for i, r := range results {
		if r.URL != links[i].URL {
			t.Errorf("result[%d].URL = %q, want %q", i, r.URL, links[i].URL)
		}
		if r.Title != links[i].Title {
			t.Errorf("result[%d].Title = %q, want %q", i, r.Title, links[i].Title)
		}
	}
}

func TestSearchGoogleCountLimit(t *testing.T) {
	links := []struct{ URL, Title string }{
		{"https://a.com", "A"},
		{"https://b.com", "B"},
		{"https://c.com", "C"},
		{"https://d.com", "D"},
	}

	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleHTML(links)))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "test", 2, "google")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2 (count limit)", len(results))
	}
}

func TestSearchGoogleFallback(t *testing.T) {
	links := []struct{ URL, Title string }{
		{"https://example.com/fallback1", "Fallback 1"},
		{"https://example.com/fallback2", "Fallback 2"},
	}

	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeGoogleFallbackHTML(links)))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "test", 5, "google")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected fallback parser to extract results, got 0")
	}
	// Fallback should have extracted at least the first link.
	if results[0].URL != links[0].URL {
		t.Errorf("result[0].URL = %q, want %q", results[0].URL, links[0].URL)
	}
}

func TestSearchDuckDuckGo(t *testing.T) {
	links := []struct{ URL, Title string }{
		{"https://example.com/ddg1", "DDG Result 1"},
		{"https://example.com/ddg2", "DDG Result 2"},
	}

	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeDuckDuckGoHTML(links)))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "duck test", 5, "duckduckgo")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	for i, r := range results {
		if r.URL != links[i].URL {
			t.Errorf("result[%d].URL = %q, want %q", i, r.URL, links[i].URL)
		}
		if r.Title != links[i].Title {
			t.Errorf("result[%d].Title = %q, want %q", i, r.Title, links[i].Title)
		}
	}
}

func TestSearchDDGAlias(t *testing.T) {
	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeDuckDuckGoHTML([]struct{ URL, Title string }{
			{"https://example.com/1", "One"},
		})))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "q", 5, "ddg")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (ddg alias)", len(results))
	}
}

func TestSearchEmptyPage(t *testing.T) {
	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><body><p>No results</p></body></html>`))
	}))
	defer cleanup()

	results, err := Search(context.Background(), "nothing here", 5, "google")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0 for empty page", len(results))
	}
}

func TestSearchServerError(t *testing.T) {
	cleanup := setupTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer cleanup()

	_, err := Search(context.Background(), "error", 5, "google")
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
