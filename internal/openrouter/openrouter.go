package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/simonbalfe/freegent/internal/agent"
)

type Tool = agent.Tool
type Message = agent.Message
type Action = agent.Action
type ModelResponse = agent.ModelResponse
type Evidence = agent.Evidence
type TokenUsage = agent.TokenUsage
type ToolCall = agent.ToolCall

type OpenRouterModel struct {
	APIKey          string
	Model           string
	Client          *http.Client
	Tools           []Tool
	MaxOutputTokens int
}

func (m OpenRouterModel) Next(ctx context.Context, messages []Message, action Action) (ModelResponse, error) {
	return m.chat(ctx, messages, action, true)
}

func (m OpenRouterModel) Finalize(ctx context.Context, task string, action Action, evidence []Evidence) (ModelResponse, error) {
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return ModelResponse{}, err
	}
	messages := []Message{
		{Role: "system", Content: firstNonEmpty(action.FinalizerInstructions, action.Instructions) + "\nReturn a JSON object with exactly two fields: answer and reasoning. Answer schema: " + string(action.Validator.Canonical)},
		{Role: "user", Content: task + "\n\nEvidence:\n" + string(encodedEvidence)},
	}
	return m.chat(ctx, messages, action, false)
}

func (m OpenRouterModel) chat(ctx context.Context, messages []Message, action Action, enableTools bool) (ModelResponse, error) {
	requestMessages, err := openRouterMessages(messages)
	if err != nil {
		return ModelResponse{}, err
	}
	body := map[string]any{
		"model":       m.Model,
		"messages":    requestMessages,
		"temperature": 0,
		"max_tokens":  m.outputTokenLimit(enableTools),
	}
	if enableTools {
		body["tools"] = toolDefinitions(m.Tools)
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = false
	} else {
		body["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "freegent_answer",
				"strict": true,
				"schema": answerEnvelopeSchema(action.Validator.Document),
			},
		}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return ModelResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return ModelResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+m.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("HTTP-Referer", "https://github.com/simonbalfe/freegent")
	response, err := m.Client.Do(request)
	if err != nil {
		return ModelResponse{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return ModelResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ModelResponse{}, fmt.Errorf("OpenRouter %s: %s", response.Status, string(data))
	}
	return parseOpenRouterResponse(data)
}

func (m OpenRouterModel) outputTokenLimit(enableTools bool) int {
	limit := m.MaxOutputTokens
	if limit < 1 {
		limit = 1500
	}
	if !enableTools && limit < 4000 {
		return 4000
	}
	return limit
}

func openRouterMessages(messages []Message) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		entry := map[string]any{"role": message.Role}
		if message.Content != "" {
			entry["content"] = message.Content
		}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				arguments, err := json.Marshal(call.Input)
				if err != nil {
					return nil, err
				}
				calls = append(calls, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": string(arguments)}})
			}
			entry["tool_calls"] = calls
		}
		if message.ToolCall != nil {
			entry["tool_call_id"] = message.ToolCall.ID
			entry["name"] = message.ToolCall.Name
		}
		result = append(result, entry)
	}
	return result, nil
}

func parseOpenRouterResponse(data []byte) (ModelResponse, error) {
	var payload struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ModelResponse{}, err
	}
	if len(payload.Choices) == 0 {
		return ModelResponse{}, errors.New("OpenRouter returned no choices")
	}
	message := payload.Choices[0].Message
	usage := TokenUsage{Input: payload.Usage.PromptTokens, Output: payload.Usage.CompletionTokens}
	if len(message.ToolCalls) > 0 {
		calls := make([]ToolCall, 0, len(message.ToolCalls))
		for _, raw := range message.ToolCalls {
			input := map[string]any{}
			if err := json.Unmarshal([]byte(raw.Function.Arguments), &input); err != nil {
				return ModelResponse{}, fmt.Errorf("invalid %s arguments: %w", raw.Function.Name, err)
			}
			calls = append(calls, ToolCall{ID: raw.ID, Name: raw.Function.Name, Input: input})
		}
		return ModelResponse{ToolCalls: calls, Usage: usage}, nil
	}
	answer, err := parseJSONObject(message.Content)
	if err != nil {
		return ModelResponse{Usage: usage}, nil
	}
	reasoning, _ := answer["reasoning"].(string)
	if nested, ok := answer["answer"].(map[string]any); ok {
		answer = nested
	}
	return ModelResponse{Final: answer, Reasoning: reasoning, Usage: usage}, nil
}

func answerEnvelopeSchema(answer map[string]any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer":    answer,
			"reasoning": map[string]any{"type": "string"},
		},
		"required":             []any{"answer", "reasoning"},
		"additionalProperties": false,
	}
}

func toolDefinitions(tools []Tool) []map[string]any {
	definitions := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		definitions = append(definitions, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name(), "description": tool.Description(), "parameters": tool.Schema()}})
	}
	return definitions
}

func parseJSONObject(value string) (map[string]any, error) {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(strings.TrimSpace(cleaned), "```")
	answer := map[string]any{}
	if err := json.Unmarshal([]byte(cleaned), &answer); err != nil {
		return nil, err
	}
	return answer, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
