package toolset

import "testing"

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
