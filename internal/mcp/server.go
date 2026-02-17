package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/user/glsi/internal/engine"
)

// webSearchInput defines the parameters for the web_search tool.
type webSearchInput struct {
	Query string `json:"query" jsonschema:"description=The search query string"`
	Count int    `json:"count" jsonschema:"description=Number of results to scrape (default 5)"`
	Force bool   `json:"force" jsonschema:"description=Bypass cache and force a fresh scrape"`
}

// clearCacheInput defines the parameters for the clear_cache tool.
type clearCacheInput struct {
	Query string `json:"query" jsonschema:"description=Specific query to evict from cache. If omitted all entries are flushed."`
}

// empty output — we return everything via CallToolResult text content.
type emptyOutput struct{}

// Serve starts the MCP stdio server, registering tools that delegate to the
// provided engine. It blocks until the client disconnects.
func Serve(eng *engine.Engine) error {
	server := gomcp.NewServer(
		&gomcp.Implementation{
			Name:    "glsi",
			Version: "v1.0.0",
		},
		nil,
	)

	// Register web_search tool.
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "web_search",
		Description: "Search the web for a query, scrape the top result pages, and return consolidated text. Results are cached for 24 hours.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input webSearchInput) (*gomcp.CallToolResult, emptyOutput, error) {
		count := input.Count
		if count <= 0 {
			count = 5
		}

		result, err := eng.Search(ctx, input.Query, count, input.Force)
		if err != nil {
			return &gomcp.CallToolResult{
				IsError: true,
				Content: []gomcp.Content{
					&gomcp.TextContent{Text: fmt.Sprintf("search failed: %v", err)},
				},
			}, emptyOutput{}, nil
		}

		meta := fmt.Sprintf("[results: %d, from_cache: %v]\n\n", result.ResultCount, result.FromCache)
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				&gomcp.TextContent{Text: meta + result.Content},
			},
		}, emptyOutput{}, nil
	})

	// Register clear_cache tool.
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "clear_cache",
		Description: "Clear cached search results. If a query is provided, only that entry is evicted; otherwise all entries are flushed.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, input clearCacheInput) (*gomcp.CallToolResult, emptyOutput, error) {
		if err := eng.ClearCache(input.Query); err != nil {
			return &gomcp.CallToolResult{
				IsError: true,
				Content: []gomcp.Content{
					&gomcp.TextContent{Text: fmt.Sprintf("clear cache failed: %v", err)},
				},
			}, emptyOutput{}, nil
		}

		msg := "all cache entries cleared"
		if input.Query != "" {
			msg = fmt.Sprintf("cache entry for %q cleared", input.Query)
		}
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				&gomcp.TextContent{Text: msg},
			},
		}, emptyOutput{}, nil
	})

	// Run the server over stdio until the client disconnects.
	return server.Run(context.Background(), &gomcp.StdioTransport{})
}
