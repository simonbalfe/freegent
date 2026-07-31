# Freegent

Freegent is a self-hosted research agent. The Go API stores work in PostgreSQL, River queues one operation per row, workers run the agent loop, and OpenExtract handles URL extraction.

## Read first

- `docs/index.md` routes all project documentation.
- `docs/architecture.md` explains the system and job flow.
- `docs/setup.md` covers installation, local development, and operations.
- `docs/openextract.md` explains the standalone extraction service integration.
- `docs/roadmap.md` records future work.

## Standing rules

- `cmd/freegent` is the only Go executable entry point.
- Keep the user-facing CLI as a thin API client.
- The API validates, persists, and enqueues work. It must not run the agent loop.
- Workers run complete research operations from the shared River `research` queue.
- PostgreSQL is the only durable source of truth. Do not add a SQLite fallback.
- Keep OpenExtract independent and reachable through `OPENEXTRACT_URL`.
- Keep search limited to Serper, Exa, and Tavily.
- Tavily is OpenExtract's final extraction fallback.
- Never fabricate URLs. Fetch only URLs supplied in a row, returned by search, or discovered during extraction.
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
- CLI: `go run ./cmd/freegent --help`
- OpenExtract integration: `docker compose config --quiet`
- Full stack: `docker compose up -d --build`
