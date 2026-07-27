# OpenClaygent Go

## Architecture

- `cmd/openclaygent-go` is the only Go executable entry point.
- The Go CLI always submits work to the Go API. Keep `--demo` as the only offline exception.
- `internal/claygent` owns the CLI, HTTP API, synchronous row worker pool, agent loop, output schemas, OpenRouter adapter, search ladder, URL provenance, run logs, and Apify enrichment tools.
- `internal/openextract` is only a typed HTTP client for the standalone OpenExtract service.
- `internal/claygent/jobs.go` owns the in-memory dashboard job status used by the HTMX dashboard.
- `internal/claygent/dashboard.templ` and its generated Go file render the server-side dashboard.
- `services/openextract` is a self-contained TypeScript API. It owns direct retrieval, HTML cleanup, Markdown conversion, structured data, link discovery, PDF parsing, Patchright browsers, proxies, solvers, and Tavily extraction fallback.
- `compose.yaml` runs two containers: `api` and `openextract`.

Keep the Go API independent from extraction implementation details. OpenExtract must remain deployable on a separate server, with the Go API configured through `OPENEXTRACT_URL`.

Do not add queueing, Postgres, or River until explicitly requested. The current API processes a submitted batch synchronously with bounded concurrency.
The dashboard's `POST /jobs` endpoint is an in-memory asynchronous view over the same worker loop; job state is lost when the API restarts.

## Development

- Use `go run ./cmd/openclaygent-go --demo` for the offline executable check.
- Use `go run ./cmd/openclaygent-go api -port 8080` to run the API without leaving a binary in the repository.
- Use `go test ./...`, `go test -race ./...`, and `go vet ./...` for Go verification.
- Use `OPENEXTRACT_URL=http://localhost:8081 RUN_LIVE_OPENEXTRACT=1 go test -count=1 ./internal/openextract -run '^TestLiveOpenExtract$' -v` for the live Go-to-TypeScript check.
- Run `bun install` and `bun run typecheck` from `services/openextract` after TypeScript changes.
- Use `docker compose up -d --build` for the local two-container stack.
- Use `RUN_LIVE_APIFY=1 go test ./internal/claygent -run '^TestLiveLinkedInCompany$' -v` only when a paid Apify smoke test is intended.
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
