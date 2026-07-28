package api

import (
	"context"
	"time"
)

type JobBackend interface {
	Close() error
	Ping(context.Context) error
	Start(context.Context, APIRequest, []map[string]any) (string, error)
	Get(string) (DashboardJob, error)
	GetPage(string, int, int) (DashboardJob, error)
	GetSummary(string) (DashboardJob, error)
	List(int) ([]DashboardJob, error)
	Wait(context.Context, string) (DashboardJob, error)
}

type JobRunner func(context.Context, APIRequest, map[string]any, func(AgentEvent)) APIResult

type DashboardJob struct {
	ID          string           `json:"id"`
	Name        string           `json:"name,omitempty"`
	Request     APIRequest       `json:"-"`
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
	AgentLog   []Step         `json:"agentLog"`
	StartedAt  time.Time      `json:"startedAt,omitempty"`
	FinishedAt time.Time      `json:"finishedAt,omitempty"`
}

func jobTerminal(status string) bool {
	return status == "completed" || status == "completed with errors"
}

func jobResults(job DashboardJob) []APIResult {
	results := make([]APIResult, len(job.Rows))
	for _, row := range job.Rows {
		if row.Index >= 0 && row.Index < len(results) {
			results[row.Index] = row.Result
		}
	}
	return results
}
