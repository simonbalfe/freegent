package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/config"
	"github.com/simonbalfe/freegent/internal/openrouter"
	"github.com/simonbalfe/freegent/internal/toolset"
)

type Step = agent.Step
type Evidence = agent.Evidence
type FetchAttempt = agent.FetchAttempt
type TokenUsage = agent.TokenUsage
type CostUsage = agent.CostUsage
type AgentEvent = agent.AgentEvent

type APIRequest struct {
	Name            string           `json:"name"`
	Instructions    string           `json:"instructions"`
	Template        string           `json:"template"`
	Schema          json.RawMessage  `json:"schema"`
	Rows            []map[string]any `json:"rows"`
	Input           map[string]any   `json:"input"`
	Model           string           `json:"model"`
	MaxSteps        int              `json:"maxSteps"`
	MaxOutputTokens int              `json:"maxOutputTokens"`
	Require         string           `json:"require"`
	Verbose         bool             `json:"verbose,omitempty"`
}

type APIResult struct {
	RunID      string         `json:"runId"`
	Result     map[string]any `json:"result"`
	Reasoning  string         `json:"reasoning,omitempty"`
	Sources    []string       `json:"sources"`
	AgentLog   []Step         `json:"agentLog"`
	Evidence   []Evidence     `json:"evidence"`
	Tokens     TokenUsage     `json:"tokens"`
	Costs      CostUsage      `json:"costs"`
	DurationMS int64          `json:"durationMs"`
	Model      string         `json:"model"`
	Skipped    bool           `json:"skipped,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func Serve(args []string) {
	flags := flag.NewFlagSet("freegent serve", flag.ExitOnError)
	port := flags.Int("port", 8080, "HTTP port")
	if err := flags.Parse(args); err != nil {
		panic(err)
	}
	store, err := OpenBackend(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "freegent database failed: %v\n", err)
		return
	}
	defer store.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		if err := store.Ping(request.Context()); err != nil {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /jobs", func(writer http.ResponseWriter, request *http.Request) {
		handleJobsJSON(writer, request, store)
	})
	mux.HandleFunc("POST /jobs", func(writer http.ResponseWriter, request *http.Request) {
		handleJob(writer, request, store)
	})
	mux.HandleFunc("GET /jobs/{id}", func(writer http.ResponseWriter, request *http.Request) {
		handleJobJSON(writer, request, store)
	})
	mux.HandleFunc("GET /jobs/{id}/stats", func(writer http.ResponseWriter, request *http.Request) {
		handleJobStatsJSON(writer, request, store)
	})
	mux.HandleFunc("GET /jobs/{id}/results.csv", func(writer http.ResponseWriter, request *http.Request) {
		handleJobCSV(writer, request, store)
	})
	dashboard := newDashboardHandler()
	mux.Handle("GET /dashboard", dashboard)
	mux.Handle("GET /dashboard/", dashboard)
	fmt.Fprintf(os.Stderr, "freegent listening on http://localhost:%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		fmt.Fprintf(os.Stderr, "freegent server stopped: %v\n", err)
	}
}

func runOneWithEvents(ctx context.Context, request APIRequest, values map[string]any, event func(AgentEvent), cache operationCache, providers config.Providers) APIResult {
	started := time.Now()
	runID := newRunID()
	modelName := request.Model
	if modelName == "" {
		modelName = providers.OpenRouterModel
	}
	result := APIResult{RunID: runID, Result: nil, Sources: []string{}, AgentLog: []Step{}, Evidence: []Evidence{}, Model: modelName}
	row := agent.Row{}
	for key, value := range values {
		row[key] = fmt.Sprint(value)
	}
	if request.Require != "" && row[request.Require] == "" {
		result.Skipped = true
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	key := providers.OpenRouterAPIKey
	if key == "" {
		result.Error = "OPENROUTER_API_KEY is not set"
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	compiledSchema, err := agent.CompileOutputSchema(request.Schema)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	action := agent.Action{
		Instructions:          agent.ResearchInstructions(request.Instructions, string(compiledSchema.Canonical)),
		FinalizerInstructions: agent.FinalizerInstructions(request.Instructions),
		Template:              request.Template,
		Validator:             compiledSchema,
	}
	maxSteps := request.MaxSteps
	if maxSteps < 1 {
		maxSteps = 5
	}
	tools := toolset.Default(providers)
	for name, tool := range tools {
		tools[name] = cachedTool{Tool: tool, cache: cache}
	}
	toolList := toolset.List(tools)
	model := openrouter.OpenRouterModel{APIKey: key, Model: modelName, Client: &http.Client{Timeout: 150 * time.Second}, Tools: toolList, MaxOutputTokens: request.MaxOutputTokens}
	runner := agent.Agent{Model: newCachedModel(model, cache, modelName, request.MaxOutputTokens, toolList), Tools: tools, MaxSteps: maxSteps, Verbose: request.Verbose, Event: event}
	run, err := runner.Run(ctx, action, row)
	result.DurationMS = time.Since(started).Milliseconds()
	result.Result, result.Reasoning, result.Sources, result.AgentLog, result.Evidence, result.Tokens, result.Costs = run.Answer, run.Reasoning, run.Sources, run.Steps, run.Evidence, run.Tokens, run.Costs
	if err != nil {
		result.Result = nil
		result.Reasoning = ""
		result.Error = err.Error()
	}
	return result
}

func newRunID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
