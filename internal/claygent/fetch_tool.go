package claygent

import (
	"context"
	"errors"
	"net/url"
	"os"

	"github.com/simonbalfe/openclaygent-go/internal/openextract"
)

type FetchTool struct{}

func (FetchTool) Name() string { return "fetch_page" }

func (FetchTool) Description() string {
	return "Fetch and clean one URL discovered by web_search or supplied in the input row. Use when snippets are insufficient."
}

func (FetchTool) Schema() map[string]any {
	return objectSchema(map[string]any{"url": map[string]any{"type": "string", "format": "uri"}}, []string{"url"})
}

func (FetchTool) Run(ctx context.Context, input map[string]any) (ToolResult, error) {
	rawURL := stringValue(input["url"])
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ToolResult{}, errors.New("fetch_page URL must use http or https")
	}
	result, err := openextract.Extract(ctx, rawURL, openextract.Config{
		BaseURL: firstNonEmpty(os.Getenv("OPENEXTRACT_URL"), "http://localhost:8081"),
	})
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{
		Text:     result.Text,
		URLs:     []string{rawURL},
		SeenURLs: result.Links,
		Provider: result.Provider,
		Attempts: result.Attempts,
	}, nil
}
