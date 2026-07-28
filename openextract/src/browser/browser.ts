import { chromium, type Browser } from "patchright";
import type { BrowserOptions } from "../extraction/types.ts";
import { firstLine } from "../support/errors.ts";
import { hasTurnstile, isChallenge, solveTurnstile } from "./challenges.ts";
import { createProxySession, hasProxy } from "./proxy.ts";
import { hasCapsolver, solveCloudflare } from "./solvers.ts";

export async function launchBrowser(attempts = 6): Promise<Browser> {
  for (let attempt = 1; attempt <= attempts; attempt++) {
    try {
      const browser = await chromium.launch({ headless: false });
      console.log(`browser launched attempt=${attempt}`);
      return browser;
    } catch (error) {
      console.error(`browser launch failed attempt=${attempt} error=${firstLine(error)}`);
      if (attempt === attempts) throw error;
      await Bun.sleep(3000);
    }
  }
  throw new Error("browser failed to launch");
}

export async function render(
  browser: Browser,
  target: string,
  options: BrowserOptions,
): Promise<string> {
  const session = options.useProxy && hasProxy ? createProxySession() : null;
  const contextOptions = {
    viewport: null,
    ...(session ? { proxy: session.browserProxy } : {}),
  };
  let context = await browser.newContext(contextOptions);
  try {
    let page = await context.newPage();
    await page.goto(target, { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => {});
    for (let attempt = 0; attempt < 4 && isChallenge(await page.title().catch(() => "")); attempt++) {
      await page.waitForTimeout(2500);
    }
    let html = await page.content();

    if (options.solve && hasTurnstile(html) && !isChallenge(html)) {
      html = await solveTurnstile(page, target, html, options.solver);
    }

    if (options.solve && session && hasCapsolver && isChallenge(html)) {
      const solution = await solveCloudflare(target, session.solverProxy);
      const cookies = Object.entries(solution.cookies ?? {}).map(([name, value]) => ({
        name,
        value: String(value),
        url: target,
      }));
      await context.close().catch(() => {});
      context = await browser.newContext({
        ...contextOptions,
        ...(solution.userAgent ? { userAgent: solution.userAgent } : {}),
      });
      if (cookies.length > 0) await context.addCookies(cookies);
      page = await context.newPage();
      await page.goto(target, { waitUntil: "domcontentloaded", timeout: 30000 });
      await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => {});
      html = await page.content();
    }
    return html;
  } finally {
    await context.close().catch(() => {});
  }
}
