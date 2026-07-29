package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDashboardSeparatesJobsAndRunPages(t *testing.T) {
	jobs := []DashboardJob{{
		ID:          "job-123",
		Name:        "Prospect research",
		Status:      "running",
		Total:       10,
		Completed:   4,
		CreatedAt:   time.Date(2026, time.July, 28, 10, 30, 0, 0, time.UTC),
		LatestEvent: "Researching row 5",
	}}

	var jobsPage bytes.Buffer
	if err := DashboardPage(jobs).Render(context.Background(), &jobsPage); err != nil {
		t.Fatal(err)
	}
	jobsHTML := jobsPage.String()
	for _, expected := range []string{"Prospect research", "4 of 10 rows", "New run", `href="/dashboard/run"`} {
		if !strings.Contains(jobsHTML, expected) {
			t.Fatalf("jobs page does not contain %q", expected)
		}
	}
	if strings.Contains(jobsHTML, `action="/dashboard/jobs"`) {
		t.Fatal("jobs page contains the run form")
	}

	var runPage bytes.Buffer
	if err := DashboardRunPage().Render(context.Background(), &runPage); err != nil {
		t.Fatal(err)
	}
	runHTML := runPage.String()
	for _, expected := range []string{"Start a run", `action="/dashboard/jobs"`, `name="csv"`, `name="rows"`} {
		if !strings.Contains(runHTML, expected) {
			t.Fatalf("run page does not contain %q", expected)
		}
	}
}

func TestDashboardJobSummary(t *testing.T) {
	jobs := []DashboardJob{
		{Status: "running", Total: 10, Completed: 4},
		{Status: "completed", Total: 3, Completed: 3},
		{Status: "completed with errors", Total: 5, Completed: 5},
	}
	if got := dashboardActiveJobCount(jobs); got != 1 {
		t.Fatalf("active jobs = %d, want 1", got)
	}
	if got := dashboardCompletedRowCount(jobs); got != 12 {
		t.Fatalf("completed rows = %d, want 12", got)
	}
	if got := dashboardAttentionJobCount(jobs); got != 1 {
		t.Fatalf("attention jobs = %d, want 1", got)
	}
	if got := dashboardJobProgress(jobs[0]); got != 40 {
		t.Fatalf("progress = %d, want 40", got)
	}
}

func TestDashboardJobRowsShowDataAndSteps(t *testing.T) {
	job := DashboardJob{
		ID:     "job-456",
		Status: "completed",
		Total:  1,
		Request: APIRequest{
			Instructions:    "Use first-party product pages.",
			Template:        "Research {{company}} using {{domain}}.",
			Schema:          json.RawMessage(`{"answer":"string","confidence":"number"}`),
			Model:           "request-model",
			MaxSteps:        7,
			MaxOutputTokens: 2048,
			Require:         "company",
		},
		Rows: []DashboardRow{{
			Index:  0,
			Status: "completed",
			Input:  map[string]any{"company": "Figma", "domain": "figma.com"},
			Result: APIResult{
				Result:     map[string]any{"answer": "A collaborative design platform.", "confidence": 0.98},
				Reasoning:  "The official product page identifies the core audience and use case.",
				Sources:    []string{"https://figma.com"},
				AgentLog:   []Step{{Kind: "tool", Name: "fetch_page", Input: map[string]any{"url": "https://figma.com"}}},
				Evidence:   []Evidence{{Tool: "fetch_page", Provider: "openextract", Text: "Official product page content."}},
				Tokens:     TokenUsage{Input: 1000, Output: 100},
				DurationMS: 1250,
				Model:      "google/gemini-3.1-flash-lite",
			},
			AgentLog: []Step{{Kind: "tool", Name: "fetch_page", Input: map[string]any{"url": "https://figma.com"}}},
		}},
	}

	var page bytes.Buffer
	if err := DashboardJobStatus(job).Render(context.Background(), &page); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	for _, expected := range []string{
		"Job setup",
		"Use first-party product pages.",
		"Prompt template",
		"Research {{company}} using {{domain}}.",
		"Full agent system prompt",
		"Output schema",
		"request-model",
		"2048",
		"Estimated cost",
		"$0.0004",
		"View breakdown",
		"AI model",
		"Serper",
		"Apify",
		"Row 1",
		"Rendered prompt",
		"Research Figma using figma.com.",
		"Row data",
		"Input",
		"Output",
		"Figma",
		"A collaborative design platform.",
		"Agent steps (1)",
		"The official product page identifies the core audience and use case.",
		"Final reasoning",
		"Tool call",
		"fetch_page via openextract result",
		"Refresh",
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("row view does not contain %q", expected)
		}
	}
	for _, unwanted := range []string{"sheet-table", `onclick="toggleSheetRow(this)"`, "Final JSON output"} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("row view still contains %q", unwanted)
		}
	}
}

func TestActiveJobPollingDoesNotReplaceOpenRows(t *testing.T) {
	job := DashboardJob{
		ID:      "job-live",
		Status:  "running",
		Total:   1,
		Request: APIRequest{Schema: json.RawMessage(`{"answer":"string"}`)},
		Rows:    []DashboardRow{{Index: 0, Status: "running", Input: map[string]any{"company": "Figma"}}},
	}

	var page bytes.Buffer
	if err := DashboardJobStatus(job).Render(context.Background(), &page); err != nil {
		t.Fatal(err)
	}
	html := page.String()
	if strings.Count(html, "hx-trigger=") != 1 {
		t.Fatalf("active job should poll only its overview")
	}
	if !strings.Contains(html, `id="job-overview"`) || !strings.Contains(html, `id="job-sheet"`) {
		t.Fatal("active job does not separate the live overview from the row view")
	}
}
