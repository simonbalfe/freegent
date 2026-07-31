# OpenExtract

Freegent workers call the standalone [OpenExtract service](https://github.com/simonbalfe/openextract) through `OPENEXTRACT_URL`. Freegent owns the client contract and Compose integration. OpenExtract owns extraction behavior, tests, and its published image.

The Compose stack pulls `ghcr.io/simonbalfe/openextract:latest` by default. Set `OPENEXTRACT_IMAGE` to pin or replace it. The worker reaches the service at `http://openextract:8081`.

OpenExtract tries direct HTTP first. It uses local Patchright only when the response requires rendering, followed by configured proxy, solver, and hosted fallbacks when needed.

Set `OPENEXTRACT_PROXY_URL` to a standard HTTP, HTTPS, SOCKS4, or SOCKS5 proxy URL. `OPENEXTRACT_PROXY_COUNTRY` optionally aligns the browser identity with a fixed proxy country.

`POST /extract` must remain compatible with the Go client in `internal/openextract`. It returns cleaned content, content type, provider, outcome, links, and attempted extraction rungs.

OpenExtract has no authentication or private-network URL filtering. Keep the service on a private network or put it behind an authenticated gateway.

See the standalone repository for runtime, API, environment, development, and extraction-ladder documentation.

Related research:

- [OpenExtract context selection](research/openextract-context-selection.md)
- [ZenRows managed extraction comparison](research/zenrows.md)
