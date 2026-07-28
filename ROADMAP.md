# Roadmap

## Self-hosted elastic worker execution

This is future infrastructure work. The current supported deployment remains one Docker Compose stack containing PostgreSQL, the API, River workers, and OpenExtract. For now, increase capacity only by changing worker replicas or concurrency inside that stack.

- Keep PostgreSQL and River as the durable job, retry, and result control plane.
- Evaluate K3s as the container scheduler for replicated Go workers.
- Compare KEDA's PostgreSQL scaler with a small Freegent-specific Go scaler that calculates desired replicas from River queue depth.
- Add Hetzner node autoscaling only after container-level scaling is proven, so pending worker Pods can provision and remove VPS capacity.
- Keep OpenExtract direct-fetch and browser capacity independently bounded and place browser workloads on a separate node pool if required.
- Require graceful worker draining, idempotent completion, provider spending limits, per-domain throttling, queue-depth metrics, and dashboard visibility before enabling automatic scaling.
- Preserve the single Compose deployment as the development and small-production option after an autoscaled deployment is introduced.

## Research extraction fallbacks for protected sites

- Investigate how Tavily can extract Cloudflare-protected pages when our local HTTP and browser attempts receive a JavaScript challenge.
- Compare Tavily URL extraction with other hosted extraction providers and authenticated browser options.
- Determine whether the result is live retrieval, cached content, or search-index content, and how fresh it is.
- Record provider limits, pricing, attribution requirements, robots and terms-of-service considerations, and failure modes.
- Add provenance and freshness metadata to extraction results where the provider exposes it.
- Keep the fallback observable in the dashboard so users can see when hosted extraction was used.
