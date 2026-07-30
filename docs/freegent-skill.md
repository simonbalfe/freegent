---
name: freegent
description: Use the thin Freegent CLI to submit a CSV or one JSON row to the Freegent API for general source-backed research and batch enrichment.
---

# Freegent skill

Freegent is a research API with a thin remote CLI. Do not implement parsing, search, browser, retry, extraction, queueing, or model logic in the calling agent. Submit the prompt and input to Freegent instead.

## Check the service

```bash
curl -fsS http://localhost:8080/health
freegent --help
```

The live dashboard runs at [http://localhost:8080/dashboard](http://localhost:8080/dashboard). Use it to upload CSVs, submit jobs interactively, and inspect durable job history, row progress, agent steps, evidence sources, answers, errors, and token usage.

## Submit one row

```bash
freegent \
  --row '{"company":"Figma","domain":"figma.com"}' \
  --instructions "Use current first-party evidence. Do not guess." \
  --prompt "Research {{company}} at {{domain}} for GTM outbound."
```

## Submit many rows

Use a CSV with headers matching the template fields:

```bash
freegent \
  --csv companies.csv \
  --instructions "Use current first-party evidence. Do not guess." \
  --prompt "Research {{company}} at {{domain}}." \
  > results.csv
```

`--instructions` applies batch-wide research rules. `--prompt` is rendered separately for every row using its fields.
The default schema is `{"answer":"string"}`. Add `--schema` only when a different structured result is required. Use `--api-url` when the API is on another host.
Use `--detach` for a large batch when the caller only needs the job ID and dashboard URL immediately.

CSV input is uploaded directly to the API, and the completed enriched CSV is streamed back to stdout. Every CLI submission creates a PostgreSQL-backed batch. Each row becomes one independently retryable River operation. For a visual live trace or previous results, open `http://localhost:8080/dashboard` in a browser. Workers resume queued operations independently of API restarts.

## What Freegent handles

- Serper first, then Exa and Tavily search fallbacks
- Agent tool calls and bounded turns
- Direct extraction, PDFs, and Patchright browser fallback through OpenExtract
- Schema validation and final-answer recovery
- Durable per-row state, progress, evidence URLs, steps, results, errors, and token usage
- PostgreSQL/River queueing with per-worker concurrency and horizontal worker scaling

Rows should contain truthful input values. The worker agent refuses fetches whose URL was not supplied in the row, returned by search, or discovered during extraction.
