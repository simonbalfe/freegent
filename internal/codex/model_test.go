package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
)

type modelTestTool struct{}

func (modelTestTool) Name() string                     { return "web_search" }
func (modelTestTool) Description() string              { return "Search the web." }
func (modelTestTool) GuardedURL(map[string]any) string { return "" }
func (modelTestTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}
}
func (modelTestTool) Run(context.Context, map[string]any) (agent.ToolResult, error) {
	return agent.ToolResult{}, nil
}

func TestModelSendsCodexResponsesRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access" || request.Header.Get("ChatGPT-Account-Id") != "account" {
			t.Errorf("authentication headers = %#v", request.Header)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Error(err)
			return
		}
		tools, _ := payload["tools"].([]any)
		if payload["model"] != "gpt-test" || payload["store"] != false || payload["stream"] != true || len(tools) != 1 {
			t.Errorf("request payload = %#v", payload)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(writer, "data: {\"type\":\"response.output_text.done\",\"text\":\"{\\\"answer\\\":\\\"ok\\\"}\"}\n\n")
		fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":2}}}\n\n")
	}))
	defer server.Close()

	auth := &TokenSource{client: server.Client(), auth: credentials{AccessToken: "access", RefreshToken: "refresh", AccountID: "account", ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	model := Model{Model: "gpt-test", Endpoint: server.URL, Client: server.Client(), Auth: auth, Tools: []agent.Tool{modelTestTool{}}}
	response, err := model.Next(context.Background(), []agent.Message{{Role: "system", Content: "Research."}, {Role: "user", Content: "Acme"}}, agent.Action{})
	if err != nil {
		t.Fatal(err)
	}
	if response.Final["answer"] != "ok" || response.Usage.Input != 5 || response.Usage.Output != 2 {
		t.Fatalf("model response = %+v", response)
	}
}

func TestResponseInputConvertsToolConversation(t *testing.T) {
	messages := []agent.Message{
		{Role: "system", Content: "Research carefully."},
		{Role: "user", Content: "Find Acme."},
		{Role: "assistant", ToolCalls: []agent.ToolCall{{ID: "call-1", Name: "web_search", Input: map[string]any{"query": "Acme"}}}, ProviderData: json.RawMessage(`[{"type":"reasoning","encrypted_content":"encrypted","summary":[]}]`)},
		{Role: "tool", ToolCall: &agent.ToolCall{ID: "call-1", Name: "web_search"}, Content: "Evidence"},
	}
	instructions, input, err := responseInput(messages)
	if err != nil {
		t.Fatal(err)
	}
	if instructions != "Research carefully." || len(input) != 4 {
		t.Fatalf("responseInput() instructions = %q, input = %#v", instructions, input)
	}
	if input[1]["type"] != "reasoning" || input[2]["type"] != "function_call" || input[3]["type"] != "function_call_output" {
		t.Fatalf("tool input items = %#v", input[1:])
	}
}

func TestParseResponseStreamToolCall(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_item.done","item":{"type":"reasoning","encrypted_content":"encrypted","summary":[]}}`,
		`data: {"type":"response.output_item.done","item":{"type":"function_call","call_id":"call-1","name":"web_search","arguments":"{\"query\":\"Acme\"}"}}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":12,"output_tokens":4}}}`,
		"data: [DONE]",
	}, "\n\n")
	response, err := parseResponseStream(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "call-1" || response.ToolCalls[0].Input["query"] != "Acme" {
		t.Fatalf("tool response = %+v", response)
	}
	if response.Usage.Input != 12 || response.Usage.Output != 4 {
		t.Fatalf("usage = %+v", response.Usage)
	}
	if !strings.Contains(string(response.ProviderData), "encrypted") {
		t.Fatalf("provider data = %s", response.ProviderData)
	}
}

func TestParseResponseStreamFinalJSON(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"{\"company\":"}`,
		`data: {"type":"response.output_text.delta","delta":"\"Acme\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":8,"output_tokens":3}}}`,
	}, "\n\n")
	response, err := parseResponseStream(strings.NewReader(stream))
	if err != nil {
		t.Fatal(err)
	}
	if response.Final["company"] != "Acme" {
		t.Fatalf("final response = %+v", response)
	}
}

func TestFinalizerOutputTokenLimit(t *testing.T) {
	model := Model{MaxOutputTokens: 1500}
	if got := model.outputTokenLimit(false); got != 8000 {
		t.Fatalf("finalizer output limit = %d, want 8000", got)
	}
}
