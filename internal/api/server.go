package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/codex"
	"github.com/simonbalfe/freegent/internal/config"
	"github.com/simonbalfe/freegent/internal/openrouter"
	"github.com/simonbalfe/freegent/internal/tools/apify"
	"github.com/simonbalfe/freegent/internal/tools/fetch"
	"github.com/simonbalfe/freegent/internal/tools/search"
)

type APIRequest struct {
	Name            string           `json:"name"`
	Instructions    string           `json:"instructions"`
	Template        string           `json:"template"`
	Schema          json.RawMessage  `json:"schema"`
	Rows            []map[string]any `json:"rows"`
	Model           string           `json:"model"`
	MaxSteps        int              `json:"maxSteps"`
	MaxOutputTokens int              `json:"maxOutputTokens"`
}

type APIResult struct {
	Result   map[string]any   `json:"result"`
	Sources  []string         `json:"sources"`
	AgentLog []agent.Step     `json:"agentLog"`
	Evidence []agent.Evidence `json:"evidence"`
	Tokens   agent.TokenUsage `json:"tokens"`
	Costs    agent.CostUsage  `json:"costs"`
	Model    string           `json:"model"`
	Error    string           `json:"error,omitempty"`
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

func runOneWithEvents(ctx context.Context, request APIRequest, values map[string]any, event func(agent.AgentEvent), cache operationCache, providers config.Providers, codexAuth *codex.TokenSource, client *http.Client) (APIResult, error) {
	modelName, modelIdentity := selectedModel(request.Model, providers)
	result := APIResult{Result: nil, Sources: []string{}, AgentLog: []agent.Step{}, Evidence: []agent.Evidence{}, Model: modelIdentity}
	row := agent.Row{}
	for key, value := range values {
		row[key] = fmt.Sprint(value)
	}
	compiledSchema, err := agent.CompileOutputSchema(request.Schema)
	if err != nil {
		result.Error = err.Error()
		return result, agent.Permanent(err)
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
	tools := defaultTools(providers)
	for name, tool := range tools {
		tools[name] = cachedTool{Tool: tool, cache: cache}
	}
	toolList := make([]agent.Tool, 0, len(tools))
	for _, name := range slices.Sorted(maps.Keys(tools)) {
		toolList = append(toolList, tools[name])
	}
	var model agent.Model
	switch providers.ModelProvider {
	case "openrouter":
		if providers.OpenRouterAPIKey == "" {
			result.Error = "OPENROUTER_API_KEY is not set"
			return result, agent.Permanent(errors.New(result.Error))
		}
		model = openrouter.OpenRouterModel{APIKey: providers.OpenRouterAPIKey, Model: modelName, Client: client, Tools: toolList, MaxOutputTokens: request.MaxOutputTokens}
	case "codex":
		if codexAuth == nil {
			result.Error = "Codex authentication is not available; run freegent auth"
			return result, agent.Permanent(errors.New(result.Error))
		}
		model = codex.Model{Model: modelName, Client: client, Auth: codexAuth, Tools: toolList, MaxOutputTokens: request.MaxOutputTokens}
	default:
		result.Error = fmt.Sprintf("unsupported model provider %q", providers.ModelProvider)
		return result, agent.Permanent(errors.New(result.Error))
	}
	runner := agent.Agent{Model: newCachedModel(model, cache, modelIdentity, request.MaxOutputTokens, toolList), Tools: tools, MaxSteps: maxSteps, Event: event}
	run, err := runner.Run(ctx, action, row)
	result.Result, result.Sources, result.AgentLog, result.Evidence, result.Tokens, result.Costs = run.Answer, run.Sources, run.Steps, run.Evidence, run.Tokens, run.Costs
	if err != nil {
		result.Result = nil
		result.Error = err.Error()
	}
	return result, err
}

func selectedModel(requested string, providers config.Providers) (string, string) {
	name := requested
	if name == "" {
		if providers.ModelProvider == "codex" {
			name = providers.CodexModel
		} else {
			name = providers.OpenRouterModel
		}
	}
	if providers.ModelProvider == "codex" {
		return name, "codex/" + name
	}
	return name, name
}

func defaultTools(providers config.Providers) map[string]agent.Tool {
	tools := map[string]agent.Tool{
		"web_search": search.New(providers.SerperAPIKey, providers.ExaAPIKey, providers.TavilyAPIKey),
		"fetch_page": fetch.New(providers.OpenExtractURL, providers.ExaAPIKey, providers.TavilyAPIKey),
	}
	if providers.ApifyAPIToken != "" {
		for _, tool := range apify.Tools(providers.ApifyAPIToken) {
			tools[tool.Name()] = tool
		}
	}
	return tools
}

func newJobID() string {
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
