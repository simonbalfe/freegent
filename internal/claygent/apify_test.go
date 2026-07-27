package claygent

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
			_, _ = writer.Write([]byte(`{"data":{"id":"run-1","status":"SUCCEEDED","defaultDatasetId":"dataset-1"}}`))
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
	if result.Status != "SUCCEEDED" || result.Items[0]["name"] != "Linear" {
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

func TestApifyToolsAreEnvironmentGated(t *testing.T) {
	t.Setenv("APIFY_API_TOKEN", "")
	if tools := defaultTools(); len(tools) != 2 {
		t.Fatalf("expected web-only tools, got %d", len(tools))
	}
	t.Setenv("APIFY_API_TOKEN", "secret")
	tools := defaultTools()
	if len(tools) != 8 {
		t.Fatalf("expected web and enrichment tools, got %d", len(tools))
	}
	for _, name := range []string{"linkedin_profile", "linkedin_posts", "linkedin_post_reactions", "linkedin_find_people", "linkedin_company", "crunchbase_company"} {
		if tools[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
}

type fabricatedURLModel struct{}

func (fabricatedURLModel) Next(context.Context, []Message, Action) (ModelResponse, error) {
	return ModelResponse{ToolCalls: []ToolCall{{ID: "one", Name: "linkedin_profile", Input: map[string]any{"url": "https://www.linkedin.com/in/invented"}}}}, nil
}

func (fabricatedURLModel) Finalize(context.Context, string, Action, []Evidence) (ModelResponse, error) {
	return ModelResponse{}, nil
}

func TestAgentRejectsFabricatedEnrichmentURL(t *testing.T) {
	schema, err := compileOutputSchema(json.RawMessage(`{"name":"string?"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = (Agent{
		Model:    fabricatedURLModel{},
		Tools:    map[string]Tool{"linkedin_profile": linkedinProfileTool()},
		MaxSteps: 1,
	}).Run(context.Background(), Action{Instructions: "Research.", Template: "Research.", Schema: schema.Canonical, Validator: schema}, Row{})
	if err == nil || !strings.Contains(err.Error(), "refusing unverified URL") {
		t.Fatalf("expected provenance rejection, got %v", err)
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
