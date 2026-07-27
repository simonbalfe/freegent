# Freegent

Freegent researches one row or many rows at a time.

```text
Go CLI → Go API → OpenExtract (TypeScript)
```

- The Go CLI sends requests to the Go API.
- The Go API runs the agent, search tools, schemas, and row processing.
- OpenExtract fetches and cleans web pages and PDFs, including browser fallback.

There is no queue or database yet. Requests run synchronously with limited concurrency.

## Start

```bash
cp .env.example .env
docker compose up -d --build
```

Add your provider keys to `.env`, then check the services:

Serper is the default search provider. Exa and Tavily are automatic fallbacks
when Serper is not configured or returns an error.

```bash
curl http://localhost:8080/health
curl http://localhost:8081/healthz
```

Open [http://localhost:8080/dashboard](http://localhost:8080/dashboard) to submit a JSON row batch and watch each row update as the agent runs.

Watch logs:

```bash
docker compose logs -f api openextract
```

## Run research

The CLI talks to `http://localhost:8080` by default:

```bash
go run ./cmd/openclaygent-go \
  --row 'company=Mastra,domain=mastra.ai' \
  --instructions 'Use current first-party evidence.' \
  --template 'Research {{company}} at {{domain}}.' \
  --schema '{"summary":"string","source":"string"}' \
  --pretty
```

For CSV input:

```bash
go run ./cmd/openclaygent-go \
  --rows companies.csv \
  --instructions 'Find the primary product.' \
  --template 'Research {{company}} at {{domain}}.' \
  --schema '{"product":"string","source":"string"}' \
  --concurrency 5 \
  --json
```

Use `--api-url` or `OPENCLAYGENT_API_URL` to call a Go API on another server.

## OpenExtract

OpenExtract is a separate, deployable TypeScript service. It owns:

- direct HTTP retrieval
- HTML and Markdown extraction
- PDF text extraction
- Patchright browser fallback
- optional proxy, captcha solver, and Tavily fallback

The Go API calls it using `OPENEXTRACT_URL`. In Docker Compose the internal address is `http://openextract:8081`.

To run it separately:

```bash
cd services/openextract
docker build -t openextract .
docker run --rm -p 8081:8081 openextract
```

Test extraction directly:

```bash
curl http://localhost:8081/extract \
  -H 'content-type: application/json' \
  -d '{"url":"https://example.com"}'
```

## Development checks

```bash
go test ./...
go test -race ./...
go vet ./...

cd services/openextract
bun run typecheck
```

## Layout

```text
cmd/openclaygent-go/       CLI entry point
internal/claygent/         Go API and agent
internal/openextract/      Go client for OpenExtract
services/openextract/      TypeScript extraction service
compose.yaml               local deployment
```

The current Compose ports bind to localhost. Authentication, tenant controls, SSRF protection, and durable queueing are future work.
