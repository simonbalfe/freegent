package codex

import (
	"bufio"
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

const responsesEndpoint = "https://chatgpt.com/backend-api/codex/responses"

type Model struct {
	Model           string
	Endpoint        string
	Client          *http.Client
	Auth            *TokenSource
	Tools           []agent.Tool
	MaxOutputTokens int
}

type responseItem struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	CallID           string          `json:"call_id"`
	Name             string          `json:"name"`
	Arguments        string          `json:"arguments"`
	EncryptedContent string          `json:"encrypted_content"`
	Summary          json.RawMessage `json:"summary"`
	Content          []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type responsePayload struct {
	Status string         `json:"status"`
	Output []responseItem `json:"output"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *streamError `json:"error"`
}

type streamEvent struct {
	Type     string           `json:"type"`
	Delta    string           `json:"delta"`
	Text     string           `json:"text"`
	Item     *responseItem    `json:"item"`
	Response *responsePayload `json:"response"`
	Error    *streamError     `json:"error"`
}

type streamError struct {
	Message string `json:"message"`
}

func (m Model) Next(ctx context.Context, messages []agent.Message, _ agent.Action) (agent.ModelResponse, error) {
	return m.respond(ctx, messages, true)
}

func (m Model) Finalize(ctx context.Context, task string, action agent.Action, evidence []agent.Evidence) (agent.ModelResponse, error) {
	encodedEvidence, err := json.Marshal(evidence)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	messages := []agent.Message{
		{Role: "system", Content: firstNonEmpty(action.FinalizerInstructions, action.Instructions) + "\nReturn only a JSON object matching this schema exactly: " + string(action.Validator.Canonical)},
		{Role: "user", Content: task + "\n\nEvidence:\n" + string(encodedEvidence)},
	}
	return m.respond(ctx, messages, false)
}

func (m Model) respond(ctx context.Context, messages []agent.Message, enableTools bool) (agent.ModelResponse, error) {
	instructions, input, err := responseInput(messages)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	body := map[string]any{
		"model":        m.Model,
		"input":        input,
		"instructions": instructions,
		"include":      []string{"reasoning.encrypted_content"},
		"reasoning":    map[string]any{"effort": "medium", "summary": "auto"},
		"store":        false,
		"stream":       true,
	}
	if limit := m.outputTokenLimit(enableTools); limit > 0 {
		body["max_output_tokens"] = limit
	}
	if enableTools {
		body["tools"] = responseTools(m.Tools)
		body["tool_choice"] = "auto"
		body["parallel_tool_calls"] = false
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	access, err := m.Auth.Access(ctx)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	endpoint := m.Endpoint
	if endpoint == "" {
		endpoint = responsesEndpoint
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return agent.ModelResponse{}, err
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+access.Token)
	if access.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", access.AccountID)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Originator", "opencode")
	request.Header.Set("User-Agent", userAgent)
	if access.Residency != "" {
		request.Header.Set("x-openai-internal-codex-residency", access.Residency)
	}
	response, err := m.Client.Do(request)
	if err != nil {
		return agent.ModelResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		err := responseError("create Codex response", response)
		if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusConflict && response.StatusCode != http.StatusTooManyRequests {
			return agent.ModelResponse{}, agent.Permanent(err)
		}
		return agent.ModelResponse{}, err
	}
	defer response.Body.Close()
	return parseResponseStream(response.Body)
}

func (m Model) outputTokenLimit(enableTools bool) int {
	limit := m.MaxOutputTokens
	if !enableTools && limit < 8000 {
		return 8000
	}
	return limit
}

func responseInput(messages []agent.Message) (string, []map[string]any, error) {
	var instructions []string
	input := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			if message.Content != "" {
				instructions = append(instructions, message.Content)
			}
		case "user", "assistant":
			if len(message.ProviderData) > 0 {
				var items []map[string]any
				if err := json.Unmarshal(message.ProviderData, &items); err != nil {
					return "", nil, fmt.Errorf("decode Codex provider data: %w", err)
				}
				input = append(input, items...)
			}
			if message.Content != "" {
				contentType := "input_text"
				if message.Role == "assistant" {
					contentType = "output_text"
				}
				input = append(input, map[string]any{"role": message.Role, "content": []map[string]any{{"type": contentType, "text": message.Content}}})
			}
			for _, call := range message.ToolCalls {
				arguments, err := json.Marshal(call.Input)
				if err != nil {
					return "", nil, err
				}
				input = append(input, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Name, "arguments": string(arguments)})
			}
		case "tool":
			if message.ToolCall == nil {
				return "", nil, errors.New("Codex tool message is missing its call")
			}
			input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCall.ID, "output": message.Content})
		default:
			return "", nil, fmt.Errorf("unsupported Codex message role %q", message.Role)
		}
	}
	return strings.Join(instructions, "\n\n"), input, nil
}

func responseTools(tools []agent.Tool) []map[string]any {
	definitions := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		definitions = append(definitions, map[string]any{"type": "function", "name": tool.Name(), "description": tool.Description(), "parameters": tool.Schema(), "strict": false})
	}
	return definitions
}

func parseResponseStream(reader io.Reader) (agent.ModelResponse, error) {
	var text strings.Builder
	calls := []agent.ToolCall{}
	reasoning := []map[string]any{}
	usage := agent.TokenUsage{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return agent.ModelResponse{}, fmt.Errorf("decode Codex stream event: %w", err)
		}
		switch event.Type {
		case "response.output_text.delta":
			text.WriteString(event.Delta)
		case "response.output_text.done":
			if text.Len() == 0 {
				text.WriteString(event.Text)
			}
		case "response.output_item.done":
			if event.Item != nil {
				switch event.Item.Type {
				case "function_call":
					call, err := parseToolCall(*event.Item)
					if err != nil {
						return agent.ModelResponse{}, err
					}
					calls = append(calls, call)
				case "reasoning":
					if item := reasoningItem(*event.Item); item != nil {
						reasoning = append(reasoning, item)
					}
				}
			}
		case "response.completed":
			if event.Response != nil {
				usage = agent.TokenUsage{Input: event.Response.Usage.InputTokens, Output: event.Response.Usage.OutputTokens}
				if len(calls) == 0 {
					var err error
					calls, err = callsFromOutput(event.Response.Output)
					if err != nil {
						return agent.ModelResponse{}, err
					}
				}
				if text.Len() == 0 {
					text.WriteString(textFromOutput(event.Response.Output))
				}
				if len(reasoning) == 0 {
					reasoning = reasoningFromOutput(event.Response.Output)
				}
			}
		case "error", "response.failed":
			if event.Error != nil && event.Error.Message != "" {
				return agent.ModelResponse{}, errors.New(event.Error.Message)
			}
			if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
				return agent.ModelResponse{}, errors.New(event.Response.Error.Message)
			}
			return agent.ModelResponse{}, fmt.Errorf("Codex response stream failed: %s", event.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.ModelResponse{}, fmt.Errorf("read Codex response stream: %w", err)
	}
	if len(calls) > 0 {
		providerData, err := json.Marshal(reasoning)
		if err != nil {
			return agent.ModelResponse{}, fmt.Errorf("encode Codex provider data: %w", err)
		}
		return agent.ModelResponse{ToolCalls: calls, Usage: usage, ProviderData: providerData}, nil
	}
	if text.Len() == 0 {
		return agent.ModelResponse{}, errors.New("Codex response contained no text or tool calls")
	}
	answer, err := parseJSONObject(text.String())
	if err != nil {
		return agent.ModelResponse{OutputError: fmt.Sprintf("model returned invalid JSON: %v", err), Usage: usage}, nil
	}
	return agent.ModelResponse{Final: answer, Usage: usage}, nil
}

func reasoningFromOutput(items []responseItem) []map[string]any {
	reasoning := []map[string]any{}
	for _, item := range items {
		if item.Type == "reasoning" {
			if value := reasoningItem(item); value != nil {
				reasoning = append(reasoning, value)
			}
		}
	}
	return reasoning
}

func reasoningItem(item responseItem) map[string]any {
	if item.EncryptedContent == "" {
		return nil
	}
	value := map[string]any{"type": "reasoning", "encrypted_content": item.EncryptedContent}
	if len(item.Summary) > 0 {
		var summary any
		if json.Unmarshal(item.Summary, &summary) == nil {
			value["summary"] = summary
		}
	}
	return value
}

func callsFromOutput(items []responseItem) ([]agent.ToolCall, error) {
	calls := []agent.ToolCall{}
	for _, item := range items {
		if item.Type != "function_call" {
			continue
		}
		call, err := parseToolCall(item)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, nil
}

func parseToolCall(item responseItem) (agent.ToolCall, error) {
	input := map[string]any{}
	if err := json.Unmarshal([]byte(item.Arguments), &input); err != nil {
		return agent.ToolCall{}, fmt.Errorf("invalid %s arguments: %w", item.Name, err)
	}
	callID := item.CallID
	if callID == "" {
		callID = item.ID
	}
	return agent.ToolCall{ID: callID, Name: item.Name, Input: input}, nil
}

func textFromOutput(items []responseItem) string {
	var text strings.Builder
	for _, item := range items {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "output_text" {
				text.WriteString(content.Text)
			}
		}
	}
	return text.String()
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
