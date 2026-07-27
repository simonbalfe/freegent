# OpenClaygent Go

OpenClaygent Go is a row-based research agent with three clear parts:

```text
Go CLI
  |
  | POST /run
  v
Go API
  |\
  | \ OpenRouter, Serper, Exa, Tavily, Apify
  |
  | POST /extract
  v
TypeScript OpenExtract service
  |
  + direct fetch, HTML, Markdown, PDF, browser, proxy, solver, Tavily fallback
```

The normal deployment has two containers:

- `api`: the Go HTTP API, agent loop, search tools, schema validation, row concurrency, and run logs.
- `openextract`: a standalone TypeScript URL extraction API containing all browser and document complexity.

The Go CLI is an API client. It does not run agent jobs locally. `--demo` is the only offline path.

There is no durable queue or database yet. A batch request is processed synchronously by the Go API with a bounded in-memory worker pool.

## Start the stack

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps
curl http://localhost:8080/health
curl http://localhost:8081/healthz
```

Put provider credentials in `.env`. Search uses Serper, Exa, and Tavily only. Missing providers are skipped.

Follow logs:

```bash
docker compose logs -f api openextract
```

Stop the stack:

```bash
docker compose down
```

## Run the CLI

The CLI calls `http://localhost:8080` by default:

```bash
go run ./cmd/openclaygent-go \
  --row 'company=Mastra,domain=mastra.ai' \
  --instructions 'Use current first-party evidence. Return null for unsupported fields.' \
  --template 'Research {{company}} at {{domain}}.' \
  --schema '{"company":"string","summary":"string","homepage":"string?","confidence":"low|medium|high"}' \
  --max-steps 6 \
  --pretty
```

Point it at another API deployment with either:

```bash
export OPENCLAYGENT_API_URL="https://claygent.example.com"
```

or:

```bash
go run ./cmd/openclaygent-go --api-url "https://claygent.example.com" ...
```

CSV batches use one API request:

```bash
go run ./cmd/openclaygent-go \
  --rows companies.csv \
  --instructions 'Find the primary product and one supporting source.' \
  --template 'Research {{company}} at {{domain}}.' \
  --schema '{"product":"string","source":"string"}' \
  --concurrency 5 \
  --json
```

Other supported inputs are `--action`, repeated `--input key=value`, `--row`, and `--require`. Output options are `--json`, `--pretty`, and `--out`.

## Run services during development

Start only OpenExtract:

```bash
docker compose up -d --build openextract
```

Run the Go API without leaving a compiled binary:

```bash
OPENEXTRACT_URL=http://localhost:8081 \
  go run ./cmd/openclaygent-go api -port 8080
```

Then run the CLI from another terminal.

## Deploy OpenExtract separately

OpenExtract is self-contained and does not import or call Go code:

```bash
cd services/openextract
docker build -t openextract .
docker run --rm -p 8081:8081 \
  -e TAVILY_API_KEY \
  -e EVOMI_USERNAME \
  -e EVOMI_PASSWORD \
  -e EVOMI_GATEWAY \
  -e CAPSOLVER_API_KEY \
  -e TWOCAPTCHA_API_KEY \
  openextract
```

Configure the Go API with its deployed address:

```bash
export OPENEXTRACT_URL="https://extract.example.com"
go run ./cmd/openclaygent-go api -port 8080
```

OpenExtract exposes:

- `GET /healthz`
- `POST /extract` with `{"url":"https://example.com"}`

Its response contains cleaned content, content type, winning provider, outcome, discovered links, and every attempted extraction rung.

## Extraction flow

OpenExtract owns the full URL pipeline:

```text
Impit direct retrieval
  -> Patchright browser
  -> Patchright with proxy
  -> Patchright with solver
  -> Tavily extraction
```

Unavailable proxy, solver, and Tavily rungs are recorded as skipped. HTML uses Mozilla Readability with a Cheerio pruning fallback, then Turndown with GFM support. It also extracts selected JSON-LD and metadata, discovers same-site links, and parses normal text PDFs.

The Go API sees only a typed OpenExtract client. This keeps agent logic independent from browser and document-processing implementation details.

## Go API

`POST /run` accepts one `input` or a `rows` batch plus:

- `instructions`
- `template`
- `schema`
- `model`
- `maxSteps`
- `maxOutputTokens`
- `concurrency`
- `require`

Each row runs independently. Results stay in input order and include the structured answer, sources, evidence, agent steps, token usage, timing, and any row error. Run traces are written atomically to `logs/<runId>.json` or `OPENCLAY_LOG_DIR`.

The model can use:

- `web_search`, which tries Serper, Exa, then Tavily.
- `fetch_page`, which calls OpenExtract for a URL already present in the row, search evidence, or discovered links.
- Optional Apify LinkedIn and Crunchbase tools when `APIFY_API_TOKEN` is configured.

The URL ledger prevents the model from fetching invented URLs.

## Output validation

Short schemas are accepted:

```json
{
  "name": "string",
  "score": "number?",
  "confidence": "low|medium|high",
  "active": "boolean"
}
```

Full JSON Schema is also accepted, including nested objects, arrays, enums, formats, numeric constraints, required fields, and `additionalProperties`. Remote HTTP `$ref` values are rejected.

Model output is validated after the agent pass and after the evidence-only finalizer pass.

## Project layout

```text
cmd/openclaygent-go/       Go executable entry point
internal/claygent/         CLI, API, agent, tools, schemas, and providers
internal/openextract/      typed Go client for the OpenExtract API
services/openextract/      standalone TypeScript extraction service
compose.yaml               local two-container deployment
```

## Verify

```bash
go test ./...
go test -race ./...
go vet ./...

cd services/openextract
bun run typecheck
```

With the service running:

```bash
OPENEXTRACT_URL=http://localhost:8081 RUN_LIVE_OPENEXTRACT=1 \
  go test -count=1 ./internal/openextract -run '^TestLiveOpenExtract$' -v
```

## Deferred work

- Durable queueing, retries, job status, cancellation, and database-backed row processing.
- Authentication, tenant controls, and SSRF protection for a public deployment. The current Compose ports bind to localhost.
- OCR for scanned or image-only PDFs.
- DataDome-specific challenge handling.

OpenAPI routes are intentionally omitted. SearXNG is intentionally omitted.
