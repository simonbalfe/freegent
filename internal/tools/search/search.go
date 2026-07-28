package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
)

type Tool struct{}

func (Tool) Name() string { return "web_search" }

func (Tool) Description() string {
	return "Search the live web through Serper, Exa, then Tavily. Search first to discover authoritative URLs."
}

func (Tool) Schema() map[string]any {
	return objectSchema(map[string]any{"query": map[string]any{"type": "string"}, "max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 8}}, []string{"query"})
}

type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type searchProvider struct {
	name string
	key  string
	run  func(context.Context, string, string, int) ([]SearchHit, error)
}

func (Tool) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	query := strings.TrimSpace(stringValue(input["query"]))
	if query == "" {
		return agent.ToolResult{}, errors.New("search query cannot be empty")
	}
	limit := boundedInt(input["max_results"], 5, 1, 8)
	providers := []searchProvider{
		{name: "serper", key: os.Getenv("SERPER_API_KEY"), run: serperSearch},
		{name: "exa", key: os.Getenv("EXA_API_KEY"), run: exaSearch},
		{name: "tavily", key: os.Getenv("TAVILY_API_KEY"), run: tavilySearch},
	}
	return runSearchLadder(ctx, providers, query, limit)
}

func runSearchLadder(ctx context.Context, providers []searchProvider, query string, limit int) (agent.ToolResult, error) {
	attempts := []agent.FetchAttempt{}
	var lastError error
	sawEmpty := false
	for _, provider := range providers {
		if provider.key == "" {
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "skipped", Detail: "not configured"})
			continue
		}
		started := time.Now()
		results, err := provider.run(ctx, provider.key, query, limit)
		if err != nil {
			lastError = err
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "error", DurationMS: time.Since(started).Milliseconds(), Detail: conciseError(err)})
			continue
		}
		if len(results) == 0 {
			sawEmpty = true
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "empty", DurationMS: time.Since(started).Milliseconds()})
			continue
		}
		attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "ok", DurationMS: time.Since(started).Milliseconds(), Detail: fmt.Sprintf("%d results", len(results))})
		urls := make([]string, 0, len(results))
		for _, result := range results {
			urls = append(urls, result.URL)
		}
		encoded, err := json.Marshal(results)
		return agent.ToolResult{Text: string(encoded), URLs: urls, SeenURLs: urls, Provider: provider.name, Attempts: attempts}, err
	}
	if sawEmpty {
		return agent.ToolResult{Text: "[]", Attempts: attempts}, nil
	}
	if lastError != nil {
		return agent.ToolResult{}, lastError
	}
	return agent.ToolResult{}, errors.New("no search provider configured: set SERPER_API_KEY, EXA_API_KEY, or TAVILY_API_KEY")
}

func serperSearch(ctx context.Context, apiKey, query string, limit int) ([]SearchHit, error) {
	var payload struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic"`
	}
	if err := postJSON(ctx, "https://google.serper.dev/search", map[string]string{"X-API-KEY": apiKey}, map[string]any{"q": query, "num": limit}, &payload); err != nil {
		return nil, err
	}
	results := make([]SearchHit, 0, min(limit, len(payload.Organic)))
	for _, item := range payload.Organic[:min(limit, len(payload.Organic))] {
		results = append(results, SearchHit{Title: item.Title, URL: item.Link, Content: item.Snippet})
	}
	return results, nil
}

func exaSearch(ctx context.Context, apiKey, query string, limit int) ([]SearchHit, error) {
	var payload struct {
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
			Text  string `json:"text"`
		} `json:"results"`
	}
	body := map[string]any{"query": query, "type": "auto", "numResults": limit, "contents": map[string]any{"text": map[string]any{"maxCharacters": 1200}}}
	if err := postJSON(ctx, "https://api.exa.ai/search", map[string]string{"x-api-key": apiKey}, body, &payload); err != nil {
		return nil, err
	}
	results := make([]SearchHit, 0, min(limit, len(payload.Results)))
	for _, item := range payload.Results[:min(limit, len(payload.Results))] {
		results = append(results, SearchHit{Title: item.Title, URL: item.URL, Content: item.Text})
	}
	return results, nil
}

func tavilySearch(ctx context.Context, apiKey, query string, limit int) ([]SearchHit, error) {
	var payload struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	body := map[string]any{"api_key": apiKey, "query": query, "max_results": limit}
	if err := postJSON(ctx, "https://api.tavily.com/search", nil, body, &payload); err != nil {
		return nil, err
	}
	results := make([]SearchHit, 0, min(limit, len(payload.Results)))
	for _, item := range payload.Results[:min(limit, len(payload.Results))] {
		results = append(results, SearchHit{Title: item.Title, URL: item.URL, Content: item.Content})
	}
	return results, nil
}

func postJSON(ctx context.Context, endpoint string, headers map[string]string, body any, output any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s", endpoint, response.Status, string(data))
	}
	return json.Unmarshal(data, output)
}

func conciseError(err error) string {
	value := err.Error()
	if len(value) > 160 {
		return value[:160] + "…"
	}
	return value
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
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
