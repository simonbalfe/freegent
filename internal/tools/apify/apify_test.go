package apify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestApifyActorStartsPollsAndReadsDataset(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		if request.URL.Query().Get("token") != "secret" {
			t.Fatalf("missing Apify token")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/acts/example~actor/runs":
			_, _ = writer.Write([]byte(`{"data":{"id":"run-1","status":"RUNNING","defaultDatasetId":"dataset-1"}}`))
		case "/actor-runs/run-1":
			if request.URL.Query().Get("waitForFinish") != "30" {
				t.Fatalf("missing waitForFinish")
			}
			_, _ = writer.Write([]byte(`{"data":{"id":"run-1","status":"SUCCEEDED","defaultDatasetId":"dataset-1","usageTotalUsd":0.02654}}`))
		case "/datasets/dataset-1/items":
			_, _ = writer.Write([]byte(`[{"name":"Linear"}]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("APIFY_API_TOKEN", "secret")
	t.Setenv("APIFY_BASE_URL", server.URL)

	result, err := runApifyActor(context.Background(), "example~actor", map[string]any{"query": "Linear"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" || result.Items[0]["name"] != "Linear" || result.CostUSD == nil || *result.CostUSD != 0.02654 {
		t.Fatalf("unexpected actor result: %+v", result)
	}
	expected := []string{
		"POST /acts/example~actor/runs",
		"GET /actor-runs/run-1",
		"GET /datasets/dataset-1/items",
	}
	if strings.Join(requests, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestLinkedInCompanyToolMapsCompactOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/acts/harvestapi~linkedin-company/runs":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if len(stringSlice(input["searches"])) != 1 || stringSlice(input["searches"])[0] != "Linear" {
				t.Fatalf("unexpected actor input: %#v", input)
			}
			_, _ = writer.Write([]byte(`{"data":{"id":"run-1","status":"SUCCEEDED","defaultDatasetId":"dataset-1"}}`))
		case "/datasets/dataset-1/items":
			_, _ = writer.Write([]byte(`[{"name":"Linear","linkedinUrl":"https://www.linkedin.com/company/linear","website":"https://linear.app","employeeCount":260,"employeeCountRange":{"start":201,"end":500},"followerCount":150000,"foundedOn":{"year":2019},"industries":[{"title":"Software Development"}],"specialities":["Issue tracking","Project management"],"locations":[{"city":"San Francisco","country":"United States","headquarter":true}]}]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	t.Setenv("APIFY_API_TOKEN", "secret")
	t.Setenv("APIFY_BASE_URL", server.URL)

	result, err := linkedinCompanyTool().Run(context.Background(), map[string]any{"company": "Linear"})
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Company map[string]any `json:"company"`
	}
	if err := json.Unmarshal([]byte(result.Text), &output); err != nil {
		t.Fatal(err)
	}
	if output.Company["employeeCount"] != float64(260) || output.Company["employeeCountRange"] != "201-500" || output.Company["foundedYear"] != float64(2019) {
		t.Fatalf("unexpected mapped company: %#v", output.Company)
	}
	if result.Provider != "apify:harvestapi~linkedin-company" || len(result.URLs) != 1 {
		t.Fatalf("unexpected tool metadata: %+v", result)
	}
}

func TestMalformedApifyRunIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"id":"run-1","status":"RUNNING"}}`))
	}))
	defer server.Close()
	t.Setenv("APIFY_API_TOKEN", "secret")
	t.Setenv("APIFY_BASE_URL", server.URL)

	_, err := runApifyActor(context.Background(), "example~actor", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "invalid run response") {
		t.Fatalf("expected boundary validation error, got %v", err)
	}
}

func TestLiveLinkedInCompany(t *testing.T) {
	if os.Getenv("RUN_LIVE_APIFY") == "" || os.Getenv("APIFY_API_TOKEN") == "" {
		t.Skip("RUN_LIVE_APIFY and APIFY_API_TOKEN are required")
	}
	result, err := linkedinCompanyTool().Run(context.Background(), map[string]any{"company": "Linear"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, `"company"`) || len(result.URLs) == 0 {
		t.Fatalf("unexpected live LinkedIn company result: %+v", result)
	}
}
