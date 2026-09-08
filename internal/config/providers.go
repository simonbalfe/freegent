package config

import (
	"os"
	"strings"
)

type Providers struct {
	ModelProvider    string
	OpenRouterAPIKey string
	OpenRouterModel  string
	CodexModel       string
	CodexAuthFile    string
	OpenExtractURL   string
	SerperAPIKey     string
	ExaAPIKey        string
	TavilyAPIKey     string
	ApifyAPIToken    string
}

func LoadProviders() Providers {
	return Providers{
		ModelProvider:    strings.ToLower(envOr("FREEGENT_MODEL_PROVIDER", "openrouter")),
		OpenRouterAPIKey: strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		OpenRouterModel:  envOr("OPENROUTER_MODEL", "deepseek/deepseek-v4-flash"),
		CodexModel:       envOr("CODEX_MODEL", "gpt-5.6-sol"),
		CodexAuthFile:    strings.TrimSpace(os.Getenv("FREEGENT_CODEX_AUTH_FILE")),
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
