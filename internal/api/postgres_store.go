package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

const postgresJobSchema = `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL DEFAULT '',
	request_json JSONB NOT NULL,
	status TEXT NOT NULL,
	total INTEGER NOT NULL,
	completed INTEGER NOT NULL DEFAULT 0,
	failed INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL,
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	latest_event TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS job_rows (
	job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	row_index INTEGER NOT NULL,
	input_json JSONB NOT NULL,
	status TEXT NOT NULL,
	result_json JSONB,
	step_results JSONB NOT NULL DEFAULT '{}'::jsonb,
	attempts INTEGER NOT NULL DEFAULT 0,
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	PRIMARY KEY (job_id, row_index)
);
ALTER TABLE job_rows ADD COLUMN IF NOT EXISTS step_results JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE TABLE IF NOT EXISTS job_events (
	id BIGSERIAL PRIMARY KEY,
	job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
	row_number INTEGER NOT NULL DEFAULT 0,
	at TIMESTAMPTZ NOT NULL,
	message TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS jobs_created_at_idx ON jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS job_rows_status_idx ON job_rows(status, job_id, row_index);
CREATE INDEX IF NOT EXISTS job_events_job_id_idx ON job_events(job_id, id);
`

const operationInsertChunk = 1000

type OperationArgs struct {
	JobID    string `json:"job_id" river:"unique"`
	RowIndex int    `json:"row_index" river:"unique"`
}

func (OperationArgs) Kind() string {
	return "freegent_operation"
}

func (OperationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 5,
		Queue:       "research",
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
		},
	}
}

type PostgresStore struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
}

func OpenBackend(ctx context.Context) (*PostgresStore, error) {
	databaseURL := os.Getenv("FREEGENT_DATABASE_URL")
	if databaseURL == "" {
		return nil, errors.New("FREEGENT_DATABASE_URL is required")
	}
	return OpenPostgresStore(ctx, databaseURL)
}

func OpenPostgresStore(ctx context.Context, databaseURL string) (*PostgresStore, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	maxConnections := int32(30)
	if raw := os.Getenv("FREEGENT_DATABASE_MAX_CONNS"); raw != "" {
		value, parseError := strconv.ParseInt(raw, 10, 32)
		if parseError != nil || value < 1 {
			return nil, errors.New("FREEGENT_DATABASE_MAX_CONNS must be a positive integer")
		}
		maxConnections = int32(value)
	}
	config.MaxConns = maxConnections
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	driver := riverpgxv5.New(pool)
	migrator, err := rivermigrate.New(driver, nil)
	if err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, postgresJobSchema); err != nil {
		pool.Close()
		return nil, err
	}
	client, err := river.NewClient(driver, &river.Config{})
	if err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresStore{pool: pool, client: client}, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PostgresStore) Start(ctx context.Context, request APIRequest, rows []map[string]any) (string, error) {
	if len(rows) == 0 {
		return "", errors.New("at least one row is required")
	}
	id := newRunID()
	storedRequest := request
	storedRequest.Rows = nil
	storedRequest.Input = nil
	requestJSON, err := json.Marshal(storedRequest)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO jobs (id, name, request_json, status, total, created_at, latest_event)
		 VALUES ($1, $2, $3, 'queued', $4, NOW(), 'Job queued')`,
		id,
		request.Name,
		requestJSON,
		len(rows),
	); err != nil {
		return "", err
	}
	copyRows := make([][]any, len(rows))
	for index, row := range rows {
		inputJSON, err := json.Marshal(row)
		if err != nil {
			return "", err
		}
		copyRows[index] = []any{id, index, inputJSON, "queued"}
	}
	if _, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"job_rows"},
		[]string{"job_id", "row_index", "input_json", "status"},
		pgx.CopyFromRows(copyRows),
	); err != nil {
		return "", err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, 0, NOW(), 'Job queued')`,
		id,
	); err != nil {
		return "", err
	}
	for start := 0; start < len(rows); start += operationInsertChunk {
		end := min(start+operationInsertChunk, len(rows))
		params := make([]river.InsertManyParams, end-start)
		for index := start; index < end; index++ {
			params[index-start] = river.InsertManyParams{
				Args: OperationArgs{JobID: id, RowIndex: index},
			}
		}
		if _, err := s.client.InsertManyTx(ctx, tx, params); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *PostgresStore) Get(id string) (DashboardJob, error) {
	return s.get(id, 0, 0)
}

func (s *PostgresStore) GetPage(id string, limit int, offset int) (DashboardJob, error) {
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return s.get(id, limit, offset)
}

func (s *PostgresStore) get(id string, limit int, offset int) (DashboardJob, error) {
	job, err := s.GetSummary(id)
	if err != nil {
		return DashboardJob{}, err
	}
	rowQuery := `SELECT row_index, input_json, status, result_json, started_at, finished_at
		FROM job_rows WHERE job_id = $1 ORDER BY row_index`
	rowArgs := []any{id}
	if limit > 0 {
		rowQuery += ` LIMIT $2 OFFSET $3`
		rowArgs = append(rowArgs, limit, offset)
	}
	rows, err := s.pool.Query(context.Background(), rowQuery, rowArgs...)
	if err != nil {
		return DashboardJob{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row DashboardRow
		var inputJSON []byte
		var resultJSON []byte
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&row.Index,
			&inputJSON,
			&row.Status,
			&resultJSON,
			&startedAt,
			&finishedAt,
		); err != nil {
			return DashboardJob{}, err
		}
		if err := json.Unmarshal(inputJSON, &row.Input); err != nil {
			return DashboardJob{}, err
		}
		if len(resultJSON) > 0 {
			if err := json.Unmarshal(resultJSON, &row.Result); err != nil {
				return DashboardJob{}, err
			}
		}
		if startedAt.Valid {
			row.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			row.FinishedAt = finishedAt.Time
		}
		job.Rows = append(job.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return DashboardJob{}, err
	}
	eventQuery := `SELECT at, row_number, message FROM job_events WHERE job_id = $1 ORDER BY id`
	if limit > 0 {
		eventQuery = `SELECT at, row_number, message FROM (
		 SELECT id, at, row_number, message FROM job_events
		 WHERE job_id = $1 ORDER BY id DESC LIMIT 200
		) recent_events ORDER BY id`
	}
	events, err := s.pool.Query(context.Background(), eventQuery, id)
	if err != nil {
		return DashboardJob{}, err
	}
	defer events.Close()
	for events.Next() {
		var event DashboardEvent
		if err := events.Scan(&event.At, &event.Row, &event.Message); err != nil {
			return DashboardJob{}, err
		}
		job.Events = append(job.Events, event)
	}
	return job, events.Err()
}

func (s *PostgresStore) GetSummary(id string) (DashboardJob, error) {
	var job DashboardJob
	var requestJSON []byte
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	err := s.pool.QueryRow(
		context.Background(),
		`SELECT id, name, request_json, status, total, completed, created_at, started_at, finished_at, latest_event
		 FROM jobs WHERE id = $1`,
		id,
	).Scan(
		&job.ID,
		&job.Name,
		&requestJSON,
		&job.Status,
		&job.Total,
		&job.Completed,
		&job.CreatedAt,
		&startedAt,
		&finishedAt,
		&job.LatestEvent,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DashboardJob{}, sql.ErrNoRows
	}
	if err != nil {
		return DashboardJob{}, err
	}
	if err := json.Unmarshal(requestJSON, &job.Request); err != nil {
		return DashboardJob{}, err
	}
	if startedAt.Valid {
		job.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = finishedAt.Time
	}
	return job, nil
}

func (s *PostgresStore) List(limit int) ([]DashboardJob, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.pool.Query(
		context.Background(),
		`SELECT id, name, status, total, completed, created_at, started_at, finished_at, latest_event
		 FROM jobs ORDER BY created_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []DashboardJob{}
	for rows.Next() {
		var job DashboardJob
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&job.ID,
			&job.Name,
			&job.Status,
			&job.Total,
			&job.Completed,
			&job.CreatedAt,
			&startedAt,
			&finishedAt,
			&job.LatestEvent,
		); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			job.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			job.FinishedAt = finishedAt.Time
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *PostgresStore) beginOperation(
	ctx context.Context,
	args OperationArgs,
	attempt int,
) (APIRequest, map[string]any, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return APIRequest{}, nil, false, err
	}
	defer tx.Rollback(ctx)
	var requestJSON []byte
	var inputJSON []byte
	var status string
	err = tx.QueryRow(
		ctx,
		`SELECT jobs.request_json, job_rows.input_json, job_rows.status
		 FROM job_rows
		 JOIN jobs ON jobs.id = job_rows.job_id
		 WHERE job_rows.job_id = $1 AND job_rows.row_index = $2
		 FOR UPDATE OF job_rows`,
		args.JobID,
		args.RowIndex,
	).Scan(&requestJSON, &inputJSON, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIRequest{}, nil, false, sql.ErrNoRows
	}
	if err != nil {
		return APIRequest{}, nil, false, err
	}
	if rowTerminal(status) {
		return APIRequest{}, nil, true, tx.Commit(ctx)
	}
	var request APIRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return APIRequest{}, nil, false, err
	}
	var input map[string]any
	if err := json.Unmarshal(inputJSON, &input); err != nil {
		return APIRequest{}, nil, false, err
	}
	message := fmt.Sprintf("Row %d started attempt %d", args.RowIndex+1, attempt)
	if _, err := tx.Exec(
		ctx,
		`UPDATE job_rows SET status = 'running', attempts = $3,
		 started_at = COALESCE(started_at, NOW()), finished_at = NULL
		 WHERE job_id = $1 AND row_index = $2`,
		args.JobID,
		args.RowIndex,
		attempt,
	); err != nil {
		return APIRequest{}, nil, false, err
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE jobs SET status = 'running', started_at = COALESCE(started_at, NOW()),
		 finished_at = NULL, latest_event = $2 WHERE id = $1`,
		args.JobID,
		message,
	); err != nil {
		return APIRequest{}, nil, false, err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, $2, NOW(), $3)`,
		args.JobID,
		args.RowIndex+1,
		message,
	); err != nil {
		return APIRequest{}, nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return APIRequest{}, nil, false, err
	}
	return request, input, false, nil
}

func (s *PostgresStore) appendOperationEvent(ctx context.Context, args OperationArgs, message string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE jobs SET latest_event = $2 WHERE id = $1`, args.JobID, message); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, $2, NOW(), $3)`,
		args.JobID,
		args.RowIndex+1,
		message,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) operationStep(ctx context.Context, args OperationArgs, key string) (json.RawMessage, bool, error) {
	var raw []byte
	err := s.pool.QueryRow(
		ctx,
		`SELECT COALESCE(step_results -> ($3::text), 'null'::jsonb)
		 FROM job_rows WHERE job_id = $1 AND row_index = $2`,
		args.JobID,
		args.RowIndex,
		key,
	).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, sql.ErrNoRows
	}
	if err != nil {
		return nil, false, err
	}
	if string(raw) == "null" {
		return nil, false, nil
	}
	return json.RawMessage(raw), true, nil
}

func (s *PostgresStore) commitOperationStep(ctx context.Context, args OperationArgs, key string, value json.RawMessage) (json.RawMessage, error) {
	var stored []byte
	err := s.pool.QueryRow(
		ctx,
		`UPDATE job_rows
		 SET step_results = jsonb_set(step_results, ARRAY[$3]::text[], $4::jsonb, true)
		 WHERE job_id = $1 AND row_index = $2 AND NOT (step_results ? ($3::text))
		 RETURNING step_results -> ($3::text)`,
		args.JobID,
		args.RowIndex,
		key,
		string(value),
	).Scan(&stored)
	if err == nil {
		return json.RawMessage(stored), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	winner, found, err := s.operationStep(ctx, args, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("operation step was not committed")
	}
	return winner, nil
}

func (s *PostgresStore) retryOperation(ctx context.Context, args OperationArgs, result APIResult, attempt int) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Row %d retrying after attempt %d: %s", args.RowIndex+1, attempt, result.Error)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(
		ctx,
		`UPDATE job_rows SET status = 'queued', result_json = $3, finished_at = NULL
		 WHERE job_id = $1 AND row_index = $2`,
		args.JobID,
		args.RowIndex,
		resultJSON,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET latest_event = $2 WHERE id = $1`, args.JobID, message); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, $2, NOW(), $3)`,
		args.JobID,
		args.RowIndex+1,
		message,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) completeOperation(ctx context.Context, args OperationArgs, result APIResult) error {
	status := "completed"
	if result.Error != "" {
		status = "failed"
	} else if result.Skipped {
		status = "skipped"
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("Row %d %s", args.RowIndex+1, status)
	failedDelta := 0
	if status == "failed" {
		failedDelta = 1
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(
		ctx,
		`UPDATE job_rows SET status = $3, result_json = $4, finished_at = NOW()
		 WHERE job_id = $1 AND row_index = $2
		 AND status NOT IN ('completed', 'failed', 'skipped')`,
		args.JobID,
		args.RowIndex,
		status,
		resultJSON,
	)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	var completed int
	var total int
	var jobStatus string
	err = tx.QueryRow(
		ctx,
		`UPDATE jobs SET
		 completed = completed + 1,
		 failed = failed + $2,
		 status = CASE
		   WHEN completed + 1 >= total AND failed + $2 > 0 THEN 'completed with errors'
		   WHEN completed + 1 >= total THEN 'completed'
		   ELSE 'running'
		 END,
		 finished_at = CASE WHEN completed + 1 >= total THEN NOW() ELSE NULL END,
		 latest_event = $3
		 WHERE id = $1
		 RETURNING completed, total, status`,
		args.JobID,
		failedDelta,
		message,
	).Scan(&completed, &total, &jobStatus)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, $2, NOW(), $3)`,
		args.JobID,
		args.RowIndex+1,
		message,
	); err != nil {
		return err
	}
	if completed >= total {
		finalMessage := "Agent run finished"
		if jobStatus == "completed with errors" {
			finalMessage = "Agent run finished with errors"
		}
		if _, err := tx.Exec(
			ctx,
			`UPDATE jobs SET latest_event = $2 WHERE id = $1`,
			args.JobID,
			finalMessage,
		); err != nil {
			return err
		}
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO job_events (job_id, row_number, at, message) VALUES ($1, 0, NOW(), $2)`,
			args.JobID,
			finalMessage,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func rowTerminal(status string) bool {
	return status == "completed" || status == "failed" || status == "skipped"
}
