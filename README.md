# GLSI — Go Local Search Indexer

A Go tool that searches the web for a given query, concurrently scrapes the top N result pages, and caches the consolidated text locally in SQLite for 24 hours.

**Triple interface:** CLI for manual use, HTTP API for programmatic access, and MCP server (stdio) for AI assistant integration.

## Quick Start

```bash
# Build
go build -o glsi ./cmd/glsi/

# Search the web
./glsi search -q "golang concurrency" -n 3

# Force a fresh scrape (bypass cache)
./glsi search -q "golang concurrency" -f

# Start the HTTP API server
./glsi serve -p 3000

# Start the MCP server (stdio transport)
./glsi mcp
```

## Installation

```bash
go install github.com/user/glsi/cmd/glsi@latest
```

Or clone and build:

```bash
git clone https://github.com/user/glsi.git
cd glsi
go build -o glsi ./cmd/glsi/
```

## CLI Usage

```
glsi <command> [flags]

Commands:
  search   Search the web, scrape pages, and return consolidated text
  serve    Start the HTTP API server
  mcp      Start the MCP stdio server
```

### `search`

| Flag | Description | Default |
|------|-------------|---------|
| `-q` | Search query (required) | — |
| `-n` | Number of results to scrape | `5` |
| `-f` | Bypass cache, force fresh scrape | `false` |

### `serve`

| Flag | Description | Default |
|------|-------------|---------|
| `-p` | HTTP server port | `8080` |

### `mcp`

No additional flags. Starts the MCP stdio server for AI assistant integration.

## HTTP API

Start the server with `glsi serve`, then use the following endpoints:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/search` | Search + scrape + cache. Query params: `q` (required), `count` (optional, default 5), `force` (optional, default false). |
| `DELETE` | `/cache` | Clear cache. Query param: `q` (optional — if omitted, flush all). |
| `GET` | `/health` | Health check — returns `{"status": "ok"}`. |

### Examples

```bash
# Search
curl "http://localhost:8080/search?q=golang+concurrency&count=3"

# Force fresh scrape
curl "http://localhost:8080/search?q=golang+concurrency&force=true"

# Clear specific cache entry
curl -X DELETE "http://localhost:8080/cache?q=golang+concurrency"

# Flush all cache
curl -X DELETE "http://localhost:8080/cache"

# Health check
curl "http://localhost:8080/health"
```

## MCP Server

GLSI exposes two MCP tools over stdio transport:

### `web_search`

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | ✅ | — | The search query |
| `count` | integer | — | `5` | Number of results to scrape |
| `force` | boolean | — | `false` | Bypass cache |

### `clear_cache`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | — | Specific query to evict. If omitted, flushes all. |

### MCP Configuration

Add to your MCP client configuration (e.g., Claude Desktop, Cursor):

```json
{
  "mcpServers": {
    "glsi": {
      "command": "glsi",
      "args": ["mcp"]
    }
  }
}
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `GLSI_SEARCH_ENGINE` | No | Search engine to scrape: `google` (default) or `duckduckgo` |
| `GLSI_RATE_LIMIT` | No | Delay between outgoing HTTP requests, e.g. `500ms`, `1s` (default: `1s`) |
| `GLSI_DB_PATH` | No | Override default cache DB path (`~/.glsi/cache.db`) |
| `GLSI_PORT` | No | Default port for the HTTP API server (default: `8080`) |

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

## Dependencies

All dependencies are pure Go — **no CGO required**.

| Package | Purpose |
|---------|---------|
| `modernc.org/sqlite` | Pure-Go SQLite driver |
| `github.com/PuerkitoBio/goquery` | HTML parsing & CSS selectors |
| `github.com/go-shiori/go-readability` | HTML → readable text extraction |
| `github.com/modelcontextprotocol/go-sdk` | Official MCP SDK (stdio server) |

## Testing

```bash
go test ./... -v
```

## License

MIT
