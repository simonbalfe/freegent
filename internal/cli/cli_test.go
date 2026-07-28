package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRowRequest(t *testing.T) {
	request, err := buildRowRequest(
		`{"company":"Linear","domain":"linear.app"}`,
		"Use first-party evidence.",
		"Research {{company}} at {{domain}}.",
		`{"answer":"string"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Instructions != "Use first-party evidence." || request.Template != "Research {{company}} at {{domain}}." {
		t.Fatalf("unexpected request: %#v", request)
	}
	if len(request.Rows) != 1 || request.Rows[0]["domain"] != "linear.app" {
		t.Fatalf("unexpected row: %#v", request.Rows)
	}
}

func TestBuildRowRequestRejectsNonObject(t *testing.T) {
	if _, err := buildRowRequest(`["Linear"]`, defaultInstructions, "Research.", defaultSchema); err == nil {
		t.Fatal("expected JSON object error")
	}
}

func TestSubmitRemoteCSVJobUsesMultipartAPI(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "companies.csv")
	if err := os.WriteFile(csvPath, []byte("company,domain\nLinear,linear.app\n"), 0644); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/jobs" {
			http.NotFound(writer, request)
			return
		}
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		if request.FormValue("instructions") != "Use primary sources." {
			t.Fatalf("unexpected instructions %q", request.FormValue("instructions"))
		}
		if request.FormValue("template") != "Research {{company}}." || request.FormValue("schema") != defaultSchema {
			t.Fatalf("unexpected form: %#v", request.MultipartForm.Value)
		}
		file, _, err := request.FormFile("csv")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "company,domain\nLinear,linear.app\n" {
			t.Fatalf("unexpected CSV %q", string(data))
		}
		writeTestJSON(writer, http.StatusAccepted, map[string]any{"jobId": "job-csv", "status": "queued"})
	}))
	defer server.Close()

	jobID, err := submitRemoteCSVJob(
		context.Background(),
		server.URL,
		csvPath,
		"Use primary sources.",
		"Research {{company}}.",
		defaultSchema,
	)
	if err != nil {
		t.Fatal(err)
	}
	if jobID != "job-csv" {
		t.Fatalf("unexpected job ID %q", jobID)
	}
}

func TestRemoteJobSubmissionAndPolling(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/jobs":
			if request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected content type %q", request.Header.Get("Content-Type"))
			}
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
					"result": map[string]any{"runId": "run-1", "result": map[string]any{"answer": "done"}},
				}}
			}
			writeTestJSON(writer, http.StatusOK, map[string]any{"id": "job-1", "status": status, "rows": rows})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	id, err := submitRemoteJob(context.Background(), server.URL, APIRequest{
		Instructions: defaultInstructions,
		Template:     "{{company}}",
		Schema:       json.RawMessage(defaultSchema),
		Rows:         []map[string]any{{"company": "Linear"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job, err := waitRemoteJob(context.Background(), server.URL, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(job.Rows) != 1 || job.Rows[0].Result.Result["answer"] != "done" {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestDownloadRemoteCSV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/jobs/job-1/results.csv" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "text/csv")
		_, _ = writer.Write([]byte("company,answer\nLinear,done\n"))
	}))
	defer server.Close()

	var output bytes.Buffer
	if err := downloadRemoteCSV(context.Background(), server.URL, "job-1", &output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "company,answer\nLinear,done\n" {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func writeTestJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		panic(err)
	}
}
