package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/config"
)

func TestParseCSVRows(t *testing.T) {
	rows, err := parseCSVRows(strings.NewReader("company,domain\nLinear,linear.app\nFigma,figma.com\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1]["domain"] != "figma.com" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
	if _, err := parseCSVRows(strings.NewReader("company,company\nLinear,linear.app\n")); err == nil {
		t.Fatal("expected duplicate header error")
	}
}

func TestDecodeJobRequestAcceptsMultipartCSV(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"instructions": "Use supplied values only.",
		"template":     "Process {{company}} at {{domain}}.",
		"schema":       `{"company":"string","domain":"string"}`,
	} {
		if err := form.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := form.CreateFormFile("csv", "companies.csv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("company,domain\nLinear,linear.app\nFigma,figma.com\n")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/jobs", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	input, rows, err := decodeJobRequest(httptest.NewRecorder(), request)
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "companies.csv" || len(rows) != 2 {
		t.Fatalf("unexpected multipart job: %#v %#v", input, rows)
	}
	if rows[0]["company"] != "Linear" || rows[1]["domain"] != "figma.com" {
		t.Fatalf("unexpected CSV rows: %#v", rows)
	}
}

func TestDecodeJobRequestRejectsLegacyInput(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{"instructions":"Research.","template":"Research {{company}}.","schema":{"answer":"string"},"input":{"company":"Linear"}}`))
	request.Header.Set("Content-Type", "application/json")
	_, _, err := decodeJobRequest(httptest.NewRecorder(), request)
	if err == nil || !strings.Contains(err.Error(), "unknown field \"input\"") {
		t.Fatalf("legacy input error = %v", err)
	}
}

func TestPermanentOperationError(t *testing.T) {
	err := agent.Permanent(errors.New("invalid output schema"))
	if !agent.IsPermanent(err) {
		t.Fatal("expected typed permanent error")
	}
	if agent.IsPermanent(errors.New("provider returned 429")) {
		t.Fatal("expected provider rate limit to be retryable")
	}
}

func TestPostgresStoreLifecycle(t *testing.T) {
	databaseURL := os.Getenv("FREEGENT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("FREEGENT_TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "freegent_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	store, err := OpenPostgresStore(ctx, parsedURL.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		_ = admin.Close(context.Background())
	})

	request := APIRequest{Instructions: "Research.", Template: "{{company}}", Schema: json.RawMessage(`{"answer":"string"}`)}
	id, err := store.Start(ctx, request, []map[string]any{{"company": "Linear"}})
	if err != nil {
		t.Fatal(err)
	}
	args := OperationArgs{JobID: id, RowIndex: 0}
	storedRequest, input, done, err := store.beginOperation(ctx, args, 1)
	if err != nil || done || storedRequest.Template != request.Template || input["company"] != "Linear" {
		t.Fatalf("begin operation = request=%+v input=%+v done=%v error=%v", storedRequest, input, done, err)
	}
	if err := store.retryOperation(ctx, args, APIResult{Error: "temporary failure"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, _, done, err = store.beginOperation(ctx, args, 2); err != nil || done {
		t.Fatalf("retry begin = done=%v error=%v", done, err)
	}
	if err := store.completeOperation(ctx, args, APIResult{Result: map[string]any{"answer": "done"}}); err != nil {
		t.Fatal(err)
	}
	job, err := store.get(ctx, id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" || job.Completed != 1 || len(job.Rows) != 1 || job.Rows[0].Result.Result["answer"] != "done" {
		t.Fatalf("completed job = %+v", job)
	}
}

func TestAccumulateDashboardStats(t *testing.T) {
	stats := DashboardStats{}
	models := map[string]*DashboardModelStats{}
	accumulateDashboardStats(&stats, models, "completed", APIResult{
		Model: "example/model", Tokens: agent.TokenUsage{Input: 100, Output: 20},
		Costs: agent.CostUsage{
			OpenRouterUSD: 0.01, OpenRouterRecorded: true, ApifyUSD: 0.03, ApifyRuns: 1,
			ProviderUsageRecorded: true, UnpricedApifyRuns: 1, SerperQueries: 2,
		},
		AgentLog: []agent.Step{{Kind: "tool"}}, Sources: []string{"https://example.com"},
	})
	accumulateDashboardStats(&stats, models, "failed", APIResult{
		Model: "example/model", Tokens: agent.TokenUsage{Input: 50, Output: 5},
		Evidence: []agent.Evidence{{Provider: "apify:example~actor"}, {Provider: "serper", Attempts: []agent.FetchAttempt{{Provider: "serper", Outcome: "ok"}}}},
	})
	model := models["example/model"]
	if stats.Completed != 1 || stats.Failed != 1 || stats.Tokens.Input != 150 || stats.Costs.OpenRouterUSD != 0.01 || stats.Costs.ApifyUSD != 0.03 || stats.UnpricedApifyRuns != 2 || stats.SerperQueries != 3 {
		t.Fatalf("unexpected aggregate stats: %+v", stats)
	}
	if stats.DurationMS != 0 {
		t.Fatalf("row runtimes must not be summed: %+v", stats)
	}
	if model == nil || model.InputTokens != 150 || model.UnpricedInputTokens != 50 || model.OpenRouterUSD != 0.01 {
		t.Fatalf("unexpected model stats: %+v", model)
	}
}

func TestDefaultToolsAreProviderGated(t *testing.T) {
	if tools := defaultTools(config.Providers{}); len(tools) != 2 {
		t.Fatalf("web-only tool count = %d, want 2", len(tools))
	}
	tools := defaultTools(config.Providers{ApifyAPIToken: "secret"})
	if len(tools) != 8 {
		t.Fatalf("enrichment tool count = %d, want 8", len(tools))
	}
	for _, name := range []string{"linkedin_profile", "linkedin_posts", "linkedin_post_reactions", "linkedin_find_people", "linkedin_company", "crunchbase_company"} {
		if tools[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
}

func TestDashboardHandler(t *testing.T) {
	handler := newDashboardHandler()
	for _, target := range []string{"/dashboard", "/dashboard/jobs/job-1"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, target, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<div id="root"></div>`) {
			t.Fatalf("dashboard %s: status=%d body=%q", target, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard/missing.js", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing asset status=%d, want %d", response.Code, http.StatusNotFound)
	}
}
