---
name: freegent
description: Use the Freegent CLI for source-backed research, URL extraction, and CSV enrichment. Trigger when an agent needs to research one JSON row or enrich many CSV rows.
---

# Freegent CLI

Freegent is a research API with a thin CLI. Submit research to it instead of implementing search, extraction, retries, queueing, or model logic in the calling agent.

## Check Freegent

```bash
curl -fsS http://localhost:8080/health
freegent --help
```

When the installed command is unavailable, run the CLI from this repository:

```bash
go run ./cmd/freegent --help
```

Use `--api-url` when the API is not at `http://localhost:8080`. The dashboard is at `http://localhost:8080/dashboard`.

## Research one row

```bash
freegent \
  --row '{"company":"Figma","domain":"figma.com"}' \
  --instructions "Use current first-party evidence. Do not guess." \
  --prompt "Research {{company}} at {{domain}} for GTM outbound."
```

The answer is written as JSON.

## Enrich a CSV

Use CSV headers as prompt fields:

```bash
freegent \
  --csv companies.csv \
  --instructions "Use current first-party evidence. Do not guess." \
  --prompt "Research {{company}} at {{domain}}." \
  > results.csv
```

The enriched CSV is written to stdout. Each row is an independent research operation.

## Structured answers

The default schema is `{"answer":"string"}`. Supply `--schema` only when every row needs the same structured fields:

```bash
freegent \
  --csv companies.csv \
  --prompt "Research {{company}} and classify it." \
  --schema '{"summary":"string","category":"string","source":"string"}' \
  > results.csv
```

## Large batches

Use `--detach` when the caller only needs the job ID and dashboard URL immediately:

```bash
freegent --csv companies.csv --prompt "Research {{company}}." --detach
```

Rows must contain truthful input values. Never fabricate URLs. Use only URLs supplied in a row, returned by search, or discovered during extraction.
