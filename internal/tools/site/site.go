package site

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/pagelode"
)

func Tools(client pagelode.Client) []agent.Tool {
	return []agent.Tool{GoogleSearch{client: client}, FindEmails{client: client}, MapSite{client: client}}
}

type GoogleSearch struct{ client pagelode.Client }

func (GoogleSearch) Name() string { return "google_search" }

func (GoogleSearch) Description() string {
	return "Search Google through PageLode. Returns organic results and Google's AI Overview with its cited sources when Google shows one."
}

func (GoogleSearch) Schema() map[string]any {
	return objectSchema(map[string]any{"query": map[string]any{"type": "string"}}, []string{"query"})
}

func (GoogleSearch) GuardedURL(map[string]any) string { return "" }

func (tool GoogleSearch) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	query := strings.TrimSpace(stringValue(input["query"]))
	if query == "" {
		return agent.ToolResult{}, errors.New("search query cannot be empty")
	}
	started := time.Now()
	result, err := tool.client.Google(ctx, query)
	if err != nil {
		return agent.ToolResult{}, err
	}
	urls := make([]string, 0, len(result.Results))
	for _, hit := range result.Results {
		urls = append(urls, hit.URL)
	}
	if result.AIOverview != nil {
		for _, source := range result.AIOverview.Sources {
			urls = append(urls, source.URL)
		}
	}
	return encoded(result, urls, urls, "pagelode-google", started)
}

type FindEmails struct{ client pagelode.Client }

func (FindEmails) Name() string { return "find_emails" }

func (FindEmails) Description() string {
	return "Crawl a website through PageLode and return the email addresses it publishes, each with the page it was found on."
}

func (FindEmails) Schema() map[string]any {
	return objectSchema(map[string]any{
		"url":       map[string]any{"type": "string", "format": "uri"},
		"max_pages": map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
	}, []string{"url"})
}

func (FindEmails) GuardedURL(input map[string]any) string { return guardedURL(input) }

func (tool FindEmails) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	started := time.Now()
	result, err := tool.client.Emails(ctx, stringValue(input["url"]), boundedInt(input["max_pages"], 20, 1, 50))
	if err != nil {
		return agent.ToolResult{}, err
	}
	seen := []string{}
	for _, email := range result.Emails {
		for _, source := range email.Sources {
			seen = append(seen, source.URL)
		}
	}
	return encoded(result, []string{result.URL}, seen, "pagelode-emails", started)
}

type MapSite struct{ client pagelode.Client }

func (MapSite) Name() string { return "map_site" }

func (MapSite) Description() string {
	return "List a website's page URLs and page templates through PageLode, from sitemaps and a shallow crawl. Use it to pick the right page before fetch_page."
}

func (MapSite) Schema() map[string]any {
	return objectSchema(map[string]any{
		"url":   map[string]any{"type": "string", "format": "uri"},
		"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 500},
	}, []string{"url"})
}

func (MapSite) GuardedURL(input map[string]any) string { return guardedURL(input) }

func (tool MapSite) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	started := time.Now()
	result, err := tool.client.Map(ctx, stringValue(input["url"]), boundedInt(input["limit"], 100, 1, 500))
	if err != nil {
		return agent.ToolResult{}, err
	}
	seen := make([]string, 0, len(result.URLs))
	for _, entry := range result.URLs {
		seen = append(seen, entry.URL)
	}
	return encoded(result, []string{result.URL}, seen, "pagelode-map", started)
}

func encoded(value any, urls, seen []string, provider string, started time.Time) (agent.ToolResult, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		Text:     string(data),
		URLs:     urls,
		SeenURLs: seen,
		Provider: provider,
		Attempts: []agent.FetchAttempt{{Provider: provider, Outcome: "ok", DurationMS: time.Since(started).Milliseconds()}},
	}, nil
}

func guardedURL(input map[string]any) string {
	value := strings.TrimSpace(stringValue(input["url"]))
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func boundedInt(value any, fallback, minimum, maximum int) int {
	number := fallback
	switch typed := value.(type) {
	case int:
		number = typed
	case int64:
		number = int(typed)
	case float64:
		number = int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			number = int(parsed)
		}
	}
	return max(minimum, min(maximum, number))
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
