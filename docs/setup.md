# Setup

## Install

You need:

- macOS or Linux on arm64 or amd64
- Git
- curl
- Docker with Docker Compose
- an OpenRouter API key
- a Serper, Exa, or Tavily search key

Run:

```bash
curl -fsSL https://raw.githubusercontent.com/simonbalfe/freegent/main/install.sh | bash
```

By default, the installer clones Freegent into `~/freegent`, starts the Docker Compose stack, builds a native CLI, and installs the Freegent skill.

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

- `OPENROUTER_API_KEY`
- one of `SERPER_API_KEY`, `EXA_API_KEY`, or `TAVILY_API_KEY`

Optional:

- `APIFY_API_TOKEN` enables enrichment tools
- Evomi settings enable browser proxying
- CapSolver or 2Captcha keys enable challenge solving
- concurrency, ports, database pool size, and operation timeout have defaults in `.env.example`

Never commit `.env`.

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

Verify OpenExtract changes:

```bash
cd openextract
bun install
bun run typecheck
bun test
```

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
cd ~/freegent
git pull --ff-only
docker compose pull
docker compose up -d --force-recreate
```

PostgreSQL data and job results use a Docker volume and survive container restarts.
