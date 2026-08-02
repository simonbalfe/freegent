package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const maxSuccessfulToolCalls = 6

type Agent struct {
	Model    Model
	Tools    map[string]Tool
	MaxSteps int
	Verbose  bool
	Event    func(AgentEvent)
}

type AgentEvent struct {
	Message string
}

func (a Agent) Run(ctx context.Context, action Action, row Row) (RunResult, error) {
	task := fillTemplate(action.Template, row)
	a.tracef("run start max_steps=%d task=%q", a.MaxSteps, task)
	ledger := newURLLedger(row)
	messages := []Message{{Role: "system", Content: action.Instructions}, {Role: "user", Content: task}}
	evidence := []Evidence{}
	steps := []Step{}
	tokens := TokenUsage{}
	costs := CostUsage{}
	toolResults := map[string]ToolResult{}
	successfulToolCalls := 0
	failed := func(err error) (RunResult, error) {
		return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, err
	}

	for range a.MaxSteps {
		response, err := a.Model.Next(ctx, messages, action)
		if err != nil {
			return failed(err)
		}
		tokens.Add(response.Usage)
		costs.addOpenRouter(response.CostUSD)
		if response.OutputError != "" {
			a.tracef("model returned invalid output; finalizing: %s", response.OutputError)
			return a.finalize(ctx, task, action, ledger, evidence, steps, tokens, costs)
		}
		if response.Final != nil {
			if err := action.Validator.Validate(response.Final); err == nil {
				a.tracef("model returned schema-valid final answer")
				steps = append(steps, Step{Kind: "answer"})
				return RunResult{Answer: response.Final, Reasoning: response.Reasoning, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, nil
			}
			a.tracef("model returned invalid final answer; finalizing from %d evidence items", len(evidence))
			return a.finalize(ctx, task, action, ledger, evidence, steps, tokens, costs)
		}
		if len(response.ToolCalls) == 0 {
			a.tracef("model returned no tool call; finalizing from %d evidence items", len(evidence))
			return a.finalize(ctx, task, action, ledger, evidence, steps, tokens, costs)
		}
		messages = append(messages, Message{Role: "assistant", ToolCalls: response.ToolCalls})

		for _, call := range response.ToolCalls {
			a.tracef("model requested tool=%s input=%v", call.Name, call.Input)
			tool, ok := a.Tools[call.Name]
			if !ok {
				return failed(fmt.Errorf("model selected unknown tool %q", call.Name))
			}
			if rawURL := guardedToolURL(call.Name, call.Input); rawURL != "" && !ledger.permits(rawURL) {
				message := fmt.Sprintf("URL %q was rejected because it was not supplied in the input or discovered through search or extraction. Use web_search to find the exact URL before calling %s again.", rawURL, call.Name)
				a.tracef("tool=%s rejected unverified url=%q", call.Name, rawURL)
				steps = append(steps, Step{Kind: "rejected", Name: call.Name, Input: call.Input})
				messages = append(messages, Message{Role: "tool", ToolCall: &call, Content: message})
				continue
			}
			encodedInput, err := json.Marshal(call.Input)
			if err != nil {
				return failed(fmt.Errorf("encode %s input: %w", call.Name, err))
			}
			callKey := call.Name + ":" + string(encodedInput)
			if result, ok := toolResults[callKey]; ok {
				a.tracef("tool=%s reused identical result", call.Name)
				steps = append(steps, Step{Kind: "reuse", Name: call.Name, Input: call.Input})
				messages = append(messages, Message{Role: "tool", ToolCall: &call, Content: result.Text})
				continue
			}
			result, err := tool.Run(ctx, call.Input)
			if err != nil {
				if ctx.Err() != nil {
					return failed(fmt.Errorf("%s: %w", call.Name, ctx.Err()))
				}
				message := fmt.Sprintf("%s failed: %v. Treat this source or method as unavailable and try another verified source or tool.", call.Name, err)
				a.tracef("tool=%s failed: %v", call.Name, err)
				steps = append(steps, Step{Kind: "tool_error", Name: call.Name, Input: call.Input})
				messages = append(messages, Message{Role: "tool", ToolCall: &call, Content: message})
				continue
			}
			toolResults[callKey] = result
			costs.addApify(result.CostUSD)
			successfulToolCalls++
			a.tracef("tool=%s completed chars=%d urls=%d", call.Name, len(result.Text), len(result.URLs))
			for _, rawURL := range result.URLs {
				ledger.addSource(rawURL)
			}
			for _, rawURL := range result.SeenURLs {
				ledger.addSeen(rawURL)
			}
			evidence = append(evidence, Evidence{Tool: call.Name, Text: result.Text, URLs: result.URLs, Provider: result.Provider, Attempts: result.Attempts})
			steps = append(steps, Step{Kind: "tool", Name: call.Name, Input: call.Input})
			messages = append(messages, Message{Role: "tool", ToolCall: &call, Content: result.Text})
			if successfulToolCalls == maxSuccessfulToolCalls {
				a.tracef("tool call limit reached; finalizing from %d evidence items", len(evidence))
				return a.finalize(ctx, task, action, ledger, evidence, steps, tokens, costs)
			}
		}
	}
	a.tracef("step limit reached; finalizing from %d evidence items", len(evidence))
	return a.finalize(ctx, task, action, ledger, evidence, steps, tokens, costs)
}

func (a Agent) finalize(ctx context.Context, task string, action Action, ledger urlLedger, evidence []Evidence, steps []Step, tokens TokenUsage, costs CostUsage) (RunResult, error) {
	for attempt := 1; attempt <= 2; attempt++ {
		a.tracef("finalizer start attempt=%d evidence=%d", attempt, len(evidence))
		response, err := a.Model.Finalize(ctx, task, action, evidence)
		if err != nil {
			if attempt == 1 && ctx.Err() == nil {
				a.tracef("finalizer request failed; retrying: %v", err)
				continue
			}
			return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, err
		}
		tokens.Add(response.Usage)
		costs.addOpenRouter(response.CostUSD)
		if response.OutputError != "" {
			if attempt == 1 {
				a.tracef("finalizer returned invalid output; retrying: %s", response.OutputError)
				action.FinalizerInstructions += "\n\nThe previous response was invalid: " + response.OutputError + ". Return shorter valid JSON that matches every required field."
				continue
			}
			return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, errors.New(response.OutputError)
		}
		if err := action.Validator.Validate(response.Final); err != nil {
			if attempt == 1 {
				a.tracef("finalizer returned invalid answer; retrying")
				action.FinalizerInstructions += "\n\nThe previous answer failed schema validation: " + err.Error() + "\nReturn every required property with the correct type."
				continue
			}
			return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, fmt.Errorf("final answer failed schema validation: %w", err)
		}
		steps = append(steps, Step{Kind: "finalize"})
		return RunResult{Answer: response.Final, Reasoning: response.Reasoning, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens, Costs: costs}, nil
	}
	panic("unreachable")
}

func (c *CostUsage) addOpenRouter(cost *float64) {
	if cost == nil {
		return
	}
	c.OpenRouterUSD += *cost
	c.OpenRouterRecorded = true
}

func (c *CostUsage) addApify(cost *float64) {
	if cost == nil {
		return
	}
	c.ApifyUSD += *cost
	c.ApifyRuns++
}

func (a Agent) tracef(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if a.Event != nil {
		a.Event(AgentEvent{Message: message})
	}
	if a.Verbose {
		fmt.Fprintln(os.Stderr, "[agent] "+message)
	}
}
