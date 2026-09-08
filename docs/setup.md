# Setup

## Install

You need:

- macOS or Linux on arm64 or amd64
- curl
- Docker with Docker Compose
- an OpenRouter API key or a ChatGPT subscription with Codex access
- a Serper, Exa, or Tavily search key

Run:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/install.sh | bash
```

By default, the installer stores only the runtime configuration in `~/freegent`, pulls the Docker images, extracts the native CLI, and installs the Freegent skill. It does not clone the repository.

Check the result:

```bash
freegent --help
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/healthz
```

The dashboard is at [http://localhost:8080/dashboard](http://localhost:8080/dashboard).

## Configuration

Copy `.env.example` to `.env` for manual setup.

Required:

- either `OPENROUTER_API_KEY`, or Codex authentication with `FREEGENT_MODEL_PROVIDER=codex`
- one of `SERPER_API_KEY`, `EXA_API_KEY`, or `TAVILY_API_KEY`

Optional:

- `OPENROUTER_MODEL` selects the model and defaults to `deepseek/deepseek-v4-flash`
- `CODEX_MODEL` selects the direct Codex model and defaults to `gpt-5.6-sol`
- `APIFY_API_TOKEN` enables Apify-backed LinkedIn profiles, posts, reactions, employee search, and company firmographics, plus Crunchbase company enrichment
- a standard proxy URL optionally enables browser proxying
- CapSolver or 2Captcha keys enable challenge solving
- concurrency, ports, database pool size, and operation timeout have defaults in `.env.example`

Never commit `.env`.

### Codex mode

Codex mode uses a ChatGPT subscription directly. It does not need an OpenRouter or OpenAI API key.

For a new install, run the installer, enter `codex` when asked for the model provider, and complete the browser login.

To switch an existing install:

1. Set `FREEGENT_MODEL_PROVIDER=codex` in `~/freegent/.env`.
2. Enable device code authorization under ChatGPT **Settings → Security and login**.
3. Run:

```bash
cd ~/freegent
docker compose run --rm --no-deps worker auth
docker compose up -d --force-recreate worker
```

Open the displayed URL and enter the device code. Freegent stores the credentials in the private `freegent_codex_auth` Docker volume and refreshes them automatically. `CODEX_MODEL` optionally changes the model from its `gpt-5.6-sol` default.

The direct ChatGPT Codex endpoint is not the public OpenAI API contract and may change.

For a worker running directly on the host, use `freegent auth`. The credential path defaults to the operating system's user configuration directory and can be overridden with `FREEGENT_CODEX_AUTH_FILE` or `freegent auth -file path`.

## Local development

Start the full stack:

```bash
docker compose up -d --build
```

Run Go processes directly:

```bash
go run ./cmd/freegent --help
FREEGENT_DATABASE_URL=... go run ./cmd/freegent api -port 8080
FREEGENT_DATABASE_URL=... go run ./cmd/freegent worker -concurrency 10 -timeout 15m
```

Verify Go changes:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Build or develop the dashboard after changing files under `internal/api/dashboard`:

```bash
cd internal/api/dashboard
npm ci
npm run build
npm run dev
```

The Vite development server proxies job API calls to `http://localhost:8080`. Commit the production build under `dist`; Docker embeds it in the Go binary.

OpenExtract source and tests live in its [standalone repository](https://github.com/simonbalfe/openextract). Freegent pulls its published image during stack startup.

## Operations

Check services:

```bash
cd ~/freegent
docker compose ps
```

View logs:

```bash
docker compose logs -f api worker openextract
```

Update:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/install.sh | bash
```

Uninstall Freegent while keeping provider keys in `~/freegent/.env`:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/uninstall.sh | bash
```

PostgreSQL data and job results use a Docker volume and survive container restarts.
