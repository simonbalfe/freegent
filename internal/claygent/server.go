package claygent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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
	Concurrency     int              `json:"concurrency"`
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

func serve(args []string) {
	flags := flag.NewFlagSet("openclaygent-go serve", flag.ExitOnError)
	port := flags.Int("port", 8080, "HTTP port")
	if err := flags.Parse(args); err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /run", handleRun)
	store := NewJobStore()
	mux.HandleFunc("POST /jobs", func(writer http.ResponseWriter, request *http.Request) {
		handleJob(writer, request, store)
	})
	mux.HandleFunc("GET /jobs/{id}", func(writer http.ResponseWriter, request *http.Request) {
		handleJobJSON(writer, request, store)
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
	fmt.Fprintf(os.Stderr, "openclaygent-go listening on http://localhost:%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), mux); err != nil {
		fmt.Fprintf(os.Stderr, "openclaygent-go server stopped: %v\n", err)
	}
}

func handleRun(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var input APIRequest
	if err := decoder.Decode(&input); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.Instructions == "" || input.Template == "" || !json.Valid(input.Schema) {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "instructions, template, and a valid schema are required"})
		return
	}
	if _, err := compileOutputSchema(input.Schema); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	rows := input.Rows
	if len(rows) == 0 {
		rows = []map[string]any{input.Input}
	}
	results := runBatch(request.Context(), input, rows)
	fmt.Fprintf(os.Stderr, "run rows=%d concurrency=%d durationMs=%d\n", len(rows), input.Concurrency, time.Since(started).Milliseconds())
	writeJSON(writer, http.StatusOK, map[string]any{"results": results})
}

func runBatch(ctx context.Context, request APIRequest, rows []map[string]any) []APIResult {
	return runBatchWithProgress(ctx, request, rows, runOne, nil)
}

func runBatchWith(ctx context.Context, request APIRequest, rows []map[string]any, run func(context.Context, APIRequest, map[string]any) APIResult) []APIResult {
	return runBatchWithProgress(ctx, request, rows, run, nil)
}

func runBatchWithProgress(ctx context.Context, request APIRequest, rows []map[string]any, run func(context.Context, APIRequest, map[string]any) APIResult, progress func(int, APIResult)) []APIResult {
	limit := request.Concurrency
	if limit < 1 {
		limit = 5
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	if limit == 0 {
		return []APIResult{}
	}
	results := make([]APIResult, len(rows))
	next := 0
	var mu sync.Mutex
	var workers sync.WaitGroup
	for range limit {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				mu.Lock()
				i := next
				next++
				mu.Unlock()
				if i >= len(rows) {
					return
				}
				results[i] = run(ctx, request, rows[i])
				if progress != nil {
					progress(i, results[i])
				}
			}
		}()
	}
	workers.Wait()
	return results
}

func runOne(ctx context.Context, request APIRequest, values map[string]any) APIResult {
	started := time.Now()
	runID := newRunID()
	modelName := request.Model
	if modelName == "" {
		modelName = "google/gemini-3.1-flash-lite"
	}
	result := APIResult{RunID: runID, Result: nil, Sources: []string{}, AgentLog: []Step{}, Evidence: []Evidence{}, Model: modelName}
	row := Row{}
	for key, value := range values {
		row[key] = fmt.Sprint(value)
	}
	if request.Require != "" && row[request.Require] == "" {
		result.Skipped = true
		result.DurationMS = time.Since(started).Milliseconds()
		writeRunLog(request, values, result)
		return result
	}
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		result.Error = "OPENROUTER_API_KEY is not set"
		result.DurationMS = time.Since(started).Milliseconds()
		writeRunLog(request, values, result)
		return result
	}
	compiledSchema, err := compileOutputSchema(request.Schema)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(started).Milliseconds()
		writeRunLog(request, values, result)
		return result
	}
	action := Action{
		Instructions:          researchInstructions(request.Instructions, string(compiledSchema.Canonical)),
		FinalizerInstructions: finalizerInstructions(request.Instructions),
		Template:              request.Template,
		Schema:                compiledSchema.Canonical,
		Validator:             compiledSchema,
	}
	maxSteps := request.MaxSteps
	if maxSteps < 1 {
		maxSteps = 5
	}
	tools := defaultTools()
	agent := Agent{Model: OpenRouterModel{APIKey: key, Model: modelName, Client: &http.Client{Timeout: 90 * time.Second}, Tools: toolList(tools), MaxOutputTokens: request.MaxOutputTokens}, Tools: tools, MaxSteps: maxSteps, Verbose: request.Verbose}
	run, err := agent.Run(ctx, action, row)
	result.DurationMS = time.Since(started).Milliseconds()
	result.Result, result.Reasoning, result.Sources, result.AgentLog, result.Evidence, result.Tokens = run.Answer, run.Reasoning, run.Sources, run.Steps, run.Evidence, run.Tokens
	if err != nil {
		result.Result = nil
		result.Reasoning = ""
		result.Error = err.Error()
	}
	writeRunLog(request, values, result)
	return result
}

func writeRunLog(request APIRequest, row map[string]any, result APIResult) {
	directory := os.Getenv("OPENCLAY_LOG_DIR")
	if directory == "" {
		directory = "logs"
	}
	if os.MkdirAll(directory, 0755) != nil {
		return
	}
	data, err := json.MarshalIndent(map[string]any{
		"recordedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"action": map[string]any{
			"name":         request.Name,
			"instructions": request.Instructions,
			"template":     request.Template,
			"schema":       request.Schema,
		},
		"input": row,
		"run":   result,
	}, "", "  ")
	if err == nil {
		path := filepath.Join(directory, result.RunID+".json")
		temporaryPath := path + ".tmp"
		if os.WriteFile(temporaryPath, append(data, '\n'), 0644) == nil {
			_ = os.Rename(temporaryPath, path)
		}
	}
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
