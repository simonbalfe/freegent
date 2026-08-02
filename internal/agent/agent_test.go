package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type stubTool struct {
	name    string
	costUSD *float64
}

func (t stubTool) Name() string         { return t.name }
func (stubTool) Description() string    { return "test tool" }
func (stubTool) Schema() map[string]any { return map[string]any{} }
func (t stubTool) Run(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{Text: "Acme builds workflow tools.", URLs: []string{"https://acme.example/about"}, CostUSD: t.costUSD}, nil
}

type countingTool struct {
	calls int
}

func (*countingTool) Name() string           { return "web_search" }
func (*countingTool) Description() string    { return "test tool" }
func (*countingTool) Schema() map[string]any { return map[string]any{} }
func (t *countingTool) Run(context.Context, map[string]any) (ToolResult, error) {
	t.calls++
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

type invalidFinalizerThenValidModel struct {
	finalizations      int
	sawValidationError bool
}

func (*invalidFinalizerThenValidModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func (m *invalidFinalizerThenValidModel) Finalize(_ context.Context, _ string, action Action, _ []Evidence) (ModelResponse, error) {
	m.finalizations++
	if m.finalizations == 1 {
		return ModelResponse{Final: map[string]any{}}, nil
	}
	m.sawValidationError = strings.Contains(action.FinalizerInstructions, "previous answer failed schema validation")
	return ModelResponse{Final: map[string]any{"score": float64(5)}}, nil
}

func TestFinalizerRetriesInvalidAnswer(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"score":"number"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &invalidFinalizerThenValidModel{}
	result, err := (Agent{Model: model, Tools: map[string]Tool{}, MaxSteps: 1}).Run(context.Background(), Action{Instructions: "Return a score.", Template: "Score this.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.finalizations != 2 || !model.sawValidationError || result.Answer["score"] != float64(5) {
		t.Fatalf("finalizer retry failed: model=%+v result=%+v", model, result)
	}
}

type failedFinalizerThenValidModel struct {
	finalizations int
}

func (*failedFinalizerThenValidModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func (m *failedFinalizerThenValidModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	m.finalizations++
	if m.finalizations == 1 {
		return ModelResponse{}, context.DeadlineExceeded
	}
	return ModelResponse{Final: map[string]any{"score": float64(5)}}, nil
}

func TestFinalizerRetriesRequestFailure(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"score":"number"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &failedFinalizerThenValidModel{}
	result, err := (Agent{Model: model, Tools: map[string]Tool{}, MaxSteps: 1}).Run(context.Background(), Action{Instructions: "Return a score.", Template: "Score this.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.finalizations != 2 || result.Answer["score"] != float64(5) {
		t.Fatalf("finalizer request retry failed: model=%+v result=%+v", model, result)
	}
}

type invalidOutputThenValidModel struct {
	finalizations int
}

func (*invalidOutputThenValidModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func (m *invalidOutputThenValidModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	m.finalizations++
	if m.finalizations == 1 {
		return ModelResponse{OutputError: "model output reached the token limit before completing valid JSON"}, nil
	}
	return ModelResponse{Final: map[string]any{"score": float64(5)}}, nil
}

func TestFinalizerRetriesInvalidModelOutput(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"score":"number"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &invalidOutputThenValidModel{}
	result, err := (Agent{Model: model, Tools: map[string]Tool{}, MaxSteps: 1}).Run(context.Background(), Action{Instructions: "Return a score.", Template: "Score this.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.finalizations != 2 || result.Answer["score"] != float64(5) {
		t.Fatalf("invalid output retry failed: model=%+v result=%+v", model, result)
	}
}

type duplicateToolModel struct {
	calls int
}

func (m *duplicateToolModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	m.calls++
	if m.calls <= 2 {
		return ModelResponse{ToolCalls: []ToolCall{{ID: []string{"one", "two"}[m.calls-1], Name: "web_search", Input: map[string]any{"query": "Acme"}}}}, nil
	}
	return ModelResponse{Final: map[string]any{"name": "Acme"}}, nil
}

func (*duplicateToolModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentReusesIdenticalToolCall(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &duplicateToolModel{}
	tool := &countingTool{}
	result, err := (Agent{Model: model, Tools: map[string]Tool{"web_search": tool}, MaxSteps: 3}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if tool.calls != 1 || len(result.Evidence) != 1 || len(result.Steps) != 3 || result.Steps[1].Kind != "reuse" {
		t.Fatalf("identical tool call was not reused: calls=%d result=%+v", tool.calls, result)
	}
}

type toolBudgetModel struct {
	nextCalls     int
	finalizations int
}

func (m *toolBudgetModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	m.nextCalls++
	return ModelResponse{ToolCalls: []ToolCall{{ID: "search", Name: "web_search", Input: map[string]any{"query": m.nextCalls}}}}, nil
}

func (m *toolBudgetModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	m.finalizations++
	return ModelResponse{Final: map[string]any{"name": "Acme"}}, nil
}

func TestAgentFinalizesAtToolCallLimit(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &toolBudgetModel{}
	tool := &countingTool{}
	result, err := (Agent{Model: model, Tools: map[string]Tool{"web_search": tool}, MaxSteps: 10}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.nextCalls != maxSuccessfulToolCalls || model.finalizations != 1 || tool.calls != maxSuccessfulToolCalls || len(result.Evidence) != maxSuccessfulToolCalls {
		t.Fatalf("tool call limit was not enforced: model=%+v tool=%+v result=%+v", model, tool, result)
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
		cost := 0.004
		return ModelResponse{ToolCalls: []ToolCall{{ID: "search", Name: "web_search", Input: map[string]any{"query": "Acme"}}}, Usage: TokenUsage{Input: 10, Output: 2}, CostUSD: &cost}, nil
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
	toolCost := 0.02
	result, err := (Agent{
		Model:    &toolThenErrorModel{},
		Tools:    map[string]Tool{"web_search": stubTool{name: "web_search", costUSD: &toolCost}},
		MaxSteps: 2,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err == nil {
		t.Fatal("expected run failure")
	}
	if len(result.Evidence) != 1 || len(result.Steps) != 1 || result.Tokens.Input != 10 || len(result.Sources) != 1 || result.Costs.OpenRouterUSD != 0.004 || result.Costs.ApifyUSD != 0.02 {
		t.Fatalf("expected preserved partial trace: %+v", result)
	}
}

type fabricatedURLModel struct {
	calls        int
	sawRejection bool
}

func (m *fabricatedURLModel) Next(_ context.Context, messages []Message, _ Action) (ModelResponse, error) {
	m.calls++
	if m.calls == 1 {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "one", Name: "linkedin_profile", Input: map[string]any{"url": "https://www.linkedin.com/in/invented"}}}}, nil
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "was rejected") {
			m.sawRejection = true
		}
	}
	return ModelResponse{Final: map[string]any{"name": "Acme"}}, nil
}

func (*fabricatedURLModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{Final: map[string]any{"name": "Acme"}}, nil
}

func TestAgentRecoversFromFabricatedEnrichmentURL(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &fabricatedURLModel{}
	result, err := (Agent{
		Model:    model,
		Tools:    map[string]Tool{"linkedin_profile": stubTool{name: "linkedin_profile"}},
		MaxSteps: 2,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || !model.sawRejection || result.Answer["name"] != "Acme" || len(result.Evidence) != 0 || len(result.Steps) != 2 || result.Steps[0].Kind != "rejected" {
		t.Fatalf("agent did not recover from rejected URL: model=%+v result=%+v", model, result)
	}
}

type deadPageTool struct{}

func (deadPageTool) Name() string           { return "fetch_page" }
func (deadPageTool) Description() string    { return "test tool" }
func (deadPageTool) Schema() map[string]any { return map[string]any{} }
func (deadPageTool) Run(context.Context, map[string]any) (ToolResult, error) {
	return ToolResult{}, errors.New("OpenExtract could not extract the URL: HTTP 404")
}

type deadPageRecoveryModel struct {
	calls    int
	sawError bool
}

func (m *deadPageRecoveryModel) Next(_ context.Context, messages []Message, _ Action) (ModelResponse, error) {
	m.calls++
	if m.calls == 1 {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "fetch", Name: "fetch_page", Input: map[string]any{"url": "https://rhythms.example/missing"}}}}, nil
	}
	for _, message := range messages {
		if message.Role == "tool" && strings.Contains(message.Content, "HTTP 404") {
			m.sawError = true
		}
	}
	return ModelResponse{Final: map[string]any{"name": "Rhythms"}}, nil
}

func (*deadPageRecoveryModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentRecoversFromToolFailure(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &deadPageRecoveryModel{}
	result, err := (Agent{
		Model:    model,
		Tools:    map[string]Tool{"fetch_page": deadPageTool{}},
		MaxSteps: 2,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Validator: schema}, Row{"url": "https://rhythms.example/missing"})
	if err != nil {
		t.Fatal(err)
	}
	if !model.sawError || result.Answer["name"] != "Rhythms" || len(result.Steps) != 2 || result.Steps[0].Kind != "tool_error" {
		t.Fatalf("agent did not recover from tool failure: model=%+v result=%+v", model, result)
	}
}
