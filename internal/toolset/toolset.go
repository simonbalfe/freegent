package toolset

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/tools/apify"
	"github.com/simonbalfe/freegent/internal/tools/fetch"
	"github.com/simonbalfe/freegent/internal/tools/search"
)

func Default() map[string]agent.Tool {
	tools := map[string]agent.Tool{
		"web_search": search.Tool{},
		"fetch_page": fetch.Tool{},
	}
	if os.Getenv("APIFY_API_TOKEN") != "" {
		for _, tool := range apify.Tools() {
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

func Select(tools map[string]agent.Tool, requested []string, task string) (map[string]agent.Tool, error) {
	if len(requested) > 0 {
		selected := make(map[string]agent.Tool, len(requested))
		for _, raw := range requested {
			name := strings.TrimSpace(raw)
			tool, ok := tools[name]
			if !ok {
				return nil, fmt.Errorf("unknown or unavailable tool %q", name)
			}
			selected[name] = tool
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("tools must contain at least one available tool")
		}
		return selected, nil
	}

	selected := map[string]agent.Tool{}
	addTool(selected, tools, "web_search")
	addTool(selected, tools, "fetch_page")
	value := strings.ToLower(task)
	for name, keywords := range map[string][]string{
		"linkedin_profile": {
			"linkedin profile", "linkedin_profile", "profile details", "work history", "employment history",
		},
		"linkedin_posts": {
			"linkedin post", "linkedin_post", "linkedin activity", "recent posts", "social posts",
		},
		"linkedin_post_reactions": {
			"linkedin reaction", "linkedin_reaction", "post reaction", "people who reacted", "engagers",
		},
		"linkedin_find_people": {
			"find people", "find contacts", "decision maker", "decision-maker", "buyer contact",
			"decision_maker", "contact name", "contact_name", "contact email", "contact_email",
			"work email", "work_email", "linkedin_url", "linkedin url",
		},
		"linkedin_company": {
			"employee count", "headcount", "company size", "linkedin company", "linkedin followers",
			"employee_count", "company_size", "linkedin_company", "linkedin_followers",
			"headquarters", "founded year", "founded_year",
		},
		"crunchbase_company": {
			"crunchbase", "funding round", "funding_round", "fundraising", "investors",
			"investment raised", "valuation",
		},
	} {
		if containsAny(value, keywords) {
			addTool(selected, tools, name)
		}
	}
	return selected, nil
}

func addTool(selected, available map[string]agent.Tool, name string) {
	if tool := available[name]; tool != nil {
		selected[name] = tool
	}
}

func containsAny(value string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
