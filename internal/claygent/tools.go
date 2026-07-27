package claygent

import (
	"os"
	"sort"
	"strings"
)

func defaultTools() map[string]Tool {
	tools := map[string]Tool{
		"web_search": SearchTool{},
		"fetch_page": FetchTool{},
	}
	if os.Getenv("APIFY_API_TOKEN") != "" {
		for _, tool := range apifyTools() {
			tools[tool.Name()] = tool
		}
	}
	return tools
}

func toolList(tools map[string]Tool) []Tool {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	values := make([]Tool, 0, len(names))
	for _, name := range names {
		values = append(values, tools[name])
	}
	return values
}

func guardedToolURL(name string, input map[string]any) string {
	var value string
	switch name {
	case "fetch_page", "linkedin_profile":
		value = stringValue(input["url"])
	case "linkedin_posts":
		value = stringValue(input["profileUrl"])
	case "linkedin_post_reactions":
		value = stringValue(input["postUrl"])
	case "linkedin_find_people", "linkedin_company", "crunchbase_company":
		value = stringValue(input["company"])
	}
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}
