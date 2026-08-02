package openextract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BaseURL string
	Client  *http.Client
}

type Attempt struct {
	Provider   string `json:"provider"`
	Outcome    string `json:"outcome"`
	Status     int    `json:"status,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Detail     string `json:"detail,omitempty"`
}

type Result struct {
	URL         string
	Text        string
	ContentType string
	Links       []string
	Provider    string
	Attempts    []Attempt
}

type response struct {
	URL         string    `json:"url"`
	Content     string    `json:"content"`
	ContentType string    `json:"contentType"`
	Provider    string    `json:"provider"`
	Outcome     string    `json:"outcome"`
	Links       []string  `json:"links"`
	Attempts    []Attempt `json:"attempts"`
}

func Extract(ctx context.Context, rawURL string, config Config) (Result, error) {
	target, err := url.Parse(rawURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
		return Result{}, errors.New("URL must use http or https")
	}
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		return Result{}, errors.New("OPENEXTRACT_URL is not configured")
	}
	body, err := json.Marshal(map[string]string{"url": target.String()})
	if err != nil {
		return Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/extract", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 150 * time.Second}
	}
	httpResponse, err := client.Do(request)
	if err != nil {
		return Result{}, fmt.Errorf("OpenExtract request failed: %w", err)
	}
	defer httpResponse.Body.Close()
	data, err := io.ReadAll(io.LimitReader(httpResponse.Body, 16<<20))
	if err != nil {
		return Result{}, err
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return Result{}, fmt.Errorf("OpenExtract %s: %s", httpResponse.Status, strings.TrimSpace(string(data)))
	}
	var payload response
	if err := json.Unmarshal(data, &payload); err != nil {
		return Result{}, fmt.Errorf("invalid OpenExtract response: %w", err)
	}
	result := Result{
		URL:         payload.URL,
		Text:        payload.Content,
		ContentType: payload.ContentType,
		Links:       payload.Links,
		Provider:    payload.Provider,
		Attempts:    payload.Attempts,
	}
	if payload.Outcome != "ok" || strings.TrimSpace(payload.Content) == "" {
		return result, fmt.Errorf("OpenExtract could not extract the URL: %s", attemptDetail(payload))
	}
	return result, nil
}

func attemptDetail(payload response) string {
	for index := len(payload.Attempts) - 1; index >= 0; index-- {
		if payload.Attempts[index].Detail != "" {
			return payload.Attempts[index].Detail
		}
	}
	if payload.Outcome != "" {
		return payload.Outcome
	}
	return "unknown failure"
}
