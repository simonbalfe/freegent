package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/simonbalfe/freegent/internal/pagelode"
)

func TestToolCallsPageLode(t *testing.T) {
	pageLode := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/extract" {
			t.Fatalf("unexpected PageLode request: %s", request.URL.String())
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"content":  "Readable page content",
			"provider": "patchright",
			"outcome":  "ok",
			"links":    []string{"https://example.com/about"},
			"attempts": []map[string]any{{"provider": "patchright", "outcome": "ok", "durationMs": 20}},
		})
	}))
	defer pageLode.Close()

	result, err := New(pagelode.Client{BaseURL: pageLode.URL}, "", "").Run(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "patchright" || len(result.Attempts) != 1 || len(result.SeenURLs) != 1 {
		t.Fatalf("unexpected PageLode result: %+v", result)
	}
}

func TestToolFallsBackFromPageLodeThroughManagedProviders(t *testing.T) {
	tests := []struct {
		name         string
		exaFails     bool
		wantProvider string
		wantText     string
		wantAttempts []string
	}{
		{name: "Exa succeeds", wantProvider: "exa", wantText: "Readable Exa content", wantAttempts: []string{"impit", "exa"}},
		{name: "Tavily follows Exa failure", exaFails: true, wantProvider: "tavily", wantText: "Readable Tavily content", wantAttempts: []string{"impit", "exa", "tavily"}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			pageLode := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(writer).Encode(map[string]any{
					"provider": "impit",
					"outcome":  "failed",
					"attempts": []map[string]any{{"provider": "impit", "outcome": "blocked", "durationMs": 10}},
				})
			}))
			defer pageLode.Close()

			providers := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/contents":
					if request.Header.Get("x-api-key") != "exa-key" || testCase.exaFails {
						http.Error(writer, "Exa unavailable", http.StatusBadGateway)
						return
					}
					_ = json.NewEncoder(writer).Encode(map[string]any{"results": []map[string]any{{"text": "Readable Exa content"}}})
				case "/extract":
					if request.Header.Get("Authorization") != "Bearer tavily-key" {
						http.Error(writer, "missing authentication", http.StatusUnauthorized)
						return
					}
					_ = json.NewEncoder(writer).Encode(map[string]any{"results": []map[string]any{{"raw_content": "Readable Tavily content"}}})
				default:
					http.NotFound(writer, request)
				}
			}))
			defer providers.Close()

			result, err := (Tool{
				pageLode:       pagelode.Client{BaseURL: pageLode.URL},
				exaAPIKey:      "exa-key",
				tavilyAPIKey:   "tavily-key",
				client:         providers.Client(),
				exaEndpoint:    providers.URL + "/contents",
				tavilyEndpoint: providers.URL + "/extract",
			}).Run(context.Background(), map[string]any{"url": "https://example.com"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Provider != testCase.wantProvider || result.Text != testCase.wantText {
				t.Fatalf("unexpected fallback result: %+v", result)
			}
			providersUsed := make([]string, 0, len(result.Attempts))
			for _, attempt := range result.Attempts {
				providersUsed = append(providersUsed, attempt.Provider)
			}
			if !slices.Equal(providersUsed, testCase.wantAttempts) {
				t.Fatalf("fallback order got %v, want %v", providersUsed, testCase.wantAttempts)
			}
		})
	}
}
