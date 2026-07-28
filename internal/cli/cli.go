package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type APIRequest struct {
	Instructions string           `json:"instructions"`
	Template     string           `json:"template"`
	Schema       json.RawMessage  `json:"schema"`
	Rows         []map[string]any `json:"rows"`
}

type remoteResult struct {
	Result map[string]any `json:"result"`
	Error  string         `json:"error,omitempty"`
}

type remoteRow struct {
	Result remoteResult `json:"result"`
}

type remoteJob struct {
	Status string      `json:"status"`
	Rows   []remoteRow `json:"rows"`
}

const defaultInstructions = "Research the requested answer using current web evidence. Do not guess unsupported facts."

const defaultSchema = `{"answer":"string"}`

const cliHelp = `freegent — submit research to the Freegent API

CSV:
  freegent --csv companies.csv \
    --prompt "Research {{company}} at {{domain}}."

  The enriched CSV is written to stdout.

One row:
  freegent --row '{"company":"Figma","domain":"figma.com"}' \
    --prompt "Research {{company}} at {{domain}}."

  The answer is written as JSON.

Options:
  --csv file       Upload a CSV batch
  --row json       Submit one JSON object
  --prompt text    Prompt using row fields such as {{company}}
  --schema json    Answer schema (default: {"answer":"string"})
  --detach         Return the job ID without waiting
  --api-url url    API address (default: http://localhost:8080)`

func Run(args []string) {
	flags := flag.NewFlagSet("freegent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	csvPath := flags.String("csv", "", "CSV file")
	rowValue := flags.String("row", "", "JSON row")
	prompt := flags.String("prompt", "", "research prompt")
	schemaValue := flags.String("schema", defaultSchema, "answer schema")
	apiURL := flags.String("api-url", "", "Freegent API")
	detach := flags.Bool("detach", false, "submit without waiting")
	help := flags.Bool("help", false, "show help")
	if err := flags.Parse(args); err != nil {
		failCLI(err)
	}
	if *help || len(args) == 0 {
		fmt.Println(cliHelp)
		return
	}
	if flags.NArg() != 0 {
		failCLI(fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " ")))
	}
	if (*csvPath == "") == (*rowValue == "") {
		failCLI(errors.New("provide exactly one of --csv or --row"))
	}
	if strings.TrimSpace(*prompt) == "" {
		failCLI(errors.New("--prompt is required"))
	}
	if !json.Valid([]byte(*schemaValue)) {
		failCLI(errors.New("--schema must be valid JSON"))
	}

	resolvedAPIURL := firstNonEmpty(*apiURL, os.Getenv("FREEGENT_API_URL"), "http://localhost:8080")
	ctx := context.Background()
	var jobID string
	var err error
	if *csvPath != "" {
		jobID, err = submitRemoteCSVJob(ctx, resolvedAPIURL, *csvPath, *prompt, *schemaValue)
	} else {
		request, requestError := buildRowRequest(*rowValue, *prompt, *schemaValue)
		if requestError != nil {
			failCLI(requestError)
		}
		jobID, err = submitRemoteJob(ctx, resolvedAPIURL, request)
	}
	if err != nil {
		failCLI(err)
	}

	dashboardURL := strings.TrimRight(resolvedAPIURL, "/") + "/dashboard/jobs/" + jobID
	downloadURL := strings.TrimRight(resolvedAPIURL, "/") + "/jobs/" + jobID + "/results.csv"
	if *detach {
		printJSON(map[string]any{
			"jobId":     jobID,
			"status":    "queued",
			"dashboard": dashboardURL,
			"download":  downloadURL,
		})
		return
	}

	fmt.Fprintf(os.Stderr, "job %s · %s\n", jobID, dashboardURL)
	job, err := waitRemoteJob(ctx, resolvedAPIURL, jobID)
	if err != nil {
		failCLI(err)
	}
	if *csvPath != "" {
		if err := downloadRemoteCSV(ctx, resolvedAPIURL, jobID, os.Stdout); err != nil {
			failCLI(err)
		}
		return
	}
	if len(job.Rows) != 1 {
		failCLI(fmt.Errorf("API returned %d rows for a single-row request", len(job.Rows)))
	}
	result := job.Rows[0].Result
	if result.Error != "" {
		failCLI(errors.New(result.Error))
	}
	printJSON(result.Result)
}

func buildRowRequest(rowValue, prompt, schemaValue string) (APIRequest, error) {
	var row map[string]any
	if err := json.Unmarshal([]byte(rowValue), &row); err != nil {
		return APIRequest{}, fmt.Errorf("--row must be a JSON object: %w", err)
	}
	if len(row) == 0 {
		return APIRequest{}, errors.New("--row must contain at least one field")
	}
	return APIRequest{
		Instructions: defaultInstructions,
		Template:     prompt,
		Schema:       json.RawMessage(schemaValue),
		Rows:         []map[string]any{row},
	}, nil
}

func submitRemoteCSVJob(ctx context.Context, baseURL, path, prompt, schemaValue string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"name":         filepath.Base(path),
		"instructions": defaultInstructions,
		"template":     prompt,
		"schema":       schemaValue,
	} {
		if err := form.WriteField(key, value); err != nil {
			return "", err
		}
	}
	part, err := form.CreateFormFile("csv", filepath.Base(path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/jobs"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	return submitJobRequest(request, endpoint)
}

func submitRemoteJob(ctx context.Context, baseURL string, request APIRequest) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/jobs"
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	return submitJobRequest(httpRequest, endpoint)
}

func submitJobRequest(request *http.Request, endpoint string) (string, error) {
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return "", fmt.Errorf("cannot reach Freegent API at %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("API %s: %s", response.Status, string(data))
	}
	var payload struct {
		JobID string `json:"jobId"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	if payload.JobID == "" {
		return "", errors.New("API returned an empty job ID")
	}
	return payload.JobID, nil
}

func getRemoteJob(ctx context.Context, baseURL, jobID string, summary bool) (remoteJob, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/jobs/" + jobID
	if summary {
		endpoint += "?summary=1"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return remoteJob{}, err
	}
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return remoteJob{}, fmt.Errorf("cannot reach Freegent API at %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 100<<20))
	if err != nil {
		return remoteJob{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return remoteJob{}, fmt.Errorf("API %s: %s", response.Status, string(data))
	}
	var job remoteJob
	if err := json.Unmarshal(data, &job); err != nil {
		return remoteJob{}, err
	}
	return job, nil
}

func waitRemoteJob(ctx context.Context, baseURL, jobID string) (remoteJob, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := getRemoteJob(ctx, baseURL, jobID, true)
		if err != nil {
			return remoteJob{}, err
		}
		if job.Status == "completed" || job.Status == "completed with errors" {
			return getRemoteJob(ctx, baseURL, jobID, false)
		}
		select {
		case <-ctx.Done():
			return remoteJob{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func downloadRemoteCSV(ctx context.Context, baseURL, jobID string, output io.Writer) error {
	endpoint := strings.TrimRight(baseURL, "/") + "/jobs/" + jobID + "/results.csv"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return fmt.Errorf("cannot reach Freegent API at %s: %w", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, readError := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if readError != nil {
			return readError
		}
		return fmt.Errorf("API %s: %s", response.Status, string(data))
	}
	_, err = io.Copy(output, response.Body)
	return err
}

func printJSON(value any) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		failCLI(err)
	}
	fmt.Println(string(encoded))
}

func failCLI(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
