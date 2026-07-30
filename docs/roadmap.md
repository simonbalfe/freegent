# Roadmap

## Current deployment

Freegent currently runs as one Docker Compose stack:

- PostgreSQL
- Go API
- Go workers
- OpenExtract

Increase capacity with worker replicas or per-worker concurrency.

## Planned work

### Elastic workers

- evaluate K3s for replicated workers
- compare KEDA with a small queue-depth scaler
- add node autoscaling only after container scaling is proven
- add graceful draining, spending limits, throttling, and queue metrics first
- keep Docker Compose for development and smaller deployments

### OpenExtract

- preserve complete extracted content instead of silently truncating it
- add structured sections and query-aware passage selection
- add provenance and freshness details for hosted fallbacks
- measure evidence recall and answer quality
- consider moving OpenExtract into its own repository

Tavily extraction is already the final fallback after direct and browser attempts.
