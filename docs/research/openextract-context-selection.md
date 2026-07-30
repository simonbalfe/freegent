# OpenExtract context-selection research

Research date: 29 July 2026.

This is a dated research note, not current architecture documentation. The benchmark figures below do not yet have a reproducible script or stored result in this repository.

## Objective

OpenExtract should turn a URL into the smallest source-faithful evidence packet that lets an AI system answer a specific question correctly. It should also preserve access to the complete extracted document so a caller can inspect evidence, retry selection, or ask a follow-up question.

OpenExtract must remain deployment-agnostic:

- It is a stateless HTTP service that can run in Docker, a VM, Kubernetes, or any compatible application platform.
- It owns retrieval, browser escalation, document cleanup, structural parsing, and optional passage selection.
- It does not own durable queues, databases, object storage, web search, row orchestration, an agent loop, or final-answer generation.
- Model, embedding, and reranking integrations must be optional adapters rather than infrastructure requirements.

## Current position

The current extraction ladder is a good capture foundation:

1. Impit direct retrieval.
2. Patchright browser rendering.
3. Patchright with a proxy.
4. Patchright with a solver.
5. Tavily as the final fallback.

HTML extraction already combines:

- Mozilla Readability for article-like pages.
- DOM-density pruning when Readability is unsuitable.
- Markdown conversion with GitHub-flavoured tables.
- JSON-LD and metadata extraction.
- Same-site link discovery.
- PDF text extraction.

The missing layer is query-aware context selection. Today, successful content is truncated to the first 12,000 characters. This is inexpensive, but it can discard evidence near the end of a page and spends context on early sections that may not answer the caller's question.

## Local benchmark

A ten-page comparison against Cloudflare Browser Run `/markdown` found:

| Measure | OpenExtract | Browser Run `/markdown` |
| --- | ---: | ---: |
| Successful pages | 10 of 10 | 10 of 10 |
| Mean latency | 370 ms | 2,017 ms |
| Mean output length | 3,518 characters | 17,754 characters |
| Mean discovered links | 68 | 152 |

OpenExtract was about 5.4 times faster and returned about one fifth as much text on this sample. Browser Run preserved substantially more page material, including more navigation and peripheral content. This result supports keeping OpenExtract's cleanup pipeline, but it does not establish that shorter content has better evidence recall. Future evaluation must score whether the passages needed to answer each question survive cleanup and selection.

## What comparable systems do

### Firecrawl

Firecrawl now exposes page-level `question` and `highlights` formats. Its freeform question path gives the cleaned Markdown and query to a language model. Its direct-quote path parses Markdown into numbered heading, prose, table, and code spans, asks a specialised model to return relevant span indices, then reconstructs Markdown from the selected original spans. Selected table rows automatically regain their table header.

This is a useful source-fidelity pattern: the model chooses identifiers instead of rewriting evidence. The limitation is selector cost because the model still receives the page's candidate spans. It is selection, not retrieval over a compact candidate set.

Sources:

- [Firecrawl query transformer at the inspected commit](https://github.com/firecrawl/firecrawl/blob/1135c555c2c8a19d209d7f4aaf84f961c37a3970/apps/api/src/scraper/scrapeURL/transformers/query.ts)
- [Firecrawl structure-aware highlight spans](https://github.com/firecrawl/firecrawl/blob/1135c555c2c8a19d209d7f4aaf84f961c37a3970/apps/api/src/lib/highlight-spans.ts)

### Crawl4AI

Crawl4AI produces separate raw and fit Markdown. Its pruning filter scores DOM nodes using text density, link density, tag importance, structural position, and suspected boilerplate markers. Its BM25 filter tokenises page blocks and the user's query, boosts important tags such as headings, code, and table headers, filters by score, then returns selected blocks in document order. An LLM filter is also available.

The pruning plus BM25 path is the strongest portable baseline for OpenExtract because it is fast, explainable, and does not require a model service. BM25 alone will miss paraphrases and implicit semantic matches.

Sources:

- [Crawl4AI fit Markdown documentation](https://docs.crawl4ai.com/core/fit-markdown/)
- [Crawl4AI content-filter implementation](https://github.com/unclecode/crawl4ai/blob/cdf2ead7ed4b78594d06b87bae930a819c685825/crawl4ai/content_filter_strategy.py)

### Exa

Exa Contents can return complete text, generated summaries, or query-relevant highlights. Highlights are extractive, accept a custom query and character budget, and return per-highlight similarity scores. Exa recommends highlights for agent workflows because they use much less context than full text.

The extraction and ranking implementation is proprietary. Exa is therefore a useful quality baseline and optional provider, not a design dependency.

Source: [Exa Contents retrieval](https://exa.ai/docs/reference/contents-retrieval)

### Tavily

Tavily Extract accepts a query, reranks extracted chunks, and returns between one and five chunks per source. Each returned chunk is capped at 500 characters. This prevents context growth but may omit surrounding qualifications, heading context, or long table rows.

OpenExtract should retain Tavily as a fallback and benchmark its focused extraction mode, but should not adopt its fixed small-chunk budget as the internal document model.

Source: [Tavily Extract best practices](https://docs.tavily.com/documentation/best-practices/best-practices-extract)

### LangChain contextual compression

LangChain's contextual compression pattern wraps a broad retriever with a document compressor. `LLMChainExtractor` asks a model to extract only the portions relevant to the query. Other compressors can filter by embedding similarity or rerank candidates with a cross-encoder.

This validates a layered interface, but OpenExtract does not need LangChain or its agent abstractions. The useful idea is the separation between high-recall retrieval and high-precision compression.

Sources:

- [LangChain LLMChainExtractor](https://reference.langchain.com/python/langchain-classic/retrievers/document_compressors/chain_extract/LLMChainExtractor)
- [LangChain contextual reranking example](https://docs.langchain.com/oss/python/integrations/retrievers/cohere-reranker)

### Contextual Retrieval

Anthropic's Contextual Retrieval work found that semantic embeddings and BM25 solve complementary failure modes. Their approach adds short document-specific context to chunks before indexing, combines semantic and lexical retrieval, and then reranks a larger candidate set. In their evaluation, contextual embeddings plus contextual BM25 and reranking reduced the top-20 retrieval failure rate by 67 percent relative to their embeddings-only baseline.

The general lesson is to preserve heading and document context with each section, combine exact and semantic retrieval, and rerank only the candidate set.

Source: [Anthropic Contextual Retrieval](https://www.anthropic.com/engineering/contextual-retrieval)

### LongLLMLingua

LongLLMLingua uses a smaller language model to rank contexts and remove lower-value contexts, sentences, and tokens before the main model call. This can produce large token savings, but token-level deletion makes citations, exact quotations, and source auditing harder.

OpenExtract should prefer complete extractive sections. Token-level prompt compression can remain a caller-side optimisation after evidence retrieval.

Source: [Microsoft LLMLingua](https://github.com/microsoft/LLMLingua)

## Recommended document model

Capture and selection should be separate phases.

The capture phase should produce a canonical document:

```ts
type ExtractDocument = {
  url: string;
  title?: string;
  content: string;
  contentType: "html" | "pdf" | "text" | "unknown";
  sections: ExtractSection[];
  links: string[];
  contentHash: string;
};

type ExtractSection = {
  id: string;
  ordinal: number;
  kind: "heading" | "prose" | "list" | "table" | "code" | "structured-data";
  headingPath: string[];
  text: string;
  tokenCount: number;
};
```

Section identifiers must be stable for the same normalized document. Selection results should refer to these identifiers and return unchanged section text. A caller can then cite, cache, compare, and request neighbouring sections without trusting generated prose.

The selection phase should accept:

```ts
type SelectionRequest = {
  query: string;
  maxTokens: number;
  maxSections?: number;
};

type SelectionResult = {
  sections: Array<{
    id: string;
    text: string;
    score: number;
    reasons: string[];
  }>;
  omittedSectionCount: number;
  tokenCount: number;
};
```

These types illustrate the intended boundary and are not yet an API commitment.

## Recommended selection pipeline

### 1. Structural sectioning

Split normalized content using document structure rather than fixed character windows:

- Heading boundaries with the complete heading path attached.
- Paragraph and list-item groups.
- Whole table rows with their headers.
- Complete code blocks.
- Small overlap only when a semantic unit exceeds the section budget.

Keep adjacent short blocks together when separating them would remove meaning. Preserve original document order and stable section identifiers.

### 2. Portable high-recall retrieval

The default selector should require no external service:

- BM25 over section text, heading paths, title, and structured metadata.
- Exact-match boosts for quoted phrases, identifiers, prices, dates, and names.
- Heading, table-header, code, and structured-data boosts.
- Neighbour expansion around high-scoring sections.

This stage should return more candidates than will fit in the final budget.

### 3. Optional semantic retrieval

An embedding adapter may retrieve sections that use different language from the query. Merge lexical and semantic ranks with reciprocal rank fusion. The core package should define a small provider interface and ship without requiring a particular vector database or cloud.

For a single freshly fetched page, embeddings can remain in memory. Persistent indexes belong to the caller.

### 4. Optional reranking

A cross-encoder reranker should score the query and each candidate together. It is cheaper and more constrained than asking a general-purpose agent to read the complete document. The adapter should receive section identifiers and text, then return identifiers and scores.

An optional LLM selector can follow Firecrawl's identifier-selection pattern when a cross-encoder is unavailable or evaluation shows a material quality gain. It must select section identifiers only. It must not rewrite evidence.

### 5. Budgeted evidence packing

Pack the reranked sections under the requested token budget while:

- Preserving selected section text exactly.
- Restoring heading paths and table headers.
- Including a neighbouring section when it resolves pronouns or qualifications.
- Deduplicating repeated navigation, legal text, and structured-data facts.
- Returning selected sections in original document order.

If evidence is incomplete, the caller should issue another focused selection request. OpenExtract should not run an autonomous agent loop.

## API direction

Keep `POST /extract` backward-compatible while adding explicit document and selection capabilities.

A practical sequence is:

1. Add structured `sections` and complete normalized content to extraction results.
2. Add an optional `selection` request containing `query` and `maxTokens`.
3. Return both the complete document and the selected evidence when requested.
4. Add a selection-only endpoint or library function for callers that already hold a captured document.
5. Add optional embedding and reranking adapters after the portable baseline is measured.

Do not truncate content before sectioning and selection. If transport limits require bounding the synchronous response, expose that condition explicitly and provide a streaming or caller-supplied storage mechanism. Silent first-N truncation is unacceptable for evidence retrieval.

## Evaluation plan

Build a versioned evaluation set containing at least:

- Documentation pages with deep answers.
- Pricing and product pages.
- Pages with repeated navigation and promotional blocks.
- JavaScript-rendered applications.
- Tables where headers are required to interpret cells.
- PDFs.
- Pages where the correct answer appears near the end.
- Pages containing prompt-injection-like text.

For every URL and question, record the gold evidence spans and acceptable answers. Measure:

- Evidence recall at the final token budget.
- Evidence precision and boilerplate ratio.
- Downstream answer correctness.
- Exact-quote and table fidelity.
- Context tokens delivered to the answering model.
- Extraction success rate.
- Selection latency and provider cost.
- Stability across repeated runs.

Compare:

1. Current OpenExtract output.
2. Complete Markdown truncated from the beginning.
3. Structural pruning only.
4. Structural pruning plus BM25.
5. BM25 plus embeddings and reciprocal rank fusion.
6. Hybrid retrieval plus a cross-encoder reranker.
7. Firecrawl highlights.
8. Exa Highlights.
9. Tavily query-aware Extract.

Quality gates should be expressed as evidence recall and answer correctness at a fixed context budget. Output length alone is not a quality metric.

## Prioritised roadmap

### P0: prevent evidence loss

- Remove silent first-12,000-character truncation before selection.
- Record normalized-content length, returned-content length, and truncation state.
- Add regression cases where evidence appears at the end of a long page.

### P1: introduce the canonical document

- Parse normalized Markdown into stable structural sections.
- Preserve heading paths, table headers, code blocks, and original order.
- Return stable section identifiers and token counts.

### P2: ship portable query-aware selection

- Add BM25 and exact-match scoring.
- Add structural boosts, neighbour expansion, and token-budget packing.
- Return scores and concise deterministic reasons for selection.

### P3: add provider adapters

- Define optional embedding and reranking interfaces.
- Merge BM25 and semantic candidates using reciprocal rank fusion.
- Keep provider credentials and infrastructure outside the core algorithm.

### P4: progressive retrieval

- Let callers select again from a captured document using a narrower query.
- Support requesting neighbouring sections by identifier.
- Expose insufficient-evidence signals without generating an answer.

### P5: evaluate before adding generative compression

- Run the versioned benchmark across the portable and provider-backed selectors.
- Add an identifier-only LLM selector only if it materially improves evidence recall at the same budget.
- Keep summaries and token-level compression outside the evidence contract.

## Decision

OpenExtract should become a source-faithful capture and passage-selection primitive, not an extraction agent.

The default path should be:

```text
retrieve
  -> clean and normalize
  -> preserve complete document
  -> create structural sections
  -> BM25 and exact-match candidate retrieval
  -> optional semantic retrieval
  -> optional cross-encoder reranking
  -> budgeted original sections
```

This design can run anywhere, avoids paying an agent to rediscover relevance on every call, and still permits a caller's agent to ask for more evidence when needed.
