package api

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

type OperationWorker struct {
	river.WorkerDefaults[OperationArgs]
	store *PostgresStore
}

func (w *OperationWorker) NextRetry(job *river.Job[OperationArgs]) time.Time {
	delays := []time.Duration{
		10 * time.Second,
		time.Minute,
		5 * time.Minute,
		30 * time.Minute,
	}
	index := job.Attempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(delays) {
		index = len(delays) - 1
	}
	return time.Now().Add(delays[index])
}

func (w *OperationWorker) Work(ctx context.Context, job *river.Job[OperationArgs]) error {
	request, input, done, err := w.store.beginOperation(ctx, job.Args, job.Attempt)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	event := func(event AgentEvent) {
		if err := w.store.appendOperationEvent(ctx, job.Args, event.Message); err != nil {
			fmt.Fprintf(os.Stderr, "operation event persistence failed job=%s row=%d error=%v\n", job.Args.JobID, job.Args.RowIndex+1, err)
		}
	}
	result := runOneWithEvents(ctx, request, input, event, newOperationCache(w.store, job.Args, event))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if result.Error != "" && !permanentOperationError(result.Error) && job.Attempt < job.MaxAttempts {
		if err := w.store.retryOperation(ctx, job.Args, result, job.Attempt); err != nil {
			return err
		}
		return errors.New(result.Error)
	}
	return w.store.completeOperation(ctx, job.Args, result)
}

func permanentOperationError(message string) bool {
	value := strings.ToLower(message)
	patterns := []string{
		"openrouter_api_key is not set",
		"api key is not set",
		"url must use http or https",
		"schema validation",
		"invalid output schema",
		"openextract could not extract the url",
		"unsupported protocol",
		"401 unauthorized",
		"404 not found",
		"410 gone",
	}
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}

func RunWorker(args []string) {
	flags := flag.NewFlagSet("freegent worker", flag.ExitOnError)
	concurrency := flags.Int("concurrency", workerConcurrency(), "maximum concurrent research operations")
	timeout := flags.Duration("timeout", 15*time.Minute, "maximum duration for one research operation")
	if err := flags.Parse(args); err != nil {
		panic(err)
	}
	if *concurrency < 1 {
		fmt.Fprintln(os.Stderr, "worker concurrency must be positive")
		return
	}
	databaseURL := os.Getenv("FREEGENT_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "FREEGENT_DATABASE_URL is required for River workers")
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := OpenPostgresStore(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "freegent worker database failed: %v\n", err)
		return
	}
	defer store.Close()
	workers := river.NewWorkers()
	river.AddWorker(workers, &OperationWorker{store: store})
	client, err := river.NewClient(riverpgxv5.New(store.pool), &river.Config{
		JobTimeout:           *timeout,
		RescueStuckJobsAfter: *timeout + time.Minute,
		SoftStopTimeout:      30 * time.Second,
		Queues: map[string]river.QueueConfig{
			"research": {MaxWorkers: *concurrency},
		},
		Workers: workers,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "freegent worker queue failed: %v\n", err)
		return
	}
	if err := client.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "freegent worker start failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "freegent worker started concurrency=%d timeout=%s\n", *concurrency, timeout.String())
	<-client.Stopped()
}

func workerConcurrency() int {
	raw := os.Getenv("FREEGENT_WORKER_CONCURRENCY")
	if raw == "" {
		return 10
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 10
	}
	return value
}
