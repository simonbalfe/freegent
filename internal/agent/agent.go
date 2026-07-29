package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
)

const maxToolErrorRecoveries = 2
const submitAnswerTool = "submit_answer"

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
	task := RenderTemplate(action.Template, row)
	a.tracef("run start max_steps=%d task=%q", a.MaxSteps, task)
	ledger := newURLLedger(row)
	messages := []Message{{Role: "system", Content: action.Instructions}, {Role: "user", Content: task}}
	evidence := []Evidence{}
	steps := []Step{}
	tokens := TokenUsage{}
	failed := func(err error) (RunResult, error) {
		return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, err
	}

	stepsRemaining := a.MaxSteps
	recoveriesRemaining := maxToolErrorRecoveries
	for stepsRemaining > 0 {
		stepsRemaining--
		response, err := a.Model.Next(ctx, messages, action)
		if err != nil {
			return failed(err)
		}
		tokens.Add(response.Usage)
		if response.Final != nil {
			if err := action.Validator.Validate(response.Final); err == nil {
				a.tracef("model returned schema-valid final answer")
				steps = append(steps, Step{Kind: "answer"})
				return RunResult{Answer: response.Final, Reasoning: response.Reasoning, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, nil
			}
			a.tracef("model returned invalid final answer; finalizing from %d evidence items", len(evidence))
			return a.finalize(ctx, task, action, ledger, evidence, steps, tokens)
		}
		if len(response.ToolCalls) == 0 {
			a.tracef("model returned no tool call; finalizing from %d evidence items", len(evidence))
			return a.finalize(ctx, task, action, ledger, evidence, steps, tokens)
		}
		messages = append(messages, Message{Role: "assistant", ToolCalls: response.ToolCalls})
		hadRecoverableError := false
		researchCalls := make([]ToolCall, 0, len(response.ToolCalls))

		for _, call := range response.ToolCalls {
			if call.Name != submitAnswerTool {
				researchCalls = append(researchCalls, call)
				continue
			}
			if len(response.ToolCalls) != 1 {
				message := "Answer submission rejected: submit_answer must be called by itself after research calls finish. Review the new evidence before submitting the answer."
				a.recordToolError(&messages, &steps, call, message)
				hadRecoverableError = true
				continue
			}
			if len(evidence) == 0 {
				message := "Answer submission rejected: gather current evidence with a research tool before calling submit_answer."
				a.recordToolError(&messages, &steps, call, message)
				hadRecoverableError = true
				continue
			}
			answer, reasoning, err := submittedAnswer(call.Input, action.Validator)
			if err != nil {
				message := fmt.Sprintf("Answer submission rejected: %s. Correct the answer to match the required schema and call submit_answer again.", err)
				a.recordToolError(&messages, &steps, call, message)
				hadRecoverableError = true
				continue
			}
			a.tracef("model submitted schema-valid final answer")
			steps = append(steps, Step{Kind: "answer"})
			return RunResult{Answer: answer, Reasoning: reasoning, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, nil
		}

		outcomes := a.runToolCalls(ctx, researchCalls, ledger)
		if ctx.Err() != nil {
			return failed(ctx.Err())
		}
		for _, outcome := range outcomes {
			call := outcome.Call
			if outcome.Error != "" {
				a.recordToolError(&messages, &steps, call, outcome.Error)
				hadRecoverableError = true
				continue
			}
			result := outcome.Result
			a.tracef("tool=%s completed chars=%d urls=%d", call.Name, len(result.Text), len(result.URLs))
			for _, rawURL := range result.URLs {
				ledger.addSource(rawURL)
			}
			for _, rawURL := range result.SeenURLs {
				ledger.addSeen(rawURL)
			}
			evidence = append(evidence, Evidence{Tool: call.Name, Text: result.Text, URLs: result.URLs, Provider: result.Provider, Attempts: result.Attempts, CostUSD: result.CostUSD, CostKnown: result.CostKnown})
			steps = append(steps, Step{Kind: "tool", Name: call.Name, Input: call.Input})
			messages = append(messages, Message{Role: "tool", ToolCall: &call, Content: result.Text})
		}
		if hadRecoverableError && recoveriesRemaining > 0 {
			stepsRemaining++
			recoveriesRemaining--
		}
	}
	a.tracef("step limit reached; finalizing from %d evidence items", len(evidence))
	return a.finalize(ctx, task, action, ledger, evidence, steps, tokens)
}

type toolCallOutcome struct {
	Call   ToolCall
	Result ToolResult
	Error  string
}

func (a Agent) runToolCalls(ctx context.Context, calls []ToolCall, ledger urlLedger) []toolCallOutcome {
	outcomes := make([]toolCallOutcome, len(calls))
	var wait sync.WaitGroup
	for index, call := range calls {
		outcomes[index].Call = call
		a.tracef("model requested tool=%s input=%v", call.Name, call.Input)
		tool, ok := a.Tools[call.Name]
		if !ok {
			outcomes[index].Error = fmt.Sprintf("Tool call failed: unknown tool %q. Choose one of the available tools and continue.", call.Name)
			continue
		}
		if rawURL := guardedToolURL(call.Name, call.Input); rawURL != "" && !ledger.permits(rawURL) {
			outcomes[index].Error = fmt.Sprintf("Tool call rejected: URL %q is unverified. Do not guess or alter the URL. Search for the exact page or entity first, use a URL returned by search or a fetched page, or pass the exact company name when the tool accepts one. Continue the task.", rawURL)
			continue
		}
		wait.Add(1)
		go func(index int, tool Tool, input map[string]any) {
			defer wait.Done()
			result, err := tool.Run(ctx, input)
			if err != nil {
				outcomes[index].Error = fmt.Sprintf("Tool call failed: %s. Correct the input or use another source, then continue the task.", err)
				return
			}
			outcomes[index].Result = result
		}(index, tool, call.Input)
	}
	wait.Wait()
	return outcomes
}

func submittedAnswer(input map[string]any, validator *CompiledSchema) (map[string]any, string, error) {
	answer, ok := input["answer"].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("answer must be an object")
	}
	if err := validator.Validate(answer); err != nil {
		return nil, "", err
	}
	reasoning, _ := input["reasoning"].(string)
	return answer, reasoning, nil
}

func (a Agent) recordToolError(messages *[]Message, steps *[]Step, call ToolCall, message string) {
	a.tracef("tool=%s recoverable_error=%q", call.Name, message)
	*steps = append(*steps, Step{Kind: "tool_error", Name: call.Name, Input: call.Input, Error: message})
	*messages = append(*messages, Message{Role: "tool", ToolCall: &call, Content: message})
}

func (a Agent) finalize(ctx context.Context, task string, action Action, ledger urlLedger, evidence []Evidence, steps []Step, tokens TokenUsage) (RunResult, error) {
	a.tracef("finalizer start evidence=%d", len(evidence))
	response, err := a.Model.Finalize(ctx, task, action, evidence)
	if err != nil {
		return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, err
	}
	tokens.Add(response.Usage)
	if err := action.Validator.Validate(response.Final); err != nil {
		return RunResult{Answer: nil, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, fmt.Errorf("final answer failed schema validation: %w", err)
	}
	steps = append(steps, Step{Kind: "finalize"})
	return RunResult{Answer: response.Final, Reasoning: response.Reasoning, Sources: ledger.sources(), Evidence: evidence, Steps: steps, Tokens: tokens}, nil
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
