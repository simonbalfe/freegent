# Roadmap

## Research extraction fallbacks for protected sites

- Investigate how Tavily can extract Cloudflare-protected pages when our local HTTP and browser attempts receive a JavaScript challenge.
- Compare Tavily URL extraction with other hosted extraction providers and authenticated browser options.
- Determine whether the result is live retrieval, cached content, or search-index content, and how fresh it is.
- Record provider limits, pricing, attribution requirements, robots and terms-of-service considerations, and failure modes.
- Add provenance and freshness metadata to extraction results where the provider exposes it.
- Keep the fallback observable in the dashboard so users can see when hosted extraction was used.

