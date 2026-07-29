package toolset

import (
	"context"
	"testing"

	"github.com/simonbalfe/freegent/internal/agent"
)

type namedTool string

func (t namedTool) Name() string         { return string(t) }
func (namedTool) Description() string    { return "" }
func (namedTool) Schema() map[string]any { return map[string]any{} }
func (namedTool) Run(context.Context, map[string]any) (agent.ToolResult, error) {
	return agent.ToolResult{}, nil
}

func TestDefaultToolsAreEnvironmentGated(t *testing.T) {
	t.Setenv("APIFY_API_TOKEN", "")
	if tools := Default(); len(tools) != 2 {
		t.Fatalf("expected web-only tools, got %d", len(tools))
	}
	t.Setenv("APIFY_API_TOKEN", "secret")
	tools := Default()
	if len(tools) != 8 {
		t.Fatalf("expected web and enrichment tools, got %d", len(tools))
	}
	for _, name := range []string{"linkedin_profile", "linkedin_posts", "linkedin_post_reactions", "linkedin_find_people", "linkedin_company", "crunchbase_company"} {
		if tools[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestSelectUsesTaskRelevantTools(t *testing.T) {
	available := map[string]agent.Tool{}
	for _, name := range []string{"web_search", "fetch_page", "linkedin_find_people", "linkedin_company", "crunchbase_company"} {
		available[name] = namedTool(name)
	}
	selected, err := Select(
		available,
		nil,
		"Find a decision maker and work email. Return linkedin_url. Also report the latest funding round.",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"web_search", "fetch_page", "linkedin_find_people", "crunchbase_company"} {
		if selected[name] == nil {
			t.Fatalf("expected selected tool %s", name)
		}
	}
	if selected["linkedin_company"] != nil {
		t.Fatal("did not expect unrelated linkedin_company tool")
	}
}

func TestSelectHonorsExplicitAllowlist(t *testing.T) {
	available := map[string]agent.Tool{
		"web_search":     namedTool("web_search"),
		"linkedin_posts": namedTool("linkedin_posts"),
	}
	selected, err := Select(available, []string{"linkedin_posts"}, "search the web")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected["linkedin_posts"] == nil {
		t.Fatalf("unexpected selected tools: %+v", selected)
	}
	if _, err := Select(available, []string{"missing"}, ""); err == nil {
		t.Fatal("expected unavailable explicit tool to fail")
	}
}
