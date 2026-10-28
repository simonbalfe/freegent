# PageLode

Freegent is an agent that calls tools. Page loading, Google search, email finding, and site mapping live in [PageLode](https://github.com/simonbalfe/pagelode), a separate service with its own repository, browsers, and release cycle. Freegent owns only the HTTP client in `internal/pagelode` and the tools built on it.

## Connect

Run PageLode anywhere the worker can reach, then set `PAGELODE_URL`:

- PageLode on the same machine as Docker: `PAGELODE_URL=http://host.docker.internal:8083`
- PageLode on another host: its private address, for example a Tailnet IP

Leave `PAGELODE_URL` empty to run without it. `fetch_page` then uses only Exa and Tavily, and the PageLode tools are not offered to the model.

PageLode has no authentication. Keep it on a private network.

## Tools

| Tool | PageLode endpoint | Use |
|---|---|---|
| `fetch_page` | `POST /extract` | Clean Markdown for one URL; Exa, then Tavily, if PageLode fails |
| `google_search` | `POST /google` with `aiOverview` | Google results and the AI Overview with its sources |
| `find_emails` | `POST /emails` | Published email addresses on a site, each with the page it came from |
| `map_site` | `POST /map` | A site's page URLs and page templates before choosing a page to fetch |

`fetch_page`, `find_emails`, and `map_site` accept only URLs supplied in a row or seen in earlier tool results.

The request and response fields are defined in PageLode's [HTTP API documentation](https://github.com/simonbalfe/pagelode/blob/main/docs/api.md).
