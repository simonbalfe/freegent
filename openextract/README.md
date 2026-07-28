# OpenExtract Service

OpenExtract is a standalone TypeScript HTTP API for turning a public URL into clean research text.

It owns direct retrieval, HTML cleanup, Markdown conversion, structured data, PDF parsing, same-site link discovery, Patchright browser escalation, optional proxy and captcha solver escalation, and Tavily fallback.

## API

Health:

```bash
curl http://localhost:8081/healthz
```

Extract:

```bash
curl -sS http://localhost:8081/extract \
  -H 'content-type: application/json' \
  -d '{"url":"https://www.iana.org/help/example-domains"}'
```

The response contains `content`, `contentType`, `provider`, `outcome`, discovered `links`, and every extraction `attempt`.

OpenExtract applies bounded in-memory backpressure. `OPENEXTRACT_MAX_CONCURRENCY` limits all active extractions, `OPENEXTRACT_BROWSER_CONCURRENCY` separately limits Chromium contexts, and `OPENEXTRACT_MAX_WAITING` limits requests waiting for an extraction slot. Saturated requests receive HTTP `429` with `Retry-After: 1`; the durable caller should retry them.

Defaults:

```text
OPENEXTRACT_MAX_CONCURRENCY=20
OPENEXTRACT_BROWSER_CONCURRENCY=4
OPENEXTRACT_MAX_WAITING=100
```

`GET /healthz` reports the configured limits and current active/waiting counts.

The service currently has no authentication or private-network URL filtering. Deploy it on a private network or behind an authenticated gateway rather than exposing it directly to the public internet.

## Run directly

```bash
bun install
bun run typecheck
bun run start
```

## Run with Docker

```bash
docker build -t openextract .
docker run --rm -p 8081:8081 \
  -e TAVILY_API_KEY \
  -e EVOMI_USERNAME \
  -e EVOMI_PASSWORD \
  -e EVOMI_GATEWAY \
  -e CAPSOLVER_API_KEY \
  -e TWOCAPTCHA_API_KEY \
  -e OPENEXTRACT_MAX_CONCURRENCY=20 \
  -e OPENEXTRACT_BROWSER_CONCURRENCY=4 \
  -e OPENEXTRACT_MAX_WAITING=100 \
  openextract
```

All credentials are optional. Without them, unavailable ladder rungs are reported as skipped.
