package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/openextract"
)

const (
	exaContentsEndpoint   = "https://api.exa.ai/contents"
	tavilyExtractEndpoint = "https://api.tavily.com/extract"
	maxExtractCharacters  = 12_000
)

type Tool struct {
	openExtractURL string
	exaAPIKey      string
	tavilyAPIKey   string
	client         *http.Client
	exaEndpoint    string
	tavilyEndpoint string
}

func New(openExtractURL, exaAPIKey, tavilyAPIKey string) Tool {
	return Tool{openExtractURL: openExtractURL, exaAPIKey: exaAPIKey, tavilyAPIKey: tavilyAPIKey}
}

type fallbackProvider struct {
	name string
	key  string
	run  func(context.Context, string, string) (string, error)
}

func (Tool) Name() string { return "fetch_page" }

func (Tool) Description() string {
	return "Fetch and clean one URL discovered by web_search or supplied in the input row. OpenExtract runs first, with managed extraction providers used automatically if it fails."
}

func (Tool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"url": map[string]any{"type": "string", "format": "uri"}},
		"required":             []string{"url"},
		"additionalProperties": false,
	}
}

func (tool Tool) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	rawURL := strings.TrimSpace(stringValue(input["url"]))
	target, err := url.ParseRequestURI(rawURL)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return agent.ToolResult{}, errors.New("URL must use http or https")
	}
	rawURL = target.String()
	started := time.Now()
	result, extractErr := openextract.Extract(ctx, rawURL, openextract.Config{BaseURL: tool.openExtractURL})
	if extractErr == nil {
		return agent.ToolResult{
			Text:     result.Text,
			URLs:     []string{rawURL},
			SeenURLs: result.Links,
			Provider: result.Provider,
			Attempts: result.Attempts,
		}, nil
	}
	if ctx.Err() != nil {
		return agent.ToolResult{}, ctx.Err()
	}
	attempts := append([]agent.FetchAttempt{}, result.Attempts...)
	if len(result.Attempts) == 0 {
		attempts = append(attempts, agent.FetchAttempt{Provider: "openextract", Outcome: "error", DurationMS: time.Since(started).Milliseconds(), Detail: conciseError(extractErr)})
	}

	client := tool.client
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	exaEndpoint := tool.exaEndpoint
	if exaEndpoint == "" {
		exaEndpoint = exaContentsEndpoint
	}
	tavilyEndpoint := tool.tavilyEndpoint
	if tavilyEndpoint == "" {
		tavilyEndpoint = tavilyExtractEndpoint
	}
	fallback, fallbackErr := runFallbacks(ctx, rawURL, attempts, []fallbackProvider{
		{name: "exa", key: tool.exaAPIKey, run: func(ctx context.Context, key, rawURL string) (string, error) {
			return extractWithExa(ctx, client, exaEndpoint, key, rawURL)
		}},
		{name: "tavily", key: tool.tavilyAPIKey, run: func(ctx context.Context, key, rawURL string) (string, error) {
			return extractWithTavily(ctx, client, tavilyEndpoint, key, rawURL)
		}},
	})
	if fallbackErr != nil {
		return agent.ToolResult{}, fmt.Errorf("%v; managed extraction fallbacks failed: %w", extractErr, fallbackErr)
	}
	return fallback, nil
}

func runFallbacks(ctx context.Context, rawURL string, attempts []agent.FetchAttempt, providers []fallbackProvider) (agent.ToolResult, error) {
	var lastErr error
	for _, provider := range providers {
		if provider.key == "" {
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "skipped", Detail: "not configured"})
			continue
		}
		started := time.Now()
		content, err := provider.run(ctx, provider.key, rawURL)
		if err != nil {
			if ctx.Err() != nil {
				return agent.ToolResult{}, ctx.Err()
			}
			lastErr = fmt.Errorf("%s: %w", provider.name, err)
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "error", DurationMS: time.Since(started).Milliseconds(), Detail: conciseError(err)})
			continue
		}
		content = bounded(content)
		if content == "" {
			lastErr = fmt.Errorf("%s returned no content", provider.name)
			attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "empty", DurationMS: time.Since(started).Milliseconds()})
			continue
		}
		attempts = append(attempts, agent.FetchAttempt{Provider: provider.name, Outcome: "ok", DurationMS: time.Since(started).Milliseconds()})
		return agent.ToolResult{Text: content, URLs: []string{rawURL}, Provider: provider.name, Attempts: attempts}, nil
	}
	if lastErr != nil {
		return agent.ToolResult{Attempts: attempts}, lastErr
	}
	return agent.ToolResult{Attempts: attempts}, errors.New("no managed extraction fallback configured")
}

func extractWithExa(ctx context.Context, client *http.Client, endpoint, apiKey, rawURL string) (string, error) {
	var payload struct {
		Results []struct {
			Text string `json:"text"`
		} `json:"results"`
	}
	body := map[string]any{"ids": []string{rawURL}, "text": map[string]any{"maxCharacters": maxExtractCharacters}}
	if err := postJSON(ctx, client, endpoint, map[string]string{"x-api-key": apiKey}, body, &payload); err != nil {
		return "", err
	}
	if len(payload.Results) == 0 {
		return "", errors.New("Exa returned no content")
	}
	return payload.Results[0].Text, nil
}

func extractWithTavily(ctx context.Context, client *http.Client, endpoint, apiKey, rawURL string) (string, error) {
	var payload struct {
		Results []struct {
			RawContent string `json:"raw_content"`
		} `json:"results"`
	}
	body := map[string]any{"urls": []string{rawURL}, "extract_depth": "advanced", "format": "markdown"}
	if err := postJSON(ctx, client, endpoint, map[string]string{"Authorization": "Bearer " + apiKey}, body, &payload); err != nil {
		return "", err
	}
	if len(payload.Results) == 0 {
		return "", errors.New("Tavily returned no content")
	}
	return payload.Results[0].RawContent, nil
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, body, output any) error {
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
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode provider response: %w", err)
	}
	return nil
}

func bounded(content string) string {
	value := []rune(strings.TrimSpace(content))
	if len(value) <= maxExtractCharacters {
		return string(value)
	}
	return string(value[:maxExtractCharacters]) + "\n\n[truncated]"
}

func conciseError(err error) string {
	value := err.Error()
	if len(value) <= 160 {
		return value
	}
	return value[:160] + "…"
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
