package config

import (
	"os"
	"strings"
)

type Providers struct {
	OpenRouterAPIKey string
	OpenRouterModel  string
	OpenExtractURL   string
	SerperAPIKey     string
	ExaAPIKey        string
	TavilyAPIKey     string
	ApifyAPIToken    string
}

func LoadProviders() Providers {
	return Providers{
		OpenRouterAPIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterModel:  envOr("OPENROUTER_MODEL", "deepseek/deepseek-v4-flash"),
		OpenExtractURL:   envOr("OPENEXTRACT_URL", "http://localhost:8081"),
		SerperAPIKey:     strings.TrimSpace(os.Getenv("SERPER_API_KEY")),
		ExaAPIKey:        strings.TrimSpace(os.Getenv("EXA_API_KEY")),
		TavilyAPIKey:     strings.TrimSpace(os.Getenv("TAVILY_API_KEY")),
		ApifyAPIToken:    strings.TrimSpace(os.Getenv("APIFY_API_TOKEN")),
	}
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
