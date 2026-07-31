package api

import (
	"time"
)

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
	StartedAt  time.Time      `json:"startedAt,omitempty"`
	FinishedAt time.Time      `json:"finishedAt,omitempty"`
}
