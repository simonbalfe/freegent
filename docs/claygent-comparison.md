# Freegent and Claygent cost comparison

This benchmark compares Freegent with Claygent for one structured web-research prompt per company. Pricing and model availability change, so treat it as a dated calculation rather than a permanent claim.

## Benchmark

Measured on August 2, 2026:

- source list: 4,093 PLG SaaS companies
- sample: first 50 rows
- task: find confirmed upcoming events and enumerate product integrations
- model: `deepseek/deepseek-v4-flash` through OpenRouter
- search: Serper with provider fallbacks available
- worker concurrency: 50

The 50-row run used 1,390,332 input tokens, 112,681 output tokens, and 129 Serper searches. It finished in 2 minutes 26 seconds of wall time. The rows contained 49 minutes 19 seconds of combined agent work because they ran concurrently.

The measured provider cost was estimated at $0.3552:

| Provider | Calculation | Cost |
|---|---:|---:|
| OpenRouter input | 1,390,332 tokens at $0.14 per million | $0.1946 |
| OpenRouter output | 112,681 tokens at $0.28 per million | $0.0316 |
| Serper | 129 searches at $0.001 | $0.1290 |
| Total | 50 rows | $0.3552 |

At the same usage per row, the complete 4,093-row sheet projects to:

| Metric | Projection |
|---|---:|
| Input tokens | 113.8 million |
| Output tokens | 9.2 million |
| Serper searches | 10,560 |
| Provider cost | $29.08 |
| Runtime at 50 concurrent workers | About 1 hour 21 minutes |

The runtime projection uses combined row work divided by worker concurrency. Allow additional time for provider rate limits, retries, and unusually slow rows.

## Claygent equivalent

An equivalent Clay table should use one Claygent prompt per row and return both research areas as one structured result. Clay charges one Action per AI prompt. Fixed web-research models currently include GPT-5 Nano at 0.5 Data Credits per row, Helium, GPT-5 Mini, and Gemini 2.5 Flash at 1 credit per row, and Argon at 3 credits per row.

For 4,093 rows:

| Option | Data Credits | Credit value at $0.05 each | Approximate paid usage |
|---|---:|---:|---:|
| Freegent | Not applicable | Not applicable | $29.08 |
| Claygent GPT-5 Nano | 2,046.5 | $102.33 | $185 Launch plan |
| Claygent 1-credit model | 4,093 | $204.65 | About $289 for Launch and top-ups |
| Claygent Argon | 12,279 | $613.95 | About $821 for Launch and top-ups |

The paid Clay estimates use the published Launch starting price of $185 per month with 2,500 Data Credits and 15,000 Actions, plus extra Data Credits at the published 30 percent top-up premium. Larger commitments can reduce the effective credit price.

Clay also offers variable-priced advanced models and bring-your-own-key usage. Variable models charge actual model cost plus one Action per prompt. Bring-your-own-key runs avoid Data Credit charges but still consume Actions and require the Clay platform plan.

If events and integrations are implemented as two separate Claygent prompts, Actions and fixed AI credits approximately double.

## What the comparison means

Freegent is cheaper in this benchmark because it pays OpenRouter and search providers directly, uses an inexpensive model, and runs on self-hosted infrastructure without a Clay subscription. Clay's price includes its hosted product, managed rate limits, GTM integrations, support, and broader workflow tooling.

This is not yet a quality benchmark. The original Freegent sample completed 46 of 50 rows, and several outputs needed stricter schema handling. Freegent now rejects unsupported schema shorthand and treats failed page fetches as recoverable tool errors. Repeat the same 50-row sample in both products before making a production buying decision.

## Sources

- [Clay AI pricing and model credit reference](https://university.clay.com/docs/ai-pricing)
- [Claygent pricing mechanics](https://www.clay.com/claygent)
- [Clay plans, Data Credits, Actions, and top-ups](https://www.clay.com/pricing)
- [OpenRouter DeepSeek V4 Flash](https://openrouter.ai/deepseek/deepseek-v4-flash)
- [Serper pricing](https://serper.dev/)
