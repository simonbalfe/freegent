# Roadmap

## Current deployment

Freegent currently runs as one Docker Compose stack:

- PostgreSQL
- Go API
- Go workers

Increase capacity with worker replicas or per-worker concurrency.

Workers drain active rows for 30 seconds during shutdown. Successful model and tool calls survive retries in PostgreSQL.

## Planned work

### Faster and cheaper research

Optimize against a fixed benchmark before increasing worker capacity. Track median and p95 row duration, model input and output tokens, provider calls, extracted characters, cache hit rate, answer validity, and evidence quality. An optimization ships only when it lowers time or cost without materially reducing answer quality.

#### Shared provider cache

- cache successful page extractions across jobs by normalized URL, extractor version, and freshness policy
- use HTTP validators such as ETag and Last-Modified when a source provides them
- cache search results for a shorter provider-specific TTL by normalized query and options
- coalesce identical in-flight requests so concurrent rows fetch the same URL only once
- record cache age and provenance with every reused result
- never cache authentication responses, errors, or private page content by default
- add explicit refresh and cache-bypass controls for time-sensitive research

Model answers remain isolated to one row operation by default. Cross-job model-result reuse requires an exact prompt, schema, model, tool, and evidence identity plus an explicit freshness policy.

#### Smaller model context

- strip navigation, repeated boilerplate, and duplicate passages before content enters the transcript
- select query-relevant passages instead of sending complete extracted pages
- enforce separate character and token budgets for each tool result
- compact older tool evidence while retaining URLs, quoted facts, and provenance
- avoid sending the same evidence to the model more than once in a row operation
- evaluate provider-native prompt caching and stable cache keys where supported

#### Adaptive agent loop

- stop as soon as every required output field has enough supporting evidence
- choose row-specific tool and step limits from task complexity instead of using one fixed ceiling
- add an official-URL fast path before broad web search
- route simple extraction and classification to a faster model while reserving stronger reasoning models for ambiguous rows
- skip the separate finalizer when the research response already passes schema validation
- allow independent tool calls to run concurrently within a bounded row budget

#### Budgets and controls

- support per-row and per-job limits for tokens, provider calls, elapsed time, and recorded spend
- estimate the cost and likely provider usage before a large job starts
- expose cache hits, avoided calls, token savings, and time savings in job statistics
- pause new work when a provider approaches its rate or spending limit
- make retries consume the remaining budget instead of resetting it

#### Rollout order

1. Add step-level latency, token, payload-size, and cache telemetry.
2. Add normalized URL extraction caching and in-flight request coalescing.
3. Add query-aware passage selection and tool-result token budgets.
4. Add early stopping and task-specific step limits.
5. Add model routing and provider-native prompt caching after quality benchmarks exist.

### Elastic workers

- evaluate K3s for replicated workers
- compare KEDA with a small queue-depth scaler
- add node autoscaling only after container scaling is proven
- add spending limits, throttling, and queue metrics first
- keep Docker Compose for development and smaller deployments

Keep row step results in JSONB while the bounded agent loop makes writes small. Move them to a separate table only if measured PostgreSQL write amplification becomes a bottleneck.

### PageLode tools

- measure evidence recall and answer quality per tool
- expose more PageLode endpoints, such as `map endpoints`, when a research use needs them

Managed extraction fallbacks belong to Freegent's `fetch_page` tool. Scraping and rendering belong to PageLode.
