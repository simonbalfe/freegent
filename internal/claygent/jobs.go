package claygent

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*DashboardJob
}

type DashboardJob struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	Total       int            `json:"total"`
	Completed   int            `json:"completed"`
	StartedAt   time.Time      `json:"startedAt,omitempty"`
	FinishedAt  time.Time      `json:"finishedAt,omitempty"`
	Rows        []DashboardRow `json:"rows"`
	LatestEvent string         `json:"latestEvent,omitempty"`
}

type DashboardRow struct {
	Index    int            `json:"index"`
	Input    map[string]any `json:"input"`
	Status   string         `json:"status"`
	Result   APIResult      `json:"result"`
	AgentLog []Step         `json:"agentLog"`
}

func NewJobStore() *JobStore {
	return &JobStore{jobs: map[string]*DashboardJob{}}
}

func (s *JobStore) Start(request APIRequest, rows []map[string]any) string {
	id := newRunID()
	job := &DashboardJob{ID: id, Status: "queued", Total: len(rows), Rows: make([]DashboardRow, len(rows))}
	for index, row := range rows {
		job.Rows[index] = DashboardRow{Index: index, Input: row, Status: "queued"}
	}
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	go s.run(id, request, rows)
	return id
}

func (s *JobStore) run(id string, request APIRequest, rows []map[string]any) {
	s.update(id, func(job *DashboardJob) {
		job.Status = "running"
		job.StartedAt = time.Now()
		job.LatestEvent = "Agent run started"
	})
	results := runBatchWithProgress(context.Background(), request, rows, runOne, func(index int, result APIResult) {
		s.update(id, func(job *DashboardJob) {
			status := "completed"
			if result.Error != "" {
				status = "failed"
			} else if result.Skipped {
				status = "skipped"
			}
			job.Rows[index].Status = status
			job.Rows[index].Result = result
			job.Rows[index].AgentLog = result.AgentLog
			job.Completed++
			job.LatestEvent = "Row " + strconv.Itoa(index+1) + " " + status
		})
	})
	s.update(id, func(job *DashboardJob) {
		job.Status = "completed"
		for _, result := range results {
			if result.Error != "" {
				job.Status = "completed with errors"
				break
			}
		}
		job.FinishedAt = time.Now()
		job.LatestEvent = "Agent run finished"
	})
}

func (s *JobStore) Get(id string) (DashboardJob, bool) {
	s.mu.RLock()
	job, ok := s.jobs[id]
	if !ok {
		s.mu.RUnlock()
		return DashboardJob{}, false
	}
	copy := *job
	copy.Rows = append([]DashboardRow(nil), job.Rows...)
	s.mu.RUnlock()
	return copy, true
}

func (s *JobStore) update(id string, update func(*DashboardJob)) {
	s.mu.Lock()
	if job, ok := s.jobs[id]; ok {
		update(job)
	}
	s.mu.Unlock()
}
