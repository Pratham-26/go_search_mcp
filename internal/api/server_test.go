package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	healthHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var resp apiResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %q, want %q", resp.Status, "ok")
	}
}

func TestSearchHandlerMissingQuery(t *testing.T) {
	// Create a handler with a nil engine — it should reject before calling engine.
	handler := searchHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var resp apiResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Error == "" {
		t.Fatal("expected error message for missing query")
	}
}

func TestSearchHandlerWrongMethod(t *testing.T) {
	handler := searchHandler(nil)

	req := httptest.NewRequest(http.MethodPost, "/search?q=test", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestCacheHandlerWrongMethod(t *testing.T) {
	handler := cacheHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/cache", nil)
	rr := httptest.NewRecorder()

	handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}
