package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/user/glsi/internal/engine"
)

// ListenAndServe starts an HTTP API server on the given address.
func ListenAndServe(addr string, eng *engine.Engine) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/search", searchHandler(eng))
	mux.HandleFunc("/cache", cacheHandler(eng))
	mux.HandleFunc("/health", healthHandler)

	fmt.Fprintf(os.Stderr, "GLSI HTTP API listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
}

type apiResponse struct {
	Content     string `json:"content,omitempty"`
	ResultCount int    `json:"result_count,omitempty"`
	FromCache   bool   `json:"from_cache,omitempty"`
	Error       string `json:"error,omitempty"`
	Status      string `json:"status,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func searchHandler(eng *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Error: "method not allowed"})
			return
		}

		q := r.URL.Query().Get("q")
		if q == "" {
			writeJSON(w, http.StatusBadRequest, apiResponse{Error: "missing required query parameter 'q'"})
			return
		}

		count := 5
		if c := r.URL.Query().Get("count"); c != "" {
			if n, err := strconv.Atoi(c); err == nil && n > 0 {
				count = n
			}
		}

		force := false
		if f := r.URL.Query().Get("force"); f == "true" || f == "1" {
			force = true
		}

		result, err := eng.Search(r.Context(), q, count, force)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, apiResponse{
			Content:     result.Content,
			ResultCount: result.ResultCount,
			FromCache:   result.FromCache,
		})
	}
}

func cacheHandler(eng *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Error: "method not allowed"})
			return
		}

		q := r.URL.Query().Get("q")
		if err := eng.ClearCache(q); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, apiResponse{Status: "ok"})
}
