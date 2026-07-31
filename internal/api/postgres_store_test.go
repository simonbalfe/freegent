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

func TestDashboardRenders(t *testing.T) {
	response := httptest.NewRecorder()
	renderDashboard(response, "job", DashboardJob{ID: "job-1", Status: "queued", Total: 1})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "/dashboard/jobs/job-1/status") {
		t.Fatalf("unexpected dashboard response: status=%d body=%q", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	renderDashboard(response, "dashboard", []DashboardJob{})
	if !strings.Contains(response.Body.String(), `Research {{subject}}`) || !strings.Contains(response.Body.String(), `{"answer":"string"}`) {
		t.Fatalf("dashboard defaults were not preserved: %q", response.Body.String())
	}
}
