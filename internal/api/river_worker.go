package api

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/simonbalfe/freegent/internal/agent"
	"github.com/simonbalfe/freegent/internal/codex"
	"github.com/simonbalfe/freegent/internal/config"
)

type OperationWorker struct {
	river.WorkerDefaults[OperationArgs]
	store     *PostgresStore
	providers config.Providers
	codexAuth *codex.TokenSource
	client    *http.Client
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
	event := func(event agent.AgentEvent) {
		if err := w.store.appendOperationEvent(ctx, job.Args, event); err != nil {
			fmt.Fprintf(os.Stderr, "operation event persistence failed job=%s row=%d error=%v\n", job.Args.JobID, job.Args.RowIndex+1, err)
		}
	}
	result, runErr := runOneWithEvents(ctx, request, input, event, newOperationCache(w.store, job.Args, event), w.providers, w.codexAuth, w.client)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil && !agent.IsPermanent(runErr) && job.Attempt < job.MaxAttempts {
		if err := w.store.retryOperation(ctx, job.Args, result, job.Attempt); err != nil {
			return err
		}
		return runErr
	}
	return w.store.completeOperation(ctx, job.Args, result)
}

func RunWorker(args []string) {
	flags := flag.NewFlagSet("freegent worker", flag.ExitOnError)
	concurrency := flags.Int("concurrency", 10, "maximum concurrent research operations")
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
	providers := config.LoadProviders()
	httpClient := &http.Client{Timeout: 150 * time.Second}
	var codexAuth *codex.TokenSource
	if providers.ModelProvider == "codex" {
		authFile := providers.CodexAuthFile
		if authFile == "" {
			authFile, err = codex.DefaultAuthPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "freegent Codex auth path failed: %v\n", err)
				return
			}
		}
		codexAuth, err = codex.NewTokenSource(authFile, httpClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "freegent Codex authentication failed: %v; run freegent auth\n", err)
			return
		}
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &OperationWorker{store: store, providers: providers, codexAuth: codexAuth, client: httpClient})
	queue, err := river.NewClient(riverpgxv5.New(store.pool), &river.Config{
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
	if err := queue.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "freegent worker start failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "freegent worker started concurrency=%d timeout=%s\n", *concurrency, timeout.String())
	<-queue.Stopped()
}
