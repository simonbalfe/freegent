package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
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
	action := Action{Instructions: "Return a score.", Template: "Score this.", Schema: schema.Canonical, Validator: schema}
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
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Schema: schema.Canonical, Validator: schema}, Row{})
	if err == nil {
		t.Fatal("expected run failure")
	}
	if len(result.Evidence) != 1 || len(result.Steps) != 1 || result.Tokens.Input != 10 || len(result.Sources) != 1 {
		t.Fatalf("expected preserved partial trace: %+v", result)
	}
}

type fabricatedURLModel struct {
	calls    int
	feedback string
}

func (m *fabricatedURLModel) Next(_ context.Context, messages []Message, _ Action) (ModelResponse, error) {
	m.calls++
	if m.calls == 1 {
		return ModelResponse{ToolCalls: []ToolCall{{ID: "one", Name: "fetch_page", Input: map[string]any{"url": "https://www.canva.com/about/"}}}}, nil
	}
	m.feedback = messages[len(messages)-1].Content
	return ModelResponse{Final: map[string]any{"name": "Canva"}}, nil
}

func (*fabricatedURLModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentReturnsUnverifiedURLRejectionToModel(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	model := &fabricatedURLModel{}
	result, err := (Agent{
		Model:    model,
		Tools:    map[string]Tool{"fetch_page": stubTool{name: "fetch_page"}},
		MaxSteps: 1,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Schema: schema.Canonical, Validator: schema}, Row{"domain": "canva.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer["name"] != "Canva" || len(result.Steps) != 2 || result.Steps[0].Kind != "tool_error" || result.Steps[0].Error == "" {
		t.Fatalf("unexpected recovered result: %+v", result)
	}
	if !strings.Contains(model.feedback, "is unverified") || !strings.Contains(model.feedback, "Continue the task") {
		t.Fatalf("model did not receive actionable rejection feedback: %q", model.feedback)
	}
}

func TestResearchPromptExplainsURLRecovery(t *testing.T) {
	prompt := ResearchInstructions("", `{"name":"string?"}`)
	for _, expected := range []string{
		"A company homepage in the row does not verify guessed paths",
		"pass the exact company name",
		"Never stop solely because one tool call failed",
		"After an unverified URL rejection, search for that exact page or entity",
		"request every independent tool call",
		"call submit_answer by itself",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("research prompt does not contain %q", expected)
		}
	}
}

type blockingTool struct {
	name    string
	started chan<- string
	release <-chan struct{}
}

func (t blockingTool) Name() string         { return t.name }
func (blockingTool) Description() string    { return "test tool" }
func (blockingTool) Schema() map[string]any { return map[string]any{} }
func (t blockingTool) Run(context.Context, map[string]any) (ToolResult, error) {
	t.started <- t.name
	<-t.release
	return ToolResult{Text: t.name + " evidence"}, nil
}

type batchedSubmitModel struct {
	calls int
}

func (m *batchedSubmitModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	m.calls++
	if m.calls == 1 {
		return ModelResponse{ToolCalls: []ToolCall{
			{ID: "one", Name: "first", Input: map[string]any{"query": "one"}},
			{ID: "two", Name: "second", Input: map[string]any{"query": "two"}},
		}}, nil
	}
	return ModelResponse{ToolCalls: []ToolCall{{
		ID:   "answer",
		Name: submitAnswerTool,
		Input: map[string]any{
			"answer":    map[string]any{"name": "Acme"},
			"reasoning": "Both sources agree.",
		},
	}}}, nil
}

func (*batchedSubmitModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, context.Canceled
}

func TestAgentRunsBatchedToolsConcurrentlyAndAcceptsSubmittedAnswer(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"name":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	model := &batchedSubmitModel{}
	resultChannel := make(chan RunResult, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, runError := (Agent{
			Model: model,
			Tools: map[string]Tool{
				"first":  blockingTool{name: "first", started: started, release: release},
				"second": blockingTool{name: "second", started: started, release: release},
			},
			MaxSteps: 2,
		}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Schema: schema.Canonical, Validator: schema}, Row{})
		resultChannel <- result
		errorChannel <- runError
	}()

	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("expected both tools to start before either was released")
		}
	}
	close(release)
	released = true
	result := <-resultChannel
	if err := <-errorChannel; err != nil {
		t.Fatal(err)
	}
	if model.calls != 2 || result.Answer["name"] != "Acme" || result.Reasoning != "Both sources agree." {
		t.Fatalf("unexpected submitted result: %+v", result)
	}
	if len(result.Steps) != 3 || result.Steps[2].Kind != "answer" {
		t.Fatalf("expected two tool steps and one answer step: %+v", result.Steps)
	}
}

func TestSubmittedAnswerMustMatchSchema(t *testing.T) {
	schema, err := CompileOutputSchema(json.RawMessage(`{"score":"number"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = submittedAnswer(map[string]any{
		"answer":    map[string]any{"score": "wrong"},
		"reasoning": "unsupported",
	}, schema)
	if err == nil {
		t.Fatal("expected invalid submitted answer to fail")
	}
}
