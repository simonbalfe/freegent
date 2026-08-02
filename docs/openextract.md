# OpenExtract

Freegent workers call OpenExtract through `OPENEXTRACT_URL`. OpenExtract's code is not stored in the Freegent repository. It lives in the separate, public [simonbalfe/openextract repository](https://github.com/simonbalfe/openextract), where its extraction behavior, tests, Dockerfile, and image workflow are maintained.

Freegent owns only the client contract and Compose integration. The OpenExtract repository is public and can be inspected or cloned independently.

The Compose stack pulls `ghcr.io/simonbalfe/openextract:latest` by default. Set `OPENEXTRACT_IMAGE` to pin or replace it. The worker reaches the service at `http://openextract:8081`.

OpenExtract tries direct HTTP first. It uses local Patchright only when the response requires rendering, followed by configured proxy and solver attempts when needed. It contains no Exa, Tavily, or other managed extraction provider.

Freegent owns the complete `fetch_page` chain. The same tool call tries OpenExtract first, Exa second, and Tavily last. The model does not select providers or issue another tool call for fallback extraction.

Set `OPENEXTRACT_PROXY_URL` to a standard HTTP, HTTPS, SOCKS4, or SOCKS5 proxy URL. `OPENEXTRACT_PROXY_COUNTRY` optionally aligns the browser identity with a fixed proxy country.

`POST /extract` must remain compatible with the Go client in `internal/openextract`. It returns cleaned content, content type, provider, outcome, links, and attempted extraction rungs.

OpenExtract has no authentication or private-network URL filtering. Keep the service on a private network or put it behind an authenticated gateway.

See the public standalone repository for runtime, API, environment, development, and extraction-ladder documentation.

Related research:

- [OpenExtract context selection](research/openextract-context-selection.md)
- [ZenRows managed extraction comparison](research/zenrows.md)
