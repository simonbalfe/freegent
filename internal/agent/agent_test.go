package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type stubTool struct {
	name string
}

func (t stubTool) Name() string         { return t.name }
func (stubTool) Description() string    { return "test tool" }
func (stubTool) Schema() map[string]any { return map[string]any{} }
func (stubTool) Run(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{Text: "Acme builds workflow tools.", URLs: []string{"https://acme.example/about"}}, nil
}

type invalidThenFinalModel struct{}

func (invalidThenFinalModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{Final: map[string]any{"score": "wrong"}}, nil
}

func (invalidThenFinalModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{Final: map[string]any{"score": float64(5)}}, nil
}

func TestInvalidAgentAnswerUsesFinalizer(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"score":"number"}`))
	if err != nil {
		t.Fatal(err)
	}
	action := Action{Instructions: "Return a score.", Template: "Score this.", Validator: schema}
	result, err := (Agent{Model: invalidThenFinalModel{}, Tools: map[string]Tool{}, MaxSteps: 1}).Run(context.Background(), action, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer["score"] != float64(5) || len(result.Steps) != 1 || result.Steps[0].Kind != "finalize" {
		t.Fatalf("unexpected finalizer result: %+v", result)
	}
}

func TestURLLedgerSeedsBareDomains(t *testing.T) {
	ledger := newURLLedger(Row{"domain": "linear.app"})
	if !ledger.permits("https://linear.app") || !ledger.permits("https://www.linear.app/") {
		t.Fatal("expected bare input domain to permit its homepage")
	}
	if ledger.permits("https://linear.app/about") {
		t.Fatal("expected an unobserved deep path to remain blocked")
	}
}

type toolThenErrorModel struct {
	calls int
}

func (m *toolThenErrorModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	m.calls++
	if m.calls == 1 {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "search", Name: "web_search", Input: map[string]any{"query": "Acme"}}}, Usage: TokenUsage{Input: 10, Output: 2}}, nil
	}
	return ModelResponse{}, context.DeadlineExceeded
}

func (*toolThenErrorModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentReturnsPartialTraceOnFailure(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Agent{
		Model:    &toolThenErrorModel{},
		Tools:    map[string]Tool{"web_search": stubTool{name: "web_search"}},
		MaxSteps: 2,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err == nil {
		t.Fatal("expected run failure")
	}
	if len(result.Evidence) != 1 || len(result.Steps) != 1 || result.Tokens.Input != 10 || len(result.Sources) != 1 {
		t.Fatalf("expected preserved partial trace: %+v", result)
	}
}

type fabricatedURLModel struct{}

func (fabricatedURLModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{ToolCalls: []ToolCall{{ID: "one", Name: "linkedin_profile", Input: map[string]any{"url": "https://www.linkedin.com/in/invented"}}}}, nil
}

func (fabricatedURLModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentRejectsFabricatedEnrichmentURL(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Agent{
		Model:    fabricatedURLModel{},
		Tools:    map[string]Tool{"linkedin_profile": stubTool{name: "linkedin_profile"}},
		MaxSteps: 1,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err == nil || !strings.Contains(err.Error(), "refusing unverified URL") {
		t.Fatalf("expected provenance rejection, got %v", err)
	}
}
