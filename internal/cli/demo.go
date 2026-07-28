package cli

import (
	"context"

	"github.com/simonbalfe/freegent/internal/agent"
)

type DemoModel struct{}

func (DemoModel) Next(_ context.Context, messages []agent.Message, _ agent.Action) (agent.ModelResponse, error) {
	for _, message := range messages {
		if message.Role == "tool" {
			return agent.ModelResponse{Final: map[string]any{"company": "Acme", "summary": "Builds durable workflow tools for engineering teams.", "confidence": "high"}}, nil
		}
	}
	return agent.ModelResponse{ToolCalls: []agent.ToolCall{{ID: "demo-search", Name: "web_search", Input: map[string]any{"query": "Acme company overview"}}}}, nil
}

func (DemoModel) Finalize(_ context.Context, _ string, _ agent.Action, evidence []agent.Evidence) (agent.ModelResponse, error) {
	if len(evidence) == 0 {
		return agent.ModelResponse{Final: map[string]any{"company": nil, "summary": nil, "confidence": "low"}}, nil
	}
	return agent.ModelResponse{Final: map[string]any{"company": "Acme", "summary": evidence[0].Text, "confidence": "medium"}}, nil
}

type DemoSearchTool struct{}

func (DemoSearchTool) Name() string           { return "web_search" }
func (DemoSearchTool) Description() string    { return "Demo search" }
func (DemoSearchTool) Schema() map[string]any { return map[string]any{} }
func (DemoSearchTool) Run(_ context.Context, _ map[string]any) (agent.ToolResult, error) {
	return agent.ToolResult{Text: "Acme's official site says it builds durable workflow tools for engineering teams.", URLs: []string{"https://acme.example/about"}}, nil
}
