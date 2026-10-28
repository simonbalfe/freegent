package pagelode

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

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type Attempt struct {
	Provider   string `json:"provider"`
	Outcome    string `json:"outcome"`
	Status     int    `json:"status,omitempty"`
	DurationMS int64  `json:"durationMs"`
	Detail     string `json:"detail,omitempty"`
}

type Page struct {
	URL         string    `json:"url"`
	Title       string    `json:"title,omitempty"`
	Content     string    `json:"content"`
	ContentType string    `json:"contentType"`
	Provider    string    `json:"provider"`
	Outcome     string    `json:"outcome"`
	Links       []string  `json:"links"`
	Attempts    []Attempt `json:"attempts"`
}

type SearchResult struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Snippet  string `json:"snippet,omitempty"`
}

type OverviewSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Overview struct {
	Found    bool             `json:"found"`
	Markdown string           `json:"markdown,omitempty"`
	Sources  []OverviewSource `json:"sources,omitempty"`
}

type Search struct {
	Query      string         `json:"query"`
	Results    []SearchResult `json:"results"`
	AIOverview *Overview      `json:"aiOverview,omitempty"`
	Provider   string         `json:"provider"`
}

type EmailSource struct {
	URL     string `json:"url"`
	FoundIn string `json:"foundIn"`
}

type Email struct {
	Address string        `json:"address"`
	Sources []EmailSource `json:"sources"`
}

type Emails struct {
	URL      string   `json:"url"`
	Outcome  string   `json:"outcome"`
	Emails   []Email  `json:"emails"`
	Warnings []string `json:"warnings,omitempty"`
}

type MapEntry struct {
	URL    string `json:"url"`
	Source string `json:"source"`
}

type Template struct {
	Pattern string   `json:"pattern"`
	Count   int      `json:"count"`
	Samples []string `json:"samples"`
}

type SiteMap struct {
	URL       string     `json:"url"`
	Outcome   string     `json:"outcome"`
	URLs      []MapEntry `json:"urls"`
	Templates []Template `json:"templates,omitempty"`
	Warnings  []string   `json:"warnings,omitempty"`
}

func (c Client) Configured() bool { return strings.TrimSpace(c.BaseURL) != "" }

func (c Client) Extract(ctx context.Context, rawURL string) (Page, error) {
	target, err := ValidURL(rawURL)
	if err != nil {
		return Page{}, err
	}
	var page Page
	if err := c.post(ctx, "/extract", map[string]string{"url": target}, &page); err != nil {
		return Page{}, err
	}
	if page.Outcome != "ok" || strings.TrimSpace(page.Content) == "" {
		return page, fmt.Errorf("PageLode could not extract the URL: %s", attemptDetail(page))
	}
	return page, nil
}

func (c Client) Google(ctx context.Context, query string) (Search, error) {
	var result Search
	err := c.post(ctx, "/google", map[string]any{"query": query, "aiOverview": true}, &result)
	return result, err
}

func (c Client) Emails(ctx context.Context, rawURL string, maxPages int) (Emails, error) {
	target, err := ValidURL(rawURL)
	if err != nil {
		return Emails{}, err
	}
	var result Emails
	err = c.post(ctx, "/emails", map[string]any{"url": target, "maxPages": maxPages}, &result)
	return result, err
}

func (c Client) Map(ctx context.Context, rawURL string, limit int) (SiteMap, error) {
	target, err := ValidURL(rawURL)
	if err != nil {
		return SiteMap{}, err
	}
	var result SiteMap
	err = c.post(ctx, "/map", map[string]any{"url": target, "limit": limit}, &result)
	return result, err
}

func ValidURL(rawURL string) (string, error) {
	target, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return "", errors.New("URL must use http or https")
	}
	return target.String(), nil
}

func (c Client) post(ctx context.Context, path string, body, output any) error {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		return errors.New("PAGELODE_URL is not configured")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 150 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("PageLode request failed: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("PageLode %s %s: %s", path, response.Status, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("invalid PageLode response: %w", err)
	}
	return nil
}

func attemptDetail(page Page) string {
	for index := len(page.Attempts) - 1; index >= 0; index-- {
		if page.Attempts[index].Detail != "" {
			return page.Attempts[index].Detail
		}
	}
	if page.Outcome != "" {
		return page.Outcome
	}
	return "unknown failure"
}
