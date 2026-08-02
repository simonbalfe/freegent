# ZenRows research

Research date: 30 July 2026.

This is a dated research note, not current architecture documentation. Prices and product limits must be checked again before implementation.

## Verdict

ZenRows is a credible managed extraction fallback for Freegent, especially for JavaScript-heavy, geo-restricted, and anti-bot-protected pages. It should not replace Freegent, River, the model loop, or the local HTTP-first OpenExtract path.

The recommended initial position is:

```text
River worker
  -> fetch_page
     -> OpenExtract: Impit, Patchright, proxy, solver
     -> ZenRows Adaptive Stealth Mode
     -> Exa fallback
     -> Tavily fallback
```

Benchmark a simpler alternative if maintaining the local proxy and solver rungs becomes expensive:

```text
fetch_page
  -> OpenExtract: Impit, Patchright
  -> ZenRows Adaptive Stealth Mode
  -> Exa fallback
  -> Tavily fallback
```

ZenRows is most valuable when it either improves the success rate of the residual difficult URLs or returns structured output that lets Freegent skip an LLM call. It is not compelling as the default fetcher while OpenExtract already handles ordinary pages on existing infrastructure.

## Product scope

ZenRows provides three services under one shared subscription balance:

| Product | Responsibility | Freegent fit |
|---|---|---|
| Universal Scraper API | Fetch pages, render JavaScript, use residential proxies, return HTML, Markdown, or structured output | Managed `fetch_page` fallback |
| Scraping Browser | Remote browser sessions billed by session time and transferred data | Possible managed browser provider, but broader than the current one-page extraction contract |
| Residential Proxies | Standalone residential proxy traffic | Alternative proxy source, not a replacement for extraction logic |

The Universal Scraper API is the relevant first integration. Its `mode=auto` Adaptive Stealth Mode starts with the cheapest configuration and escalates to JavaScript rendering or premium proxies. Only the successful configuration is billed. Failed internal attempts are not charged.

## Current pricing

Universal Scraper API pricing uses a base request rate and feature multipliers:

| Successful request type | Multiplier |
|---|---:|
| Basic HTTP page | 1x |
| JavaScript rendering | 5x |
| Premium residential proxy | 10x |
| JavaScript and premium proxy | 25x |

The entry Developer plan costs $69.99 per month and includes a shared $69.99 balance. At its published rates, it covers:

| Request type | Included successful requests | Cost per 1,000 |
|---|---:|---:|
| Basic | 250,000 | $0.28 |
| JavaScript | 50,000 | $1.40 |
| Premium proxy | 25,000 | $2.80 |
| JavaScript and premium proxy | 10,000 | $7.00 |

Higher plans lower the unit price:

| Plan | Monthly price | Basic per 1,000 | JavaScript per 1,000 | Premium proxy per 1,000 | Protected per 1,000 |
|---|---:|---:|---:|---:|---:|
| Developer | $69.99 | $0.28 | $1.40 | $2.80 | $7.00 |
| Startup | $129.99 | $0.13 | $0.65 | $1.30 | $3.25 |
| Business | $299.99 | $0.10 | $0.50 | $1.00 | $2.50 |
| Business 500 | $499.99 | $0.08 | $0.40 | $0.80 | $2.08 |

The approximate minimum plan needed for 100,000 successful pages is:

| Workload | Minimum suitable public plan | Monthly cash outlay |
|---|---|---:|
| 100,000 basic pages | Developer | $69.99 |
| 100,000 JavaScript pages | Startup | $129.99 |
| 100,000 premium-proxy pages | Startup | $129.99 |
| 100,000 pages needing JavaScript and premium proxies | Business | $299.99 |

The balance is shared across Universal Scraper API, Scraping Browser, and proxy usage. Unused capacity is still subject to the monthly subscription floor.

## Price position

For 100,000 ordinary page extractions:

| Route | Approximate extraction cost | Qualification |
|---|---:|---|
| Existing OpenExtract | No new platform fee | Existing compute, proxies, and engineering remain |
| Spider basic scrape | About $10 | Usage-based estimate, output quality and protected-site success require benchmarking |
| ZenRows | $69.99 minimum plan | Includes up to 250,000 basic successful requests |
| Firecrawl | $83 annually or $99 monthly | Includes 100,000 page credits |
| Exa Contents | $100 | Pay as you go at $1 per 1,000 pages per content type |

ZenRows is inexpensive for ordinary pages, but its subscription floor makes it less attractive for irregular small batches. Its pricing becomes more meaningful on difficult pages because the comparison must include success rate, retries, browser operations, proxy traffic, and engineering time.

## Inference-cost impact

ZenRows does not answer Freegent's research question by itself. Using its Markdown output still requires a model finalizer or the existing agent loop.

It can reduce inference costs in four ways:

1. `response_type=markdown` removes raw HTML tags and scripts before the model sees the page.
2. `css_extractor` can return only selected page elements.
3. `outputs` can return targeted data such as emails, phone numbers, headings, links, metadata, and tables.
4. `autoparse=true` can return common product, article, job, property, contact, and event fields without a separate model call.

Markdown output has an important limitation. ZenRows documents that it converts the full page and can retain navigation, headers, footers, and sidebars. Freegent should not send this output directly to a model without its own sectioning, relevance selection, and evidence budget.

For common supported page types, Autoparse or deterministic selectors could let Freegent validate and store the result without inference. That is more valuable than a small reduction in input-token price.

## Comparison with OpenExtract

| Capability | OpenExtract | ZenRows |
|---|---|---|
| Ordinary HTTP retrieval | Impit on existing infrastructure | Managed basic request |
| JavaScript rendering | Local Patchright | Managed rendering at 5x |
| Residential proxies | Optional configured proxy | Premium proxy at 10x |
| Protected JavaScript pages | Local browser, proxy, and solver ladder | Combined mode at 25x |
| Adaptive escalation | Explicit local ladder | `mode=auto` |
| Failed attempt billing | Local infrastructure still runs | Failed internal attempts are not billed |
| Clean Markdown | Existing extraction and cleanup | Available with no separate Markdown surcharge |
| Targeted extraction | Existing structured data and future context selection | CSS, output filters, and Autoparse |
| Evidence selection | Planned query-aware section selection | Full-page Markdown still needs caller-side selection |
| Durable retries and result state | River and PostgreSQL | Not replaced |
| Research and final answer | Freegent model loop | Not provided |

ZenRows overlaps with OpenExtract's retrieval rungs, not with Freegent's orchestration and research responsibilities.

## Integration shape

Add ZenRows to Freegent's existing `fetch_page` provider chain after OpenExtract. Keep the model-facing tool contract unchanged.

The adapter should:

- call the Universal Scraper API with `mode=auto`
- request Markdown or targeted structured output
- preserve the final URL and response status
- read `X-Request-Cost` and `X-Request-Credits`
- record ZenRows as one extraction attempt
- retain the successful configuration and cost in provider diagnostics
- return the same Freegent tool result shape
- keep River as the durable retry owner

Managed provider selection belongs inside Freegent's `fetch_page` implementation. The model should not need to know whether evidence came from OpenExtract, ZenRows, Exa, or Tavily.

## Benchmark

Do not adopt ZenRows from list pricing alone. Run at least 500 representative URLs across:

- ordinary static pages
- JavaScript-rendered company and product pages
- Cloudflare, DataDome, Akamai, and PerimeterX targets
- geo-restricted pages
- known OpenExtract failures
- pages where the required evidence appears outside the first visible section

Measure:

- successful HTTP response rate
- usable-content success rate
- required-evidence recall
- cost per successful usable page
- response duration
- output characters and model input tokens
- percentage billed as basic, JavaScript, proxy, or combined
- schema validity after the cheap model finalizer
- percentage of rows that avoid inference through selectors or Autoparse

The decision metric is cost per valid supported answer, not cost per request.

## Recommendation

Keep OpenExtract as the default. Add ZenRows only after a benchmark shows one of these outcomes:

- it recovers enough current OpenExtract failures to justify the subscription
- it removes enough local proxy, solver, and browser maintenance to justify outsourcing
- its structured extraction lets a material share of rows skip model inference
- it reduces evidence size without lowering required-evidence recall

If adopted, start with ZenRows after OpenExtract and before Exa and Tavily. Move it earlier only when measured operational savings exceed the additional subscription cost.

## Sources

- [ZenRows pricing](https://docs.zenrows.com/first-steps/pricing)
- [Universal Scraper API](https://docs.zenrows.com/universal-scraper-api/api-reference)
- [Adaptive Stealth Mode](https://docs.zenrows.com/universal-scraper-api/features/adaptive-stealth-mode)
- [Markdown response](https://docs.zenrows.com/universal-scraper-api/features/markdown)
- [Autoparse](https://docs.zenrows.com/universal-scraper-api/features/autoparse)
- [Output filters](https://docs.zenrows.com/universal-scraper-api/features/output-filters)
- [ZenRows MCP](https://docs.zenrows.com/integrations/mcp/mcp-overview)
