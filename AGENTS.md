# Freegent

## Architecture

- `cmd/freegent` is the only Go executable entry point.
- The Go CLI always submits work to the Go API. Keep `--demo` as the only offline exception.
- `internal/app` routes the executable to the CLI, HTTP API, or River worker.
- `internal/cli` owns argument parsing, CSV input, the remote API client, result rendering, and the offline demo.
- `internal/api` owns the HTTP API, PostgreSQL job history and state, River workers, run logs, and HTMX dashboard.
- `internal/agent` owns the agent loop, shared model and tool contracts, output schemas, prompts, and URL provenance.
- `internal/openrouter` is the OpenRouter model adapter.
- `internal/tools` contains the search, OpenExtract fetch, and Apify enrichment tools.
- `internal/toolset` registers the tools enabled by the current environment.
- `internal/openextract` is only a typed HTTP client for the standalone OpenExtract service.
- `internal/api/postgres_store.go` owns the PostgreSQL schema, durable batch and row state, and transactional River enqueue.
- `internal/api/river_worker.go` owns queued operation execution, retry policy, and worker lifecycle.
- `internal/api/dashboard.templ` and its generated Go file render the server-side dashboard.
- `services/openextract` is a self-contained TypeScript API. It owns direct retrieval, HTML cleanup, Markdown conversion, structured data, link discovery, PDF parsing, Patchright browsers, proxies, solvers, and Tavily extraction fallback.
- OpenExtract bounds all active extraction requests and browser contexts separately. Saturation returns HTTP `429`; River remains the only durable queue and retry owner.
- `compose.yaml` runs PostgreSQL, API, worker, and OpenExtract services.
- The current deployment target is one Docker Compose stack. Do not add K3s, KEDA, a custom replica scaler, or Hetzner node autoscaling until the roadmap item is explicitly started.

Keep the Go API independent from extraction implementation details. OpenExtract must remain deployable on a separate server, with the Go API configured through `OPENEXTRACT_URL`.

PostgreSQL is the only durable source of truth for jobs, rows, events, results, and River queue state. Do not add a SQLite fallback.
The CLI and dashboard submit through `POST /jobs`. The compatibility `POST /run` endpoint creates the same durable job and waits for it.
One submitted batch contains one operation per input row. Each operation is one River job whose arguments contain only the batch ID and row index.
The API validates, persists, and enqueues work. It must not execute the agent loop.
Workers claim operations from the shared `research` queue and run the complete agent loop, including search, fetch, extraction, schema validation, and final answer generation.
River provides at-least-once delivery. Application completion is idempotent, so completed rows are not rerun after a worker acknowledgement failure.
Worker concurrency is deployment-wide capacity configured per replica. Do not restore per-batch concurrency controls.

## Development

- Use `go run ./cmd/freegent --demo` for the offline executable check.
- Use `FREEGENT_DATABASE_URL=... go run ./cmd/freegent api -port 8080` to run the API without leaving a binary in the repository.
- Use `FREEGENT_DATABASE_URL=... go run ./cmd/freegent worker -concurrency 10 -timeout 15m` to run a worker.
- Use `go test ./...`, `go test -race ./...`, and `go vet ./...` for Go verification.
- Use `OPENEXTRACT_URL=http://localhost:8081 RUN_LIVE_OPENEXTRACT=1 go test -count=1 ./internal/openextract -run '^TestLiveOpenExtract$' -v` for the live Go-to-TypeScript check.
- Run `bun install` and `bun run typecheck` from `services/openextract` after TypeScript changes.
- Use `docker compose up -d --build` for the local PostgreSQL, API, worker, and OpenExtract stack.
- Use `RUN_LIVE_APIFY=1 go test ./internal/tools/apify -run '^TestLiveLinkedInCompany$' -v` only when a paid Apify smoke test is intended.
- Generated run traces belong in `logs/` and must not be committed.
- Keep code comment-free. Put durable explanations here or in `README.md`.

## Boundaries

- Search remains Serper, Exa, and Tavily only. Do not restore SearXNG.
- OpenExtract may use Tavily only as the final extraction fallback.
- Do not add OpenAPI or hosted API-reference routes.
- Never fabricate URLs. Fetch and enrichment URLs must originate in the row, search results, or links discovered during the run.
- Keep all provider credentials in environment variables.
- Keep OpenExtract focused on URL extraction. It must not contain the agent loop, search queries, row orchestration, output schemas, or model calls.
- Keep the Go CLI thin. It parses input, calls the Go API, and renders results.
