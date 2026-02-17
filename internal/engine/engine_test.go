package engine

import (
	"fmt"
	"testing"

	"github.com/user/glsi/internal/scraper"
)

var errDummy = fmt.Errorf("dummy error")

func TestQueryHash(t *testing.T) {
	// Same query, different casing/whitespace → same hash.
	h1 := queryHash("Golang concurrency")
	h2 := queryHash("  golang concurrency  ")
	h3 := queryHash("GOLANG CONCURRENCY")

	if h1 != h2 {
		t.Fatalf("hash mismatch: %q vs %q", h1, h2)
	}
	if h1 != h3 {
		t.Fatalf("hash mismatch: %q vs %q", h1, h3)
	}

	// Different queries → different hash.
	h4 := queryHash("different query")
	if h1 == h4 {
		t.Fatal("different queries should produce different hashes")
	}
}

func TestConsolidate(t *testing.T) {
	tests := []struct {
		name      string
		pages     []scraper.ScrapedPage
		want      string
		wantCount int
	}{
		{
			name:      "empty",
			pages:     nil,
			want:      "",
			wantCount: 0,
		},
		{
			name: "all_errors",
			pages: []scraper.ScrapedPage{
				{URL: "http://a.com", Err: errDummy},
			},
			want:      "",
			wantCount: 0,
		},
		{
			name: "single_page",
			pages: []scraper.ScrapedPage{
				{URL: "http://a.com", Content: "Hello"},
			},
			want:      "## http://a.com\n\nHello",
			wantCount: 1,
		},
		{
			name: "multiple_pages",
			pages: []scraper.ScrapedPage{
				{URL: "http://a.com", Content: "First"},
				{URL: "http://b.com", Content: "Second"},
			},
			want:      "## http://a.com\n\nFirst\n\n---\n\n## http://b.com\n\nSecond",
			wantCount: 2,
		},
		{
			name: "skip_errors_and_empty",
			pages: []scraper.ScrapedPage{
				{URL: "http://a.com", Content: "OK"},
				{URL: "http://b.com", Err: errDummy},
				{URL: "http://c.com", Content: "  "},
				{URL: "http://d.com", Content: "Also OK"},
			},
			want:      "## http://a.com\n\nOK\n\n---\n\n## http://d.com\n\nAlso OK",
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotCount := consolidate(tt.pages)
			if got != tt.want {
				t.Errorf("consolidate() =\n%q\nwant\n%q", got, tt.want)
			}
			if gotCount != tt.wantCount {
				t.Errorf("consolidate() count = %d, want %d", gotCount, tt.wantCount)
			}
		})
	}
}
