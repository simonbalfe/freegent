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
	"github.com/simonbalfe/freegent/internal/openrouter"
	"github.com/simonbalfe/freegent/internal/toolset"
)

type Step = agent.Step
type Evidence = agent.Evidence
type TokenUsage = agent.TokenUsage
type AgentEvent = agent.AgentEvent

const defaultOpenRouterModel = "deepseek/deepseek-v4-flash"

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
	mux.HandleFunc("GET /jobs/{id}/results.csv", func(writer http.ResponseWriter, request *http.Request) {
		handleJobCSV(writer, request, store)
	})
	mux.HandleFunc("GET /dashboard", func(writer http.ResponseWriter, request *http.Request) {
		handleDashboard(writer, request, store)
	})
	mux.HandleFunc("POST /dashboard/jobs", func(writer http.ResponseWriter, request *http.Request) {
		handleDashboardJob(writer, request, store)
	})
	mux.HandleFunc("GET /dashboard/jobs/{id}", func(writer http.ResponseWriter, request *http.Request) {
		handleDashboardJobPage(writer, request, store)
	})
	mux.HandleFunc("GET /dashboard/jobs/{id}/status", func(writer http.ResponseWriter, request *http.Request) {
		handleDashboardJobStatus(writer, request, store)
	})
	fmt.Fprintf(os.Stderr, "freegent listening on http://localhost:%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		fmt.Fprintf(os.Stderr, "freegent server stopped: %v\n", err)
	}
}

func runOneWithEvents(ctx context.Context, request APIRequest, values map[string]any, event func(AgentEvent)) APIResult {
	started := time.Now()
	runID := newRunID()
	modelName := request.Model
	if modelName == "" {
		modelName = os.Getenv("OPENROUTER_MODEL")
	}
	if modelName == "" {
		modelName = defaultOpenRouterModel
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
	key := os.Getenv("OPENROUTER_API_KEY")
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
	tools := toolset.Default()
	runner := agent.Agent{Model: openrouter.OpenRouterModel{APIKey: key, Model: modelName, Client: &http.Client{Timeout: 90 * time.Second}, Tools: toolset.List(tools), MaxOutputTokens: request.MaxOutputTokens}, Tools: tools, MaxSteps: maxSteps, Verbose: request.Verbose, Event: event}
	run, err := runner.Run(ctx, action, row)
	result.DurationMS = time.Since(started).Milliseconds()
	result.Result, result.Reasoning, result.Sources, result.AgentLog, result.Evidence, result.Tokens = run.Answer, run.Reasoning, run.Sources, run.Steps, run.Evidence, run.Tokens
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
