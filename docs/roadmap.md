# Roadmap

## Current deployment

Freegent currently runs as one Docker Compose stack:

- PostgreSQL
- Go API
- Go workers
- OpenExtract

Increase capacity with worker replicas or per-worker concurrency.

Workers drain active rows for 30 seconds during shutdown. Successful model and tool calls survive retries in PostgreSQL.

## Planned work

### Elastic workers

- evaluate K3s for replicated workers
- compare KEDA with a small queue-depth scaler
- add node autoscaling only after container scaling is proven
- add spending limits, throttling, and queue metrics first
- keep Docker Compose for development and smaller deployments

Keep row step results in JSONB while the bounded agent loop makes writes small. Move them to a separate table only if measured PostgreSQL write amplification becomes a bottleneck.

### OpenExtract

- preserve complete extracted content instead of silently truncating it
- add structured sections and query-aware passage selection
- measure evidence recall and answer quality

Managed extraction fallbacks belong to Freegent's `fetch_page` tool. OpenExtract stays a local extraction service.
