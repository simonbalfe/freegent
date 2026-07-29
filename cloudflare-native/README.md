# Freegent Cloudflare-native prototype

This is a clean-slate Cloudflare implementation of scalable agentic research. It is isolated from the existing Go deployment and deployed only as a test Worker.

## Architecture

```text
HTTP API Worker
    |
    +--> D1: jobs, run status, summaries and dispatch state
    |
    +--> Run Queue: admission and Workflow creation rate
             |
             +--> one Research Workflow per input row
                      |
                      +--> OpenRouter model decisions
                      +--> Serper search
                      +--> Browser Queue
                               |
                               +--> Browser Run Markdown extraction
                               +--> R2 browser artifact
                               +--> Workflow event
                      |
                      +--> R2 full result and evidence artifact
                      +--> D1 terminal status and summary
```

Research Workflows are durable and can wait without holding an active Worker invocation. Browser requests use a separate Queue with concurrency ten, matching the included paid-plan browser concurrency and preventing a large research batch from overrunning Browser Run.

D1 contains only indexed control-plane data and compact result summaries. R2 contains larger evidence and extraction artifacts. Deterministic Workflow IDs and browser object keys make duplicate Queue delivery safe. The run Queue creates Workflows in idempotent batches of 100 with one consumer invocation at a time, matching the per-Workflow creation limit of 100 instances per second.

## API

`POST /jobs` accepts:

```json
{
  "name": "Company research",
  "instructions": "Prefer first-party sources.",
  "template": "Research {{company}} using {{website}}.",
  "schema": {
    "type": "object",
    "properties": {
      "employeeCount": {
        "type": ["integer", "null"]
      }
    },
    "required": ["employeeCount"],
    "additionalProperties": false
  },
  "rows": [
    {
      "company": "Example",
      "website": "https://example.com"
    }
  ]
}
```

Jobs currently accept up to 1,000 rows per request. Every route except `/health` requires `Authorization: Bearer <API_TOKEN>`.

`GET /jobs/:id` returns aggregate status and compact row results.

## Local verification

```bash
bun install
bun run check
```

Quick Actions are remote-only during local development. Pure tests and deployment bundling do not call Cloudflare services.

## Test deployment

The test Worker is available at `https://freegent-native.sbmain17.workers.dev`. It uses test resources named `freegent-native`, `freegent-native-artifacts`, `freegent-runs` and `freegent-browser`, plus separate dead-letter Queues.

The first completed browser smoke test ran one row through the API, Queue, Workflow, Browser Run, R2 and D1 in 13 seconds. Browser Run extracted 17,661 characters and 116 links from the requested page in 2.137 seconds.

To reproduce the deployment in another account:

1. Create the D1 database, R2 bucket, run Queue, browser Queue and both dead-letter Queues.
2. Update the D1 identifier in `wrangler.jsonc`.
3. Apply `migrations/0001_initial.sql`.
4. Set `API_TOKEN`, `OPENROUTER_API_KEY` and `SERPER_API_KEY` as Worker secrets.
5. Deploy with Wrangler.

No production resources are used by this prototype.
