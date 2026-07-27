package claygent

import "context"

type DemoModel struct{}

func (DemoModel) Next(_ context.Context, messages []Message, _ Action) (ModelResponse, error) {
	for _, message := range messages {
		if message.Role == "tool" {
			return ModelResponse{Final: map[string]any{"company": "Acme", "summary": "Builds durable workflow tools for engineering teams.", "confidence": "high"}}, nil
		}
	}
	return ModelResponse{ToolCalls: []ToolCall{{ID: "demo-search", Name: "web_search", Input: map[string]any{"query": "Acme company overview"}}}}, nil
}

func (DemoModel) Finalize(_ context.Context, _ string, _ Action, evidence []Evidence) (ModelResponse, error) {
	if len(evidence) == 0 {
		return ModelResponse{Final: map[string]any{"company": nil, "summary": nil, "confidence": "low"}}, nil
	}
	return ModelResponse{Final: map[string]any{"company": "Acme", "summary": evidence[0].Text, "confidence": "medium"}}, nil
}

type DemoSearchTool struct{}

func (DemoSearchTool) Name() string           { return "web_search" }
func (DemoSearchTool) Description() string    { return "Demo search" }
func (DemoSearchTool) Schema() map[string]any { return map[string]any{} }
func (DemoSearchTool) Run(_ context.Context, _ map[string]any) (ToolResult, error) {
	return ToolResult{Text: "Acme's official site says it builds durable workflow tools for engineering teams.", URLs: []string{"https://acme.example/about"}}, nil
}
