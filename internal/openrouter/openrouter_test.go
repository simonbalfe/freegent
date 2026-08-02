package openrouter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMalformedModelJSONDefersToFinalizer(t *testing.T) {
	message := map[string]any{"content": `{"answer":{"company":"Notion"}`}
	payload := map[string]any{
		"choices": []any{map[string]any{"message": message}},
		"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": 4, "cost": 0.0042},
	}
	data, _ := json.Marshal(payload)
	response, err := parseOpenRouterResponse(data)
	if err != nil {
		t.Fatal(err)
	}
	if response.Final != nil || response.Usage.Input != 12 || response.Usage.Output != 4 || response.CostUSD == nil || *response.CostUSD != 0.0042 {
		t.Fatalf("expected empty final response with preserved usage: %+v", response)
	}
}

func TestMalformedToolArgumentsStillFail(t *testing.T) {
	function := map[string]any{"name": "web_search", "arguments": "{bad"}
	toolCall := map[string]any{"id": "1", "function": function}
	message := map[string]any{"tool_calls": []any{toolCall}}
	payload := map[string]any{"choices": []any{map[string]any{"message": message}}, "usage": map[string]any{}}
	data, _ := json.Marshal(payload)
	_, err := parseOpenRouterResponse(data)
	if err == nil || !strings.Contains(err.Error(), "invalid web_search arguments") {
		t.Fatalf("expected malformed tool argument error, got %v", err)
	}
}
