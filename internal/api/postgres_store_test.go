package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestPermanentOperationError(t *testing.T) {
	for _, message := range []string{
		"OPENROUTER_API_KEY is not set",
		"URL must use http or https",
		"fetch_page: OpenExtract could not extract the URL: Response appears to be a JavaScript shell or block page",
		"401 Unauthorized",
	} {
		if !permanentOperationError(message) {
			t.Fatalf("expected permanent error for %q", message)
		}
	}
	if permanentOperationError("provider returned 429") {
		t.Fatal("expected provider rate limit to be retryable")
	}
}

func TestAccumulateDashboardStats(t *testing.T) {
	stats := DashboardStats{}
	models := map[string]*DashboardModelStats{}
	accumulateDashboardStats(&stats, models, "completed", APIResult{
		Model: "example/model", Tokens: TokenUsage{Input: 100, Output: 20},
		Costs:    CostUsage{OpenRouterUSD: 0.01, OpenRouterRecorded: true, ApifyUSD: 0.03, ApifyRuns: 1},
		AgentLog: []Step{{Kind: "tool"}}, Sources: []string{"https://example.com"}, DurationMS: 500,
	})
	accumulateDashboardStats(&stats, models, "failed", APIResult{
		Model: "example/model", Tokens: TokenUsage{Input: 50, Output: 5},
		Evidence: []Evidence{{Provider: "apify:example~actor"}, {Provider: "serper", Attempts: []FetchAttempt{{Provider: "serper", Outcome: "ok"}}}}, DurationMS: 250,
	})
	model := models["example/model"]
	if stats.Completed != 1 || stats.Failed != 1 || stats.Tokens.Input != 150 || stats.Costs.OpenRouterUSD != 0.01 || stats.Costs.ApifyUSD != 0.03 || stats.UnpricedApifyRuns != 1 || stats.SerperQueries != 1 {
		t.Fatalf("unexpected aggregate stats: %+v", stats)
	}
	if model == nil || model.InputTokens != 150 || model.UnpricedInputTokens != 50 || model.OpenRouterUSD != 0.01 {
		t.Fatalf("unexpected model stats: %+v", model)
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
