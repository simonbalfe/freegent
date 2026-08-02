package toolset

import (
	"sort"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/config"
	"github.com/simonbalfe/freegent/internal/tools/apify"
	"github.com/simonbalfe/freegent/internal/tools/fetch"
	"github.com/simonbalfe/freegent/internal/tools/search"
)

func Default(providers config.Providers) map[string]agent.Tool {
	tools := map[string]agent.Tool{
		"web_search": search.New(providers.SerperAPIKey, providers.ExaAPIKey, providers.TavilyAPIKey),
		"fetch_page": fetch.New(providers.OpenExtractURL, providers.ExaAPIKey, providers.TavilyAPIKey),
	}
	if providers.ApifyAPIToken != "" {
		for _, tool := range apify.Tools(providers.ApifyAPIToken) {
			tools[tool.Name()] = tool
		}
	}
	return tools
}

func List(tools map[string]agent.Tool) []agent.Tool {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]agent.Tool, 0, len(names))
	for _, name := range names {
		values = append(values, tools[name])
	}
	return values
}
