package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeArticlePage returns a realistic-looking HTML page that go-readability
// can extract text from.
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

// setupScrapeServer creates a test server and overrides the package-level
// httpClient. Returns the server URL and a cleanup function.
func setupScrapeServer(t *testing.T, handler http.Handler) (serverURL string, cleanup func()) {
	t.Helper()

	srv := httptest.NewServer(handler)

	origClient := httpClient
	httpClient = srv.Client()

	return srv.URL, func() {
		srv.Close()
		httpClient = origClient
	}
}

func TestScrapeSinglePage(t *testing.T) {
	serverURL, cleanup := setupScrapeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage(
			"Test Article",
			"This is the main content of the test article. It contains enough text for readability to extract.",
		)))
	}))
	defer cleanup()

	pages := Scrape(context.Background(), []string{serverURL + "/page"})
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if pages[0].Err != nil {
		t.Fatalf("unexpected error: %v", pages[0].Err)
	}
	if !strings.Contains(pages[0].Content, "main content") {
		t.Errorf("content should contain article text, got: %q", pages[0].Content)
	}
}

func TestScrapeMultiplePages(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("Page 1", "Content from the first page with enough detail.")))
	})
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("Page 2", "Content from the second page with more detail.")))
	})

	serverURL, cleanup := setupScrapeServer(t, mux)
	defer cleanup()

	urls := []string{serverURL + "/page1", serverURL + "/page2"}
	pages := Scrape(context.Background(), urls)

	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}

	for i, p := range pages {
		if p.Err != nil {
			t.Errorf("page[%d]: unexpected error: %v", i, p.Err)
		}
		if p.URL != urls[i] {
			t.Errorf("page[%d].URL = %q, want %q", i, p.URL, urls[i])
		}
		if p.Content == "" {
			t.Errorf("page[%d].Content is empty", i)
		}
	}
}

func TestScrapeServerError(t *testing.T) {
	serverURL, cleanup := setupScrapeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer cleanup()

	pages := Scrape(context.Background(), []string{serverURL + "/error"})
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if pages[0].Err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestScrapeNotFound(t *testing.T) {
	serverURL, cleanup := setupScrapeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer cleanup()

	pages := Scrape(context.Background(), []string{serverURL + "/missing"})
	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	if pages[0].Err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestScrapeEmptyList(t *testing.T) {
	pages := Scrape(context.Background(), nil)
	if len(pages) != 0 {
		t.Fatalf("got %d pages, want 0 for empty URL list", len(pages))
	}
}

func TestScrapePreservesOrder(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("A", "Alpha article content here.")))
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("B", "Bravo article content here.")))
	})
	mux.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("C", "Charlie article content here.")))
	})

	serverURL, cleanup := setupScrapeServer(t, mux)
	defer cleanup()

	urls := []string{serverURL + "/a", serverURL + "/b", serverURL + "/c"}
	pages := Scrape(context.Background(), urls)

	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	for i, p := range pages {
		if p.URL != urls[i] {
			t.Errorf("page[%d].URL = %q, want %q (order mismatch)", i, p.URL, urls[i])
		}
	}
}

func TestScrapePartialFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fakeArticlePage("OK Page", "This page loads successfully with good content.")))
	})
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	serverURL, cleanup := setupScrapeServer(t, mux)
	defer cleanup()

	urls := []string{serverURL + "/ok", serverURL + "/fail"}
	pages := Scrape(context.Background(), urls)

	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	if pages[0].Err != nil {
		t.Errorf("page[0] should succeed, got error: %v", pages[0].Err)
	}
	if pages[1].Err == nil {
		t.Error("page[1] should fail, got nil error")
	}
}
