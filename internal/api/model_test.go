package api

import (
	"testing"

	"github.com/simonbalfe/freegent/internal/config"
)

func TestSelectedModel(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		providers config.Providers
		wantModel string
		wantID    string
	}{
		{name: "OpenRouter default", providers: config.Providers{ModelProvider: "openrouter", OpenRouterModel: "deepseek/model"}, wantModel: "deepseek/model", wantID: "deepseek/model"},
		{name: "Codex default", providers: config.Providers{ModelProvider: "codex", CodexModel: "gpt-test"}, wantModel: "gpt-test", wantID: "codex/gpt-test"},
		{name: "Codex request override", requested: "gpt-other", providers: config.Providers{ModelProvider: "codex", CodexModel: "gpt-test"}, wantModel: "gpt-other", wantID: "codex/gpt-other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model, identity := selectedModel(test.requested, test.providers)
			if model != test.wantModel || identity != test.wantID {
				t.Fatalf("selectedModel() = %q, %q; want %q, %q", model, identity, test.wantModel, test.wantID)
			}
		})
	}
}
