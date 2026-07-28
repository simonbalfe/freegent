package fetch

import (
	"context"
	"errors"
	"net/url"
	"os"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/openextract"
)

type Tool struct{}

func (Tool) Name() string { return "fetch_page" }

func (Tool) Description() string {
	return "Fetch and clean one URL discovered by web_search or supplied in the input row. Use when snippets are insufficient."
}

func (Tool) Schema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"url": map[string]any{"type": "string", "format": "uri"}},
		"required":             []string{"url"},
		"additionalProperties": false,
	}
}

func (Tool) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	rawURL, _ := input["url"].(string)
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return agent.ToolResult{}, errors.New("fetch_page URL must use http or https")
	}
	baseURL := os.Getenv("OPENEXTRACT_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	result, err := openextract.Extract(ctx, rawURL, openextract.Config{
		BaseURL: baseURL,
	})
	if err != nil {
		return agent.ToolResult{}, err
	}
	return agent.ToolResult{
		Text:     result.Text,
		URLs:     []string{rawURL},
		SeenURLs: result.Links,
		Provider: result.Provider,
		Attempts: result.Attempts,
	}, nil
}
