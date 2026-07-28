package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonbalfe/freegent/internal/agent"
)

func TestBuildCLIRequestFromActionAndCSV(t *testing.T) {
	directory := t.TempDir()
	actionPath := filepath.Join(directory, "action.json")
	rowsPath := filepath.Join(directory, "rows.csv")
	if err := os.WriteFile(actionPath, []byte(`{"name":"crm","instructions":"Find the CRM.","template":"{{company}} {{domain}}","schema":{"crm":"string?","confidence":"low|medium|high"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rowsPath, []byte("company,domain\nLinear,linear.app\nVercel,vercel.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	request, rows, err := buildCLIRequest(actionPath, "", "", "", "", nil, rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	if request.Name != "crm" || len(rows) != 2 || rows[1]["domain"] != "vercel.com" {
		t.Fatalf("unexpected CLI request: %#v %#v", request, rows)
	}
}

func TestBuildCLIRequestUsesCSVFilenameAsJobName(t *testing.T) {
	rowsPath := filepath.Join(t.TempDir(), "prospects.csv")
	if err := os.WriteFile(rowsPath, []byte("company\nLinear\n"), 0644); err != nil {
		t.Fatal(err)
	}
	request, _, err := buildCLIRequest("", "Research.", "{{company}}", `{"summary":"string"}`, "", nil, rowsPath)
	if err != nil {
		t.Fatal(err)
	}
	if request.Name != "prospects.csv" {
		t.Fatalf("expected CSV filename, got %q", request.Name)
	}
}

func TestBuildCLIRequestMergesRepeatedInputsAndRow(t *testing.T) {
	request, rows, err := buildCLIRequest(
		"",
		"Research.",
		"{{company}} {{domain}}",
		`{"summary":"string"}`,
		"domain=linear.app",
		repeatedInputs{"company": "Linear"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Template != "{{company}} {{domain}}" || rows[0]["company"] != "Linear" || rows[0]["domain"] != "linear.app" {
		t.Fatalf("unexpected merged row: %#v", rows)
	}
}

func TestResearchInstructionsIncludeDoctrineAndTaskRules(t *testing.T) {
	value := agent.ResearchInstructions("Check only official sources.", `{"name":{"type":"string"}}`)
	for _, expected := range []string{"Never fabricate a URL", "Task-specific rules:", "Check only official sources.", "Answer schema:"} {
		if !strings.Contains(value, expected) {
			t.Fatalf("missing %q in instructions", expected)
		}
	}
}

func TestRemoteJobSubmissionAndPolling(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/jobs":
			writeTestJSON(writer, http.StatusAccepted, map[string]any{"jobId": "job-1", "status": "queued"})
		case request.Method == http.MethodGet && request.URL.Path == "/jobs/job-1":
			requests++
			status := "running"
			rows := []map[string]any{}
			if requests > 1 {
				status = "completed"
				rows = []map[string]any{{
					"index":  0,
					"status": "completed",
					"result": map[string]any{"runId": "run-1", "result": map[string]any{"summary": "done"}},
				}}
			}
			writeTestJSON(writer, http.StatusOK, map[string]any{"id": "job-1", "status": status, "rows": rows})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	id, err := submitRemoteJob(context.Background(), server.URL, APIRequest{
		Instructions: "Research.",
		Template:     "{{company}}",
		Schema:       json.RawMessage(`{"summary":"string"}`),
		Rows:         []map[string]any{{"company": "Linear"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := waitRemoteJob(context.Background(), server.URL, id)
	if err != nil {
		t.Fatal(err)
	}
	results := remoteJobResults(job)
	if len(results) != 1 || results[0].Result["summary"] != "done" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func writeTestJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		panic(err)
	}
}
