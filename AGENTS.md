# Freegent

Freegent is a self-hosted research agent that calls tools. The Go API stores work in PostgreSQL, River queues one operation per row, workers run the agent loop, and the separate PageLode service backs page loading, Google search, email finding, and site mapping.

## Read first

- `docs/index.md` routes all project documentation.
- `docs/architecture.md` explains the system and job flow.
- `docs/setup.md` covers installation, local development, and operations.
- `docs/pagelode.md` explains the PageLode service integration and the tools built on it.
- `docs/roadmap.md` records future work.

## Standing rules

- `cmd/freegent` is the only Go executable entry point.
- Keep the user-facing CLI as a thin API client.
- The API validates, persists, and enqueues work. It must not run the agent loop.
- Workers run complete research operations from the shared River `research` queue.
- PostgreSQL is the only durable source of truth. Do not add a SQLite fallback.
- Keep PageLode independent and reachable through `PAGELODE_URL`. Freegent holds only its HTTP client and tools, never scraping or rendering code.
- Load provider environment variables once through `internal/config` and pass typed configuration into runtime dependencies.
- Keep `web_search` limited to Serper, Exa, and Tavily. Google search through PageLode is the separate `google_search` tool.
- Keep the `fetch_page` fallback order PageLode, Exa, then Tavily. Managed providers must not live inside PageLode.
- Never fabricate URLs. Fetch, map, or crawl only URLs supplied in a row, returned by search, or discovered by an earlier tool.
- Keep the current deployment as one Docker Compose stack until a roadmap item explicitly changes it.
- Keep code comment-free. Put durable explanations in `docs/`.

## Documentation style

- Write for humans and agents.
- Use short sentences, plain language, and small sections.
- Keep one canonical home for each fact. Link to it instead of repeating it.
- Use small Mermaid diagrams only when they make a flow easier to understand.
- Keep `README.md` as the user quickstart, `AGENTS.md` as rules and routing, and `docs/` as the explanation layer.
- Update the matching document when architecture, setup, routes, or runtime behavior changes.

## Verification

- Go: `go test ./...`, `go test -race ./...`, and `go vet ./...`
- Dashboard: `cd internal/api/dashboard && npm ci && npm run build`
- CLI: `go run ./cmd/freegent --help`
- Compose: `docker compose config --quiet`
- Full stack: `docker compose up -d --build`
