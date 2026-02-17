# Go Local Search Indexer (GLSI)

**Status:** Draft

---

## Overview

Go tool that searches the web for a given query, concurrently scrapes the top N result pages, and caches the consolidated text locally in SQLite for 24 hours. Triple interface: a **CLI** for manual use, an **HTTP API** for programmatic access, and an **MCP server** (stdio transport) for AI assistant integration.

---

## Functional Requirements

### Search & Scrape

- Accepts a `query` string and result count `n` (default: 5).
- Fetches URLs by scraping a search engine (Google / DuckDuckGo) directly — no paid API required.
- Concurrently scrapes `n` URLs via goroutines; skips any URL that takes > 3s.
- Extracts main readable content (strips HTML, scripts, styles, nav chrome) using `go-readability`.

### Cache Layer

- **Store:** SQLite, single table.
- **Schema:**
  | Column | Type | Note |
  |---|---|---|
  | `query_hash` | TEXT PK | SHA-256 of the query string |
  | `content` | TEXT | Combined scraped text |
  | `updated_at` | DATETIME | Last scrape timestamp |
- **Hit:** `query_hash` exists, `updated_at` < 24h old, `force` not set → return `content`.
- **Miss:** Otherwise → scrape, upsert, return.

### MCP Interface

**Transport:** stdio (JSON-RPC over stdin/stdout).

**Tools exposed:**

| Tool | Description |
|---|---|
| `web_search` | Search + scrape + cache for a query. Returns consolidated text. |
| `clear_cache` | Evict a cached query or flush all entries. |

**`web_search` parameters:**

| Param | Type | Required | Default | Description |
|---|---|---|---|---|
| `query` | string | ✅ | — | The search string |
| `count` | integer | — | `5` | Number of results to scrape |
| `force` | boolean | — | `false` | Bypass cache, force fresh scrape |

**`clear_cache` parameters:**

| Param | Type | Required | Description |
|---|---|---|---|
| `query` | string | — | Specific query to evict. If omitted, flushes all. |

### HTTP API

| Method | Path | Description |
|---|---|---|
| `GET` | `/search` | Search + scrape + cache. Query params: `q` (required), `count` (optional, default 5), `force` (optional, default false). Returns JSON. |
| `DELETE` | `/cache` | Clear cache. Query param: `q` (optional — if omitted, flush all). |
| `GET` | `/health` | Health check — returns `{"status": "ok"}`. |

Default port: `8080` (override via `-p` flag or `GLSI_PORT` env var).

### CLI

Same binary, subcommand mode:

```
glsi search -q "golang concurrency" [-n 5] [-f]
glsi serve [-p 8080]                              # starts HTTP API server
glsi mcp                                          # starts MCP server (stdio)
```

| Flag | Description | Default |
|---|---|---|
| `-q, --query` | Search string (search mode) | *(required)* |
| `-n, --count` | Number of results (search mode) | `5` |
| `-f, --force` | Bypass cache (search mode) | `false` |
| `-p, --port` | HTTP server port (serve mode) | `8080` |

Output goes to `stdout` (pipe-friendly).

---

## Tech Stack

| Concern | Choice |
|---|---|
| Language | Go |
| MCP SDK | `github.com/modelcontextprotocol/go-sdk` (official) |
| Database | `modernc.org/sqlite` (pure Go, no CGO) |
| HTML Selection | `github.com/PuerkitoBio/goquery` (jQuery-like CSS selectors) |
| HTML Parsing | `github.com/go-shiori/go-readability` |
| Concurrency | Goroutines + `sync.WaitGroup` |