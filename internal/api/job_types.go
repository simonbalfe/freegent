package api

import (
	"encoding/json"
	"time"
)

type DashboardJob struct {
	ID          string           `json:"id"`
	Name        string           `json:"name,omitempty"`
	Request     APIRequest       `json:"-"`
	Template    string           `json:"template,omitempty"`
	Schema      json.RawMessage  `json:"schema,omitempty"`
	Status      string           `json:"status"`
	Total       int              `json:"total"`
	Completed   int              `json:"completed"`
	CreatedAt   time.Time        `json:"createdAt"`
	StartedAt   time.Time        `json:"startedAt,omitempty"`
	FinishedAt  time.Time        `json:"finishedAt,omitempty"`
	Rows        []DashboardRow   `json:"rows"`
	Events      []DashboardEvent `json:"events"`
	LatestEvent string           `json:"latestEvent,omitempty"`
}

type DashboardEvent struct {
	At      time.Time `json:"at"`
	Row     int       `json:"row,omitempty"`
	Message string    `json:"message"`
}

type DashboardRow struct {
	Index      int            `json:"index"`
	Input      map[string]any `json:"input"`
	Status     string         `json:"status"`
	Result     APIResult      `json:"result"`
	StartedAt  time.Time      `json:"startedAt,omitempty"`
	FinishedAt time.Time      `json:"finishedAt,omitempty"`
}

type DashboardStats struct {
	Rows              int                   `json:"rows"`
	Completed         int                   `json:"completed"`
	Failed            int                   `json:"failed"`
	Skipped           int                   `json:"skipped"`
	Tokens            TokenUsage            `json:"tokens"`
	AgentSteps        int                   `json:"agentSteps"`
	Sources           int                   `json:"sources"`
	DurationMS        int64                 `json:"durationMs"`
	Costs             CostUsage             `json:"costs"`
	UnpricedApifyRuns int                   `json:"unpricedApifyRuns"`
	SerperQueries     int                   `json:"serperQueries"`
	Models            []DashboardModelStats `json:"models"`
}

type DashboardModelStats struct {
	Model                string  `json:"model"`
	InputTokens          int     `json:"inputTokens"`
	OutputTokens         int     `json:"outputTokens"`
	OpenRouterUSD        float64 `json:"openRouterUsd"`
	UnpricedInputTokens  int     `json:"unpricedInputTokens"`
	UnpricedOutputTokens int     `json:"unpricedOutputTokens"`
}
