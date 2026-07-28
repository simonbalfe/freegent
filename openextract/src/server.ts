import { z } from "zod";
import { launchBrowser, render } from "./browser/browser.ts";
import { hasProxy } from "./browser/proxy.ts";
import { hasSolver, solverOrder } from "./browser/solvers.ts";
import { ConcurrencyLimiter, QueueAbortedError, QueueSaturatedError } from "./concurrency/limiter.ts";
import { parseEnvironment } from "./config/environment.ts";
import { extract } from "./extraction/extract.ts";
import { firstLine } from "./support/errors.ts";

const extractRequestSchema = z.object({ url: z.string() });
const environment = parseEnvironment(process.env);
const extractionLimiter = new ConcurrencyLimiter(
  environment.OPENEXTRACT_MAX_CONCURRENCY,
  environment.OPENEXTRACT_MAX_WAITING,
);
const browserLimiter = new ConcurrencyLimiter(
  environment.OPENEXTRACT_BROWSER_CONCURRENCY,
  environment.OPENEXTRACT_MAX_WAITING,
);
const browser = await launchBrowser();

const server = Bun.serve({
  hostname: "0.0.0.0",
  port: environment.PORT,
  async fetch(request) {
    const requestURL = new URL(request.url);
    if (requestURL.pathname === "/healthz") {
      return Response.json({
        ok: true,
        proxy: hasProxy,
        solvers: solverOrder(),
        limits: {
          extraction: extractionLimiter.stats,
          browser: browserLimiter.stats,
        },
      });
    }
    if (requestURL.pathname === "/extract" && request.method === "POST") {
      try {
        const body = extractRequestSchema.parse(await request.json());
        const started = performance.now();
        const result = await extractionLimiter.run(
          () =>
            extract(
              body.url,
              (target, options) =>
                browserLimiter.run(() => render(browser, target, options), request.signal),
              {
                proxy: hasProxy,
                solver: hasSolver,
              },
            ),
          request.signal,
        );
        console.log(
          `extract outcome=${result.outcome} provider=${result.provider} attempts=${result.attempts.length} durationMs=${Math.round(performance.now() - started)} url=${result.url}`,
        );
        return Response.json(result);
      } catch (error) {
        if (error instanceof QueueSaturatedError) {
          return Response.json(
            {
              error: error.message,
              limits: {
                extraction: extractionLimiter.stats,
                browser: browserLimiter.stats,
              },
            },
            { status: 429, headers: { "retry-after": "1" } },
          );
        }
        if (error instanceof QueueAbortedError) {
          return Response.json({ error: error.message }, { status: 408 });
        }
        console.error(`extract failed error=${firstLine(error)}`);
        return Response.json({ error: firstLine(error) }, { status: 400 });
      }
    }
    return new Response("not found", { status: 404 });
  },
});

async function shutdown(): Promise<void> {
  server.stop();
  await browser.close().catch(() => {});
  process.exit(0);
}

process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);

console.log(
  `openextract service listening port=${server.port} proxy=${hasProxy ? "on" : "off"} solvers=${solverOrder().join(",") || "none"} concurrency=${environment.OPENEXTRACT_MAX_CONCURRENCY} browserConcurrency=${environment.OPENEXTRACT_BROWSER_CONCURRENCY} maxWaiting=${environment.OPENEXTRACT_MAX_WAITING}`,
);
