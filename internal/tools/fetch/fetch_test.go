package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToolCallsOpenExtract(t *testing.T) {
	openExtract := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/extract" {
			t.Fatalf("unexpected OpenExtract request: %s", request.URL.String())
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"content":  "Readable page content",
			"provider": "patchright",
			"outcome":  "ok",
			"links":    []string{"https://example.com/about"},
			"attempts": []map[string]any{{"provider": "patchright", "outcome": "ok", "durationMs": 20}},
		})
	}))
	defer openExtract.Close()
	t.Setenv("OPENEXTRACT_URL", openExtract.URL)

	result, err := (Tool{}).Run(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "patchright" || len(result.Attempts) != 1 || len(result.SeenURLs) != 1 {
		t.Fatalf("unexpected OpenExtract result: %+v", result)
	}
}
