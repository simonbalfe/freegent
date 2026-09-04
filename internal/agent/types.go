package agent

import (
	"context"
	"errors"

	"github.com/simonbalfe/freegent/internal/openextract"
)

type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func Permanent(err error) error {
	return permanentError{err: err}
}

func IsPermanent(err error) bool {
	var target permanentError
	return errors.As(err, &target)
}

type Row map[string]string

type Action struct {
	Instructions          string
	FinalizerInstructions string
	Template              string
	Validator             *CompiledSchema
}

type Message struct {
	Role      string
	Content   string
	ToolCall  *ToolCall
	ToolCalls []ToolCall
}

type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

type ModelResponse struct {
	ToolCalls   []ToolCall
	Final       map[string]any
	OutputError string
	Usage       TokenUsage
	CostUSD     *float64
}

type Model interface {
	Next(context.Context, []Message, Action) (ModelResponse, error)
	Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error)
}

type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
}

func (u *TokenUsage) Add(other TokenUsage) {
	u.Input += other.Input
	u.Output += other.Output
}

type ToolResult struct {
	Text     string
	URLs     []string
	SeenURLs []string
	Provider string
	Attempts []FetchAttempt
	CostUSD  *float64
}

type Tool interface {
	Name() string
	Description() string
	Schema() map[string]any
	GuardedURL(map[string]any) string
	Run(context.Context, map[string]any) (ToolResult, error)
}

type Evidence struct {
	Tool     string         `json:"tool"`
	Text     string         `json:"text"`
	URLs     []string       `json:"urls"`
	Provider string         `json:"provider,omitempty"`
	Attempts []FetchAttempt `json:"attempts,omitempty"`
}

type FetchAttempt = openextract.Attempt

type Step struct {
	Kind  string         `json:"kind"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type CostUsage struct {
	OpenRouterUSD         float64 `json:"openRouterUsd"`
	ApifyUSD              float64 `json:"apifyUsd"`
	OpenRouterRecorded    bool    `json:"openRouterRecorded"`
	ProviderUsageRecorded bool    `json:"providerUsageRecorded"`
	ApifyRuns             int     `json:"apifyRuns"`
	UnpricedApifyRuns     int     `json:"unpricedApifyRuns"`
	SerperQueries         int     `json:"serperQueries"`
}

type RunResult struct {
	Answer   map[string]any `json:"answer"`
	Sources  []string       `json:"sources"`
	Evidence []Evidence     `json:"evidence"`
	Steps    []Step         `json:"steps"`
	Tokens   TokenUsage     `json:"tokens"`
	Costs    CostUsage      `json:"costs"`
}
