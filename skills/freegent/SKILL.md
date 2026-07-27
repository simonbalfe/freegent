---
name: freegent
description: Use the Freegent CLI to submit one or many web-research rows to the local Freegent API. Use for structured company research, GTM enrichment, source-backed extraction, and batch jobs.
---

# Freegent

Freegent is a local research API with a small CLI surface. Do not implement the search, browser, retry, or extraction logic in the calling agent. Submit the task and rows to Freegent instead.

## Check the service

```bash
curl -fsS http://localhost:8080/health
freegent --help
```

## Submit one row

```bash
freegent \
  --instructions "Research the company using current first-party evidence. Do not guess unsupported facts." \
  --template "Research {{company}} at {{domain}} for GTM outbound." \
  --schema '{"product":"string","targetCustomer":"string","fit":"high|medium|low","source":"string"}' \
  --row 'company=Figma,domain=figma.com' \
  --pretty
```

## Submit many rows

Use a CSV with headers matching the template fields:

```bash
freegent \
  --rows companies.csv \
  --instructions "Research each company using current evidence." \
  --template "Research {{company}} at {{domain}}." \
  --schema '{"product":"string","targetCustomer":"string","source":"string"}' \
  --concurrency 5 \
  --json
```

Use `--out results.json` to save the full response. Use `--api-url` when the API is on another host.

## What Freegent handles

- Serper first, then Exa and Tavily search fallbacks
- Agent tool calls and bounded turns
- Direct extraction, PDFs, and Patchright browser fallback through OpenExtract
- Schema validation and final-answer recovery
- Per-row progress, evidence URLs, steps, and token usage

Rows should contain truthful company/domain pairs. The API rejects guessed deep URLs that were not supplied in the row or returned by search.
