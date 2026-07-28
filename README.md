# Freegent

Freegent researches one row or many rows at a time.

```text
Go CLI → Go API → PostgreSQL/River → Go workers → OpenExtract
```

- The Go CLI sends requests to the Go API.
- The Go API stores batches and rows in PostgreSQL, then inserts one River job per row.
- Separate Go workers claim River jobs and run the complete research operation.
- OpenExtract fetches and cleans web pages and PDFs, including browser fallback.

Each CLI, API, or dashboard submission creates a durable job. Job history, row state, results, sources, errors, and token usage survive API restarts.

One submitted batch is a Freegent job. Each row inside that batch is one operation and one River job. A 100,000-row CSV therefore creates one Freegent batch containing 100,000 independently retryable River jobs.

## Start

Install the full stack on a new machine:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/install.sh | bash
```

The installer clones the repo, asks for provider keys, pulls the separate prebuilt Go API, River worker, and OpenExtract images from GHCR, and starts Docker Compose. It copies the thin `freegent` CLI from the API image, so Go is not required on the server. If GHCR is unavailable, it falls back to building the images locally.

## API keys and credentials

The minimum working setup is:

1. `OPENROUTER_API_KEY`
2. At least one search key: `SERPER_API_KEY`, `EXA_API_KEY`, or `TAVILY_API_KEY`

Serper is tried first, followed by Exa and Tavily. Configure more than one search provider if you want automatic fallback when a provider is unavailable.

| Environment variable | Required? | What Freegent uses it for | Get it from |
|---|---|---|---|
| `OPENROUTER_API_KEY` | Yes | Runs the research model and final answer model. | [OpenRouter API keys](https://openrouter.ai/settings/keys) |
| `SERPER_API_KEY` | One search key required. Recommended primary. | Google search results for the agent's first search attempt. | [Serper](https://serper.dev/) |
| `EXA_API_KEY` | One search key required. Optional fallback. | Web search and result text when Serper is unavailable or returns an error. | [Exa API keys](https://dashboard.exa.ai/api-keys) |
| `TAVILY_API_KEY` | One search key required. Optional fallback. | Final search fallback and final OpenExtract extraction fallback. | [Tavily API platform](https://app.tavily.com/home) |
| `APIFY_API_TOKEN` | Optional | Enables LinkedIn and Crunchbase enrichment tools. Actor runs can consume paid Apify credits. | [Apify API & Integrations](https://console.apify.com/account/integrations) |
| `EVOMI_USERNAME` and `EVOMI_PASSWORD` | Optional pair | Routes browser fallback through an Evomi residential proxy. | [Evomi dashboard](https://dashboard.evomi.com/) |
| `CAPSOLVER_API_KEY` | Optional | Solves supported CAPTCHA and Cloudflare challenges during browser extraction. | [CapSolver dashboard](https://dashboard.capsolver.com/) |
| `TWOCAPTCHA_API_KEY` | Optional | Secondary CAPTCHA solver when CapSolver is not configured or fails. | [2Captcha dashboard](https://2captcha.com/enterpage) |

Copy the example environment file and add the credentials you want to use:

```bash
cp .env.example .env
docker compose up -d --build
```

`EVOMI_GATEWAY` is not a secret. Keep the default unless your Evomi account uses a different proxy endpoint. `FREEGENT_PORT` and `OPENEXTRACT_PORT` only change the localhost ports exposed by Docker Compose.

PostgreSQL is required. Compose configures it automatically. For a standalone deployment, set `FREEGENT_DATABASE_URL` on both the API and every worker.

Never commit `.env`. Provider usage may be billable, especially OpenRouter models, Apify Actors, proxy traffic, and CAPTCHA solving.

Then check the services:

```bash
curl http://localhost:8080/health
curl http://localhost:8081/healthz
```

Open [http://localhost:8080/dashboard](http://localhost:8080/dashboard) to upload a CSV or submit JSON rows, watch each row update, and inspect previous jobs.

Watch logs:

```bash
docker compose logs -f api worker openextract postgres
```

## Run research

The CLI is only a client for `http://localhost:8080` by default. It does not run research, parse CSV batches, or execute workers locally.

```bash
go run ./cmd/freegent \
  --row '{"company":"Mastra","domain":"mastra.ai"}' \
  --prompt 'Research {{company}} at {{domain}}.'
```

For CSV input:

```bash
go run ./cmd/freegent \
  --csv companies.csv \
  --prompt 'Research {{company}} at {{domain}}.' \
  > results.csv
```

The default schema is `{"answer":"string"}`. Use `--schema` only when the answer needs a different structured shape.

The CLI uploads the CSV directly to `POST /jobs`, waits for completion, then streams `GET /jobs/{id}/results.csv` to stdout. For one JSON row, it prints the result object. The API owns validation, PostgreSQL persistence, River enqueueing, defaults, and execution.

Use `--detach` to submit a large batch without keeping the terminal open:

```bash
freegent \
  --csv companies.csv \
  --prompt 'Research {{company}} at {{domain}}.' \
  --detach
```

Detached output includes the batch ID and its dashboard URL. Use detached mode for very large batches so the client does not need to download every result in one response.

Use `--api-url` or `FREEGENT_API_URL` to call a Go API on another server.

## Queue, storage, and scaling

PostgreSQL is the only job store and River is the durable queue. Docker Compose stores the database in the `freegent_postgres` volume.

The current supported deployment is one Docker Compose stack containing PostgreSQL, the API, River workers, and OpenExtract. Increase capacity by scaling worker replicas or changing per-worker concurrency within that stack. K3s, KEDA, and Hetzner node autoscaling are future work documented in [ROADMAP.md](ROADMAP.md), not part of the current deployment.

- API instances are stateless apart from PostgreSQL and may be replicated behind a load balancer.
- Worker capacity is global. `FREEGENT_WORKER_CONCURRENCY=10` gives each worker container 10 concurrent operations.
- Scale horizontally with `docker compose up -d --scale worker=10` or the equivalent deployment-platform replica setting.
- Each River operation has its own retry state. Transient failures retry with backoff; validation and authentication failures finish immediately.
- Application row completion is idempotent. If a worker exits after saving a result but before River acknowledges it, the retry observes the terminal row and does not repeat provider calls.
- The dashboard and `GET /jobs/{id}?limit=200&offset=0` use paged row reads. `GET /jobs/{id}?summary=1` returns counters without row payloads.

- `POST /jobs` creates an asynchronous durable job.
- `GET /jobs` lists recent job history.
- `GET /jobs/{id}` returns job, row, event, and result state.
- `GET /jobs/{id}/results.csv` downloads the original rows with one appended `answer` column.
- `POST /run` remains compatible with synchronous clients but records the run as the same durable job.

`POST /jobs` accepts either the existing JSON request or a multipart CSV upload:

```bash
curl http://localhost:8080/jobs \
  -F 'name=prospects.csv' \
  -F 'instructions=Research each company using current first-party evidence.' \
  -F 'template=Research {{company}} at {{domain}}.' \
  -F 'schema={"product":"string","source":"string"}' \
  -F 'csv=@prospects.csv'
```

The CSV header names become row fields available to the template. The request creates the same durable PostgreSQL batch and one River operation per CSV record.

After the job finishes, download the Clay-style enriched spreadsheet:

```bash
curl -OJ http://localhost:8080/jobs/JOB_ID/results.csv
```

The export retains the original input columns and adds exactly one `answer` column containing the complete structured result as compact JSON. Failed rows store their error in that same column. If the input already contains an `answer` column, the appended column receives a numeric suffix.

For local processes outside Compose:

```bash
export FREEGENT_DATABASE_URL='postgres://freegent:freegent@localhost:5432/freegent?sslmode=disable'
go run ./cmd/freegent api -port 8080
go run ./cmd/freegent worker -concurrency 10 -timeout 15m
```

The API and worker apply River and Freegent database migrations at startup.

## OpenExtract

OpenExtract is a separate, deployable TypeScript service. It owns:

- direct HTTP retrieval
- HTML and Markdown extraction
- PDF text extraction
- Patchright browser fallback
- optional proxy, captcha solver, and Tavily fallback
- bounded extraction and browser concurrency with HTTP `429` backpressure

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
cmd/freegent/              executable entry point
internal/agent/            agent loop, schemas, prompts, and provenance
internal/api/              HTTP API, PostgreSQL state, River workers, and dashboard
internal/app/              CLI/API/worker command routing
internal/cli/              thin remote API client
internal/openrouter/       OpenRouter model adapter
internal/tools/            search, fetch, and Apify tools
internal/toolset/          tool registration
internal/openextract/      Go client for OpenExtract
services/openextract/      TypeScript extraction service
compose.yaml               local deployment
```

The current Compose ports bind to localhost. Authentication, tenant controls, and SSRF hardening are future work.
