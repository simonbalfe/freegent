package apify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultApifyBaseURL = "https://api.apify.com/v2"

type apifyRun struct {
	ID               string   `json:"id"`
	Status           string   `json:"status"`
	DefaultDatasetID string   `json:"defaultDatasetId"`
	UsageTotalUSD    *float64 `json:"usageTotalUsd"`
}

type apifyRunResult struct {
	Items      []map[string]any
	RunID      string
	DatasetID  string
	Status     string
	DurationMS int64
	CostUSD    *float64
}

func runApifyActor(ctx context.Context, actor string, input map[string]any) (apifyRunResult, error) {
	token := os.Getenv("APIFY_API_TOKEN")
	if token == "" {
		return apifyRunResult{}, errors.New("APIFY_API_TOKEN is not set")
	}
	baseURL := strings.TrimRight(envOr("APIFY_BASE_URL", defaultApifyBaseURL), "/")
	started := time.Now()
	run, err := startApifyRun(ctx, baseURL, token, actor, input)
	if err != nil {
		return apifyRunResult{}, err
	}
	deadline := time.Now().Add(150 * time.Second)
	for run.Status == "READY" || run.Status == "RUNNING" {
		if time.Now().After(deadline) {
			return apifyRunResult{}, fmt.Errorf("Apify %s timed out (run %s)", actor, run.ID)
		}
		run, err = getApifyRun(ctx, baseURL, token, actor, run.ID)
		if err != nil {
			return apifyRunResult{}, err
		}
	}
	items, err := getApifyDataset(ctx, baseURL, token, actor, run.DefaultDatasetID)
	if err != nil {
		return apifyRunResult{}, err
	}
	return apifyRunResult{Items: items, RunID: run.ID, DatasetID: run.DefaultDatasetID, Status: run.Status, DurationMS: time.Since(started).Milliseconds(), CostUSD: run.UsageTotalUSD}, nil
}

func startApifyRun(ctx context.Context, baseURL, token, actor string, input map[string]any) (apifyRun, error) {
	endpoint := fmt.Sprintf("%s/acts/%s/runs?token=%s", baseURL, url.PathEscape(actor), url.QueryEscape(token))
	var response struct {
		Data apifyRun `json:"data"`
	}
	if err := apifyJSON(ctx, http.MethodPost, endpoint, input, &response); err != nil {
		return apifyRun{}, fmt.Errorf("Apify %s: %w", actor, err)
	}
	if err := validateApifyRun(response.Data); err != nil {
		return apifyRun{}, err
	}
	return response.Data, nil
}

func getApifyRun(ctx context.Context, baseURL, token, actor, runID string) (apifyRun, error) {
	endpoint := fmt.Sprintf("%s/actor-runs/%s?token=%s&waitForFinish=30", baseURL, url.PathEscape(runID), url.QueryEscape(token))
	var response struct {
		Data apifyRun `json:"data"`
	}
	if err := apifyJSON(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return apifyRun{}, fmt.Errorf("Apify %s poll: %w", actor, err)
	}
	if err := validateApifyRun(response.Data); err != nil {
		return apifyRun{}, err
	}
	return response.Data, nil
}

func getApifyDataset(ctx context.Context, baseURL, token, actor, datasetID string) ([]map[string]any, error) {
	endpoint := fmt.Sprintf("%s/datasets/%s/items?token=%s", baseURL, url.PathEscape(datasetID), url.QueryEscape(token))
	var items []map[string]any
	if err := apifyJSON(ctx, http.MethodGet, endpoint, nil, &items); err != nil {
		return nil, fmt.Errorf("Apify %s items: %w", actor, err)
	}
	return items, nil
}

func apifyJSON(ctx context.Context, method, endpoint string, input any, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := (&http.Client{Timeout: 40 * time.Second}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 20<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%d: %s", response.StatusCode, clipLength(string(data), 300))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("Apify returned invalid JSON")
	}
	return nil
}

func validateApifyRun(run apifyRun) error {
	if run.ID == "" || run.Status == "" || run.DefaultDatasetID == "" {
		return errors.New("Apify returned an invalid run response")
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
