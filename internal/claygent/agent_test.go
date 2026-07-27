package claygent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchToolCallsOpenExtract(t *testing.T) {
	openExtract := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/extract" {
			t.Fatalf("unexpected OpenExtract request: %s", request.URL.String())
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"content":  "Readable page content",
			"provider": "patchright",
			"outcome":  "ok",
			"links":    []string{"https://example.com/about"},
			"attempts": []map[string]any{{"provider": "patchright", "outcome": "ok", "durationMs": 20}},
		})
	}))
	defer openExtract.Close()

	old := os.Getenv("OPENEXTRACT_URL")
	if err := os.Setenv("OPENEXTRACT_URL", openExtract.URL); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Setenv("OPENEXTRACT_URL", old) }()

	result, err := (FetchTool{}).Run(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "patchright" || len(result.Attempts) != 1 || len(result.SeenURLs) != 1 {
		t.Fatalf("unexpected OpenExtract result: %+v", result)
	}
}

type invalidThenFinalModel struct{}

func (invalidThenFinalModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{Final: map[string]any{"score": "wrong"}}, nil
}

func (invalidThenFinalModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{Final: map[string]any{"score": float64(5)}}, nil
}

func TestInvalidAgentAnswerUsesFinalizer(t *testing.T) {
	schema, err := compileOutputSchema(json.RawMessage(`{"score":"number"}`))
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
	schema, err := compileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Agent{
		Model:    &toolThenErrorModel{},
		Tools:    map[string]Tool{"web_search": DemoSearchTool{}},
		MaxSteps: 2,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Schema: schema.Canonical, Validator: schema}, Row{})
	if err == nil {
		t.Fatal("expected run failure")
	}
	if len(result.Evidence) != 1 || len(result.Steps) != 1 || result.Tokens.Input != 10 || len(result.Sources) != 1 {
		t.Fatalf("expected preserved partial trace: %+v", result)
	}
}
