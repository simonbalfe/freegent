package config

import "testing"

func TestLoadProviders(t *testing.T) {
	for _, name := range []string{"OPENROUTER_API_KEY", "OPENROUTER_MODEL", "OPENEXTRACT_URL", "SERPER_API_KEY", "EXA_API_KEY", "TAVILY_API_KEY", "APIFY_API_TOKEN"} {
		t.Setenv(name, "")
	}
	defaults := LoadProviders()
	if defaults.OpenRouterModel != "deepseek/deepseek-v4-flash" || defaults.OpenExtractURL != "http://localhost:8081" {
		t.Fatalf("provider defaults got model %q and OpenExtract URL %q", defaults.OpenRouterModel, defaults.OpenExtractURL)
	}

	t.Setenv("OPENROUTER_API_KEY", " router ")
	t.Setenv("OPENROUTER_MODEL", " model ")
	t.Setenv("OPENEXTRACT_URL", " http://extract:8081 ")
	t.Setenv("SERPER_API_KEY", " serper ")
	t.Setenv("EXA_API_KEY", " exa ")
	t.Setenv("TAVILY_API_KEY", " tavily ")
	t.Setenv("APIFY_API_TOKEN", " apify ")
	configured := LoadProviders()
	if configured.OpenRouterAPIKey != "router" || configured.OpenRouterModel != "model" || configured.OpenExtractURL != "http://extract:8081" || configured.SerperAPIKey != "serper" || configured.ExaAPIKey != "exa" || configured.TavilyAPIKey != "tavily" || configured.ApifyAPIToken != "apify" {
		t.Fatalf("provider configuration was not loaded and trimmed: %+v", configured)
	}
}
