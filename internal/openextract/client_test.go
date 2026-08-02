package openextract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestExtractCallsOpenExtractService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/extract" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var input map[string]string
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input["url"] != "https://example.com/page" {
			t.Fatalf("unexpected URL: %q", input["url"])
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"url":         "https://example.com/page",
			"content":     "Useful extracted content",
			"contentType": "html",
			"provider":    "patchright",
			"outcome":     "ok",
			"links":       []string{"https://example.com/about"},
			"attempts": []map[string]any{
				{"provider": "impit", "outcome": "blocked", "durationMs": 10},
				{"provider": "patchright", "outcome": "ok", "durationMs": 20},
			},
		})
	}))
	defer server.Close()

	result, err := Extract(context.Background(), "https://example.com/page", Config{BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != "https://example.com/page" || result.ContentType != "html" || result.Provider != "patchright" || len(result.Links) != 1 || len(result.Attempts) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExtractReturnsServiceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"provider": "patchright+solver",
			"outcome":  "failed",
			"attempts": []map[string]any{
				{"provider": "patchright+solver", "outcome": "error", "durationMs": 20, "detail": "provider unavailable"},
			},
		})
	}))
	defer server.Close()

	result, err := Extract(context.Background(), "https://example.com", Config{BaseURL: server.URL})
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Provider != "patchright+solver" || len(result.Attempts) != 1 {
		t.Fatalf("failure result lost extraction attempts: %+v", result)
	}
}

func TestLiveOpenExtract(t *testing.T) {
	baseURL := os.Getenv("OPENEXTRACT_URL")
	if os.Getenv("RUN_LIVE_OPENEXTRACT") == "" || baseURL == "" {
		t.Skip("RUN_LIVE_OPENEXTRACT and OPENEXTRACT_URL are required")
	}
	cases := []struct {
		name     string
		url      string
		contains string
		provider string
	}{
		{name: "direct HTML", url: "https://www.iana.org/help/example-domains", contains: "example domain", provider: "impit"},
		{name: "browser HTML", url: "https://quotes.toscrape.com/js/", contains: "albert einstein", provider: "patchright"},
		{name: "PDF", url: "https://pdfobject.com/pdf/sample.pdf", contains: "sample pdf", provider: "impit"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := Extract(context.Background(), testCase.url, Config{BaseURL: baseURL})
			if err != nil {
				t.Fatal(err)
			}
			if result.Provider != testCase.provider || !strings.Contains(strings.ToLower(result.Text), testCase.contains) {
				t.Fatalf("unexpected live extraction: %+v", result)
			}
		})
	}
}
