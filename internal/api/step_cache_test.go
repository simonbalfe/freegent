package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/simonbalfe/freegent/internal/agent"
)

type replayTestModel struct {
	nextCalls     int
	finalizeCalls int
}

func (m *replayTestModel) Next(_ context.Context, messages []agent.Message, _ agent.Action) (agent.ModelResponse, error) {
	m.nextCalls++
	if len(messages) == 2 {
		return agent.ModelResponse{
			ToolCalls: []agent.ToolCall{{ID: "search", Name: "web_search", Input: map[string]any{"query": "Acme"}}},
			Usage:     agent.TokenUsage{Input: 10, Output: 2},
		}, nil
	}
	return agent.ModelResponse{Usage: agent.TokenUsage{Input: 20, Output: 3}}, nil
}

func (m *replayTestModel) Finalize(context.Context, string, agent.Action, []agent.Evidence) (agent.ModelResponse, error) {
	m.finalizeCalls++
	return agent.ModelResponse{
		Final: map[string]any{"name": "Acme"},
		Usage: agent.TokenUsage{Input: 30, Output: 4},
	}, nil
}

type replayTestTool struct {
	calls int
}

func (*replayTestTool) Name() string        { return "web_search" }
func (*replayTestTool) Description() string { return "search the web" }
func (*replayTestTool) Schema() map[string]any {
	return map[string]any{"query": map[string]any{"type": "string"}}
}
func (t *replayTestTool) Run(context.Context, map[string]any) (agent.ToolResult, error) {
	t.calls++
	return agent.ToolResult{Text: "Acme builds workflow tools.", URLs: []string{"https://acme.example"}}, nil
}

func TestAgentReplayReusesCompletedExternalSteps(t *testing.T) {
	cache, events := memoryOperationCache()
	model := &replayTestModel{}
	tool := &replayTestTool{}
	cachedSearch := cachedTool{Tool: tool, cache: cache}
	cachedAgentModel := newCachedModel(model, cache, "test-model", 1000, []agent.Tool{cachedSearch})
	schema, err := agent.CompileOutputSchema(json.RawMessage(`{"name":"string"}`))
	if err != nil {
		t.Fatal(err)
	}
	action := agent.Action{Instructions: "Research.", FinalizerInstructions: "Answer.", Template: "Research Acme.", Validator: schema}
	runner := agent.Agent{Model: cachedAgentModel, Tools: map[string]agent.Tool{"web_search": cachedSearch}, MaxSteps: 3}

	first, err := runner.Run(context.Background(), action, agent.Row{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Run(context.Background(), action, agent.Row{})
	if err != nil {
		t.Fatal(err)
	}
	if model.nextCalls != 2 || model.finalizeCalls != 1 || tool.calls != 1 {
		t.Fatalf("provider calls after replay: next=%d finalize=%d tool=%d", model.nextCalls, model.finalizeCalls, tool.calls)
	}
	if len(*events) != 4 {
		t.Fatalf("replay events: got %d, want 4", len(*events))
	}
	if first.Answer["name"] != second.Answer["name"] || len(first.Evidence) != len(second.Evidence) || first.Tokens != second.Tokens {
		t.Fatalf("replayed result changed: first=%+v second=%+v", first, second)
	}
	firstKey, err := operationStepKey("tool.web_search", map[string]any{"company": "Acme", "domain": "acme.example"})
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := operationStepKey("tool.web_search", map[string]any{"domain": "acme.example", "company": "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if firstKey != secondKey {
		t.Fatalf("step key changed with map order: %s != %s", firstKey, secondKey)
	}
}

func TestOperationCacheDoesNotStoreErrors(t *testing.T) {
	cache, _ := memoryOperationCache()
	calls := 0
	var output struct {
		Value string `json:"value"`
	}
	call := func() error {
		calls++
		if calls == 1 {
			return errors.New("temporary failure")
		}
		output.Value = "done"
		return nil
	}
	if err := cache.run(context.Background(), "test", "input", &output, call); err == nil {
		t.Fatal("expected first call to fail")
	}
	if err := cache.run(context.Background(), "test", "input", &output, call); err != nil {
		t.Fatal(err)
	}
	if err := cache.run(context.Background(), "test", "input", &output, call); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || output.Value != "done" {
		t.Fatalf("error caching behavior: calls=%d output=%+v", calls, output)
	}
}

func memoryOperationCache() (operationCache, *[]AgentEvent) {
	values := map[string]json.RawMessage{}
	events := []AgentEvent{}
	return operationCache{
		load: func(_ context.Context, key string) (json.RawMessage, bool, error) {
			value, ok := values[key]
			return append(json.RawMessage(nil), value...), ok, nil
		},
		commit: func(_ context.Context, key string, value json.RawMessage) (json.RawMessage, error) {
			if stored, ok := values[key]; ok {
				return append(json.RawMessage(nil), stored...), nil
			}
			values[key] = append(json.RawMessage(nil), value...)
			return append(json.RawMessage(nil), value...), nil
		},
		event: func(event AgentEvent) {
			events = append(events, event)
		},
	}, &events
}
