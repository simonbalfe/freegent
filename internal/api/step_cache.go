package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/simonbalfe/freegent/internal/agent"
)

const operationStepVersion = "1"

type operationCache struct {
	load   func(context.Context, string) (json.RawMessage, bool, error)
	commit func(context.Context, string, json.RawMessage) (json.RawMessage, error)
	event  func(AgentEvent)
}

func newOperationCache(store *PostgresStore, args OperationArgs, event func(AgentEvent)) operationCache {
	return operationCache{
		load: func(ctx context.Context, key string) (json.RawMessage, bool, error) {
			return store.operationStep(ctx, args, key)
		},
		commit: func(ctx context.Context, key string, value json.RawMessage) (json.RawMessage, error) {
			return store.commitOperationStep(ctx, args, key, value)
		},
		event: event,
	}
}

func (c operationCache) run(ctx context.Context, kind string, input any, output any, call func() error) error {
	key, err := operationStepKey(kind, input)
	if err != nil {
		return err
	}
	raw, found, err := c.load(ctx, key)
	if err != nil {
		return err
	}
	if found {
		if err := json.Unmarshal(raw, output); err != nil {
			return fmt.Errorf("decode cached %s: %w", kind, err)
		}
		if c.event != nil {
			c.event(AgentEvent{Message: "replay reused " + kind})
		}
		return nil
	}
	if err := call(); err != nil {
		return err
	}
	raw, err = json.Marshal(output)
	if err != nil {
		return fmt.Errorf("encode %s result: %w", kind, err)
	}
	stored, err := c.commit(ctx, key, raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(stored, output); err != nil {
		return fmt.Errorf("decode committed %s: %w", kind, err)
	}
	return nil
}

func operationStepKey(kind string, input any) (string, error) {
	encoded, err := json.Marshal(struct {
		Version string `json:"version"`
		Kind    string `json:"kind"`
		Input   any    `json:"input"`
	}{Version: operationStepVersion, Kind: kind, Input: input})
	if err != nil {
		return "", fmt.Errorf("encode %s input: %w", kind, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

type cachedModel struct {
	model    agent.Model
	cache    operationCache
	identity modelIdentity
}

type modelIdentity struct {
	Name            string         `json:"name"`
	MaxOutputTokens int            `json:"maxOutputTokens"`
	Tools           []toolIdentity `json:"tools"`
}

type toolIdentity struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

type actionIdentity struct {
	Instructions          string          `json:"instructions"`
	FinalizerInstructions string          `json:"finalizerInstructions"`
	Template              string          `json:"template"`
	Schema                json.RawMessage `json:"schema"`
}

func newCachedModel(model agent.Model, cache operationCache, name string, maxOutputTokens int, tools []agent.Tool) cachedModel {
	identities := make([]toolIdentity, len(tools))
	for index, tool := range tools {
		identities[index] = toolIdentity{Name: tool.Name(), Description: tool.Description(), Schema: tool.Schema()}
	}
	return cachedModel{
		model: model,
		cache: cache,
		identity: modelIdentity{
			Name:            name,
			MaxOutputTokens: maxOutputTokens,
			Tools:           identities,
		},
	}
}

func (m cachedModel) Next(ctx context.Context, messages []agent.Message, action agent.Action) (agent.ModelResponse, error) {
	input := struct {
		Model    modelIdentity   `json:"model"`
		Messages []agent.Message `json:"messages"`
		Action   actionIdentity  `json:"action"`
	}{Model: m.identity, Messages: messages, Action: identifyAction(action)}
	var result agent.ModelResponse
	err := m.cache.run(ctx, "model.next", input, &result, func() error {
		var err error
		result, err = m.model.Next(ctx, messages, action)
		return err
	})
	return result, err
}

func (m cachedModel) Finalize(ctx context.Context, task string, action agent.Action, evidence []agent.Evidence) (agent.ModelResponse, error) {
	input := struct {
		Model    modelIdentity    `json:"model"`
		Task     string           `json:"task"`
		Action   actionIdentity   `json:"action"`
		Evidence []agent.Evidence `json:"evidence"`
	}{Model: m.identity, Task: task, Action: identifyAction(action), Evidence: evidence}
	var result agent.ModelResponse
	err := m.cache.run(ctx, "model.finalize", input, &result, func() error {
		var err error
		result, err = m.model.Finalize(ctx, task, action, evidence)
		return err
	})
	return result, err
}

func identifyAction(action agent.Action) actionIdentity {
	var schema json.RawMessage
	if action.Validator != nil {
		schema = action.Validator.Canonical
	}
	return actionIdentity{
		Instructions:          action.Instructions,
		FinalizerInstructions: action.FinalizerInstructions,
		Template:              action.Template,
		Schema:                schema,
	}
}

type cachedTool struct {
	agent.Tool
	cache operationCache
}

func (t cachedTool) Run(ctx context.Context, input map[string]any) (agent.ToolResult, error) {
	identity := struct {
		Tool  toolIdentity   `json:"tool"`
		Input map[string]any `json:"input"`
	}{
		Tool:  toolIdentity{Name: t.Name(), Description: t.Description(), Schema: t.Schema()},
		Input: input,
	}
	var result agent.ToolResult
	err := t.cache.run(ctx, "tool."+t.Name(), identity, &result, func() error {
		var err error
		result, err = t.Tool.Run(ctx, input)
		return err
	})
	return result, err
}
