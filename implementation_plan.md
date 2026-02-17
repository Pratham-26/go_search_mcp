# GLSI — Implementation Plan

**Status:** Complete ✅  
**Date:** 2026-02-16  
**PRD:** [`docs/prd.md`](docs/prd.md)

---

## File Structure

```
web_query_index/
├── cmd/
│   └── glsi/
│       └── main.go                 # Entry point — CLI parsing, subcommand dispatch
│
├── internal/
│   ├── search/
│   │   ├── search.go               # Scrapes Google/DuckDuckGo results via goquery
│   │   └── search_test.go
│   │
│   ├── scraper/
│   │   ├── scraper.go              # Concurrent URL scraping (goroutines, 3s timeout)
│   │   └── scraper_test.go
│   │
│   ├── cache/
│   │   ├── cache.go                # SQLite cache — init, get, set, clear, TTL
│   │   └── cache_test.go
│   │
│   ├── engine/
│   │   ├── engine.go               # Orchestrator — wires search → scrape → cache
│   │   └── engine_test.go
│   │
│   ├── api/
│   │   ├── server.go               # HTTP REST API server
│   │   └── server_test.go
│   │
│   └── mcp/
│       ├── server.go               # MCP stdio server — tool registration & handlers
│       └── server_test.go
│
├── docs/
│   └── prd.md
│
├── go.mod
├── go.sum
├── implementation_plan.md
└── README.md
```

### Why this layout?

| Directory | Purpose |
|---|---|
| `cmd/glsi/` | Single binary entry point. Keeps `main.go` thin — just flag parsing and dispatch. |
| `internal/search/` | Scrapes search-engine result pages directly using goquery (no external API). |
| `internal/scraper/` | Pure scraping logic (HTTP fetch + readability extraction). No knowledge of cache or search. |
| `internal/cache/` | SQLite operations and TTL logic. No knowledge of where data comes from. |
| `internal/engine/` | Glues the three layers together. Single `Run(query, n, force)` method all interfaces call. |
| `internal/api/` | HTTP REST API server. Exposes the same operations as the CLI and MCP server over HTTP. |
| `internal/mcp/` | MCP stdio server built on the official Go SDK. Registers tools and delegates to the engine. |

---

## Architecture

```
┌─────────────┐  ┌─────────────┐  ┌─────────────┐
│    CLI      │  │  HTTP API   │  │  MCP Server │
│  (search)   │  │  (REST)     │  │   (stdio)   │
└──────┬──────┘  └──────┬──────┘  └──────┬──────┘
       │                │                │
       └────────────────┼────────────────┘
                        │
                 ┌──────▼──────┐
                 │   Engine    │   ← orchestrates everything
                 │  Run(q,n,f) │
                 └──┬───────┬──┘
                    │       │
             ┌──────▼──┐  ┌─▼──────────┐
             │  Cache  │  │   Search   │
             │ (SQLite)│  │ (goquery)  │
             └─────────┘  └──────┬─────┘
                                 │
                          ┌──────▼──────┐
                          │   Scraper   │
                          │ (concurrent)│
                          └─────────────┘
```

**Flow for `web_search` / `glsi search`:**

1. Engine hashes the query → `query_hash`.
2. **Cache check** — if hit (`< 24h` and `!force`), return cached content immediately.
3. **Cache miss** — scrape search engine (Google/DuckDuckGo) to get result URLs.
4. Pass URLs to Scraper — concurrent goroutine fetch with 3s per-URL timeout.
5. Consolidate text (each section headed by its source URL), upsert into cache, return.

---

## Implementation Phases

### Phase 1 — Project Scaffold

- [x] `go mod init` with module path.
- [x] Create directory structure.
- [x] Add initial dependencies to `go.mod`:
  - `github.com/modelcontextprotocol/go-sdk`
  - `modernc.org/sqlite`
  - `github.com/go-shiori/go-readability`
  - `github.com/PuerkitoBio/goquery`

### Phase 2 — Cache Layer (`internal/cache/`)

**File:** `cache.go`

```go
type Cache struct { db *sql.DB }

func New(dbPath string) (*Cache, error)        // open DB, create table if missing
func (c *Cache) Get(queryHash string) (string, bool, error)  // returns content, hit, err
func (c *Cache) Set(queryHash, content string) error          // upsert
func (c *Cache) Clear(queryHash string) error                 // delete one or all
func (c *Cache) Close() error
```

Key details:
- Schema: `query_hash TEXT PK`, `content TEXT`, `updated_at DATETIME DEFAULT CURRENT_TIMESTAMP`.
- `Get` checks `updated_at` to determine if the entry is stale (> 24h).
- `Clear("")` → `DELETE FROM cache` (flush all).
- `Clear("abc")` → `DELETE FROM cache WHERE query_hash = 'abc'`.
- DB file defaults to `~/.glsi/cache.db`.

### Phase 3 — Search (`internal/search/`)

**File:** `search.go`

```go
type Result struct {
    URL   string
    Title string
}

func Search(ctx context.Context, query string, count int, engine string) ([]Result, error)
```

Key details:
- Directly scrapes a search engine's HTML results page — **no paid API needed**.
- Uses `goquery` (`github.com/PuerkitoBio/goquery`) to parse the results page and extract links + titles via CSS selectors.
- Supported engines: Google (`https://www.google.com/search?q=...&num=...`), DuckDuckGo (`https://html.duckduckgo.com/html/?q=...`).
- Engine configured via `GLSI_SEARCH_ENGINE` env var (default: `google`).
- Sets a realistic `User-Agent` header to avoid bot blocking.
- Respects `GLSI_RATE_LIMIT` delay between search requests.

### Phase 4 — Scraper (`internal/scraper/`)

**File:** `scraper.go`

```go
type ScrapedPage struct {
    URL     string
    Content string
    Err     error
}

func Scrape(ctx context.Context, urls []string) []ScrapedPage
```

Key details:
- Spawn one goroutine per URL, coordinated by `sync.WaitGroup`.
- Each goroutine uses `http.Client{Timeout: 3 * time.Second}`.
- Feed HTML body into `go-readability` to extract article text.
- Return all results (including per-URL errors for logging).

### Phase 5 — Engine (`internal/engine/`)

**File:** `engine.go`

```go
type Engine struct {
    cache  *cache.Cache
    config Config
}

type Config struct {
    SearchEngine string        // "google" or "duckduckgo"
    RateLimit    time.Duration // delay between outgoing requests
}

func New(cache *cache.Cache, cfg Config) *Engine

func (e *Engine) Search(ctx context.Context, query string, count int, force bool) (string, error)
func (e *Engine) ClearCache(query string) error
```

Key details:
- `Search` implements the full flow: hash → cache check → search (scrape) → scrape pages → consolidate → upsert → return.
- **Consolidation format:** each page's text is preceded by a `## <source URL>` header, separated by `---`.
  ```
  ## https://example.com/article-1
  
  <extracted text>
  
  ---
  
  ## https://example.com/article-2
  
  <extracted text>
  ```
- Query hash: `sha256(strings.TrimSpace(strings.ToLower(query)))`.
- Applies rate limiting between outgoing requests (configurable via `GLSI_RATE_LIMIT`).

### Phase 6 — CLI (`cmd/glsi/main.go`)

```go
func main() {
    // Subcommands: "search", "serve", "mcp"
    // search: -q/--query, -n/--count, -f/--force
    // serve:  -p/--port (default 8080), starts HTTP API server
    // mcp:    no extra flags, starts MCP stdio server
}
```

Key details:
- Use Go's standard `flag` package + manual subcommand dispatch (no external CLI lib needed for three commands).
- `search` → call `engine.Search()`, print result to stdout.
- `serve` → call `api.ListenAndServe()` (blocking).
- `mcp` → call `mcp.Serve()` (blocking).
- Exit code 1 on error, message to stderr.

### Phase 7 — HTTP API (`internal/api/`)

**File:** `server.go`

```go
func ListenAndServe(addr string, engine *engine.Engine) error
```

**Endpoints:**

| Method | Path | Description |
|---|---|---|
| `GET` | `/search` | Search + scrape + cache. Query params: `q` (required), `count` (optional, default 5), `force` (optional, default false). |
| `DELETE` | `/cache` | Clear cache. Query param: `q` (optional — if omitted, flush all). |
| `GET` | `/health` | Health check — returns `{"status": "ok"}`. |

Key details:
- Uses Go's `net/http` standard library (no framework needed).
- JSON responses with appropriate status codes.
- `Content-Type: application/json` for all responses.
- Delegates to `engine.Search()` and `engine.ClearCache()`.
- Default port: `8080`, configurable via `-p`/`--port` flag or `GLSI_PORT` env var.

**Example usage:**
```bash
# Start the server
glsi serve -p 3000

# Search
curl "http://localhost:3000/search?q=golang+concurrency&count=3"

# Force fresh scrape
curl "http://localhost:3000/search?q=golang+concurrency&force=true"

# Clear specific cache entry
curl -X DELETE "http://localhost:3000/cache?q=golang+concurrency"

# Flush all cache
curl -X DELETE "http://localhost:3000/cache"
```

### Phase 8 — MCP Server (`internal/mcp/`)

**File:** `server.go`

```go
func Serve(engine *engine.Engine) error
```

Key details:
- Use `github.com/modelcontextprotocol/go-sdk/mcp` to create a stdio server.
- Register two tools:
  - `web_search` → params: `query` (string, required), `count` (int, optional, default 5), `force` (bool, optional, default false). Calls `engine.Search()`.
  - `clear_cache` → params: `query` (string, optional). Calls `engine.ClearCache()`.
- All logging goes to stderr (stdout is the stdio JSON-RPC channel).

### Phase 9 — Testing & Polish

- [x] Unit tests for each package (cache, search, scraper, engine, api).
- [x] Mock HTTP responses in search and scraper tests.
- [x] Test API endpoints with `httptest`.
- [x] Integration test: full search→scrape→cache round-trip.
- [x] Error handling audit — consistent wrapping with `fmt.Errorf("...: %w", err)`.
- [x] README with build, usage, and env-var documentation.


---

## Dependencies

| Package | Purpose | CGO? |
|---|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite driver | No |
| `github.com/PuerkitoBio/goquery` | jQuery-like HTML parsing & CSS selectors (search result extraction) | No |
| `github.com/go-shiori/go-readability` | HTML → readable text extraction (page content) | No |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP SDK (stdio server) | No |

All dependencies are pure Go — **no CGO required**, which simplifies cross-compilation.

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `GLSI_SEARCH_ENGINE` | No | Search engine to scrape: `google` (default) or `duckduckgo` |
| `GLSI_RATE_LIMIT` | No | Delay between outgoing HTTP requests, e.g. `500ms`, `1s` (default: `1s`) |
| `GLSI_DB_PATH` | No | Override default cache DB path (`~/.glsi/cache.db`) |
| `GLSI_PORT` | No | Default port for the HTTP API server (default: `8080`) |
