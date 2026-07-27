import { chromium, type Browser, type Page } from "patchright";
import { extract } from "./extract.ts";

type TokenVendor = "turnstile" | "hcaptcha" | "recaptcha";
type SolverName = "capsolver" | "twocaptcha";

type SolverSolution = {
  token?: string;
  gRecaptchaResponse?: string;
  cookies?: Record<string, string>;
  userAgent?: string;
};

type SolverTaskResponse = {
  errorId?: number;
  errorCode?: string;
  errorDescription?: string;
  taskId?: string | number;
  status?: string;
  solution?: SolverSolution;
};

const proxyConfig = {
  username: process.env.EVOMI_USERNAME ?? "",
  password: process.env.EVOMI_PASSWORD ?? "",
  gateway: process.env.EVOMI_GATEWAY ?? "",
};

const capsolverKey = process.env.CAPSOLVER_API_KEY ?? "";
const twoCaptchaKey = process.env.TWOCAPTCHA_API_KEY ?? "";
const hasProxy = Boolean(proxyConfig.username && proxyConfig.password && proxyConfig.gateway);
const hasSolver = Boolean(capsolverKey || twoCaptchaKey);
const port = Number.parseInt(process.env.PORT ?? "8081", 10);

const challengePattern =
  /just a moment|checking your browser|cf-browser-verification|verifying you are human|enable javascript|captcha/i;
const turnstileSitekeyPattern = /(?:data-sitekey|sitekey["'\s:=]+)["']?(0x[0-9A-Za-z_-]{20,})/;

const solvers = {
  capsolver: {
    key: capsolverKey,
    baseURL: "https://api.capsolver.com",
    tasks: {
      turnstile: (url: string, sitekey: string) => ({
        type: "AntiTurnstileTaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      hcaptcha: (url: string, sitekey: string) => ({
        type: "HCaptchaTaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      recaptcha: (url: string, sitekey: string) => ({
        type: "ReCaptchaV2TaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
    },
  },
  twocaptcha: {
    key: twoCaptchaKey,
    baseURL: "https://api.2captcha.com",
    tasks: {
      turnstile: (url: string, sitekey: string) => ({
        type: "TurnstileTaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      hcaptcha: (url: string, sitekey: string) => ({
        type: "HCaptchaTaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      recaptcha: (url: string, sitekey: string) => ({
        type: "RecaptchaV2TaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
    },
  },
};

function solverOrder(preferred?: string): SolverName[] {
  const available = (Object.keys(solvers) as SolverName[]).filter((name) => solvers[name].key);
  if (preferred === "capsolver" || preferred === "twocaptcha") {
    if (solvers[preferred].key) {
      return [preferred, ...available.filter((name) => name !== preferred)];
    }
  }
  return available;
}

function turnstileSolved(html: string): boolean {
  return /cf-turnstile-response["'\s:=]+[^"'\s>]{20,}|name=["']cf-turnstile-response["'][^>]*value=["'][^"'\s>]{20,}/i.test(
    html,
  );
}

async function launchBrowser(attempts = 6): Promise<Browser> {
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

function stickySession() {
  const sessionID = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e6).toString(36)}`;
  const password = `${proxyConfig.password}_session-${sessionID}`;
  return {
    browserProxy: {
      server: `http://${proxyConfig.gateway}`,
      username: proxyConfig.username,
      password,
    },
    solverProxy: `${proxyConfig.gateway}:${proxyConfig.username}:${password}`,
  };
}

async function createSolverTask(
  baseURL: string,
  clientKey: string,
  task: Record<string, unknown>,
): Promise<SolverSolution> {
  const created = await fetch(`${baseURL}/createTask`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ clientKey, task }),
  }).then((response) => response.json() as Promise<SolverTaskResponse>);
  if (created.errorId) {
    throw new Error(`createTask: ${created.errorDescription ?? created.errorCode}`);
  }
  const taskID = created.taskId;
  if (!taskID) throw new Error("createTask returned no taskId");

  for (let attempt = 0; attempt < 40; attempt++) {
    await Bun.sleep(3000);
    const result = await fetch(`${baseURL}/getTaskResult`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ clientKey, taskId: taskID }),
    }).then((response) => response.json() as Promise<SolverTaskResponse>);
    if (result.errorId) {
      throw new Error(`getTaskResult: ${result.errorDescription ?? result.errorCode}`);
    }
    if (result.status === "ready") {
      return result.solution ?? {};
    }
  }
  throw new Error("solver timeout");
}

async function solveToken(
  vendor: TokenVendor,
  target: string,
  sitekey: string | null,
  preferred?: string,
): Promise<{ token: string; provider: SolverName }> {
  if (!sitekey) throw new Error(`no sitekey for ${vendor}`);
  let lastError: unknown;
  for (const name of solverOrder(preferred)) {
    try {
      const solver = solvers[name];
      const solution = await createSolverTask(solver.baseURL, solver.key, solver.tasks[vendor](target, sitekey));
      const token = solution.token ?? solution.gRecaptchaResponse;
      if (token) return { token, provider: name };
      lastError = new Error("empty solution");
    } catch (error) {
      lastError = error;
      console.error(`solver failed provider=${name} vendor=${vendor} error=${firstLine(error)}`);
    }
  }
  throw new Error(`no token for ${vendor}: ${firstLine(lastError ?? "no solver configured")}`);
}

async function solveCloudflare(target: string, proxy: string): Promise<SolverSolution> {
  return createSolverTask(solvers.capsolver.baseURL, capsolverKey, {
    type: "AntiCloudflareTask",
    websiteURL: target,
    proxy,
  });
}

async function clickTurnstile(page: Page): Promise<boolean> {
  const element = await page.$(".cf-turnstile, [data-sitekey]");
  if (!element) return false;
  const box = await element.boundingBox();
  if (!box) return false;
  const x = box.x + 28;
  const y = box.y + box.height / 2;
  await page.mouse.move(x - 60, y - 25, { steps: 8 });
  await page.mouse.move(x, y, { steps: 14 });
  await page.waitForTimeout(250 + Math.random() * 400);
  await page.mouse.click(x, y);
  return true;
}

async function injectTurnstileToken(page: Page, token: string): Promise<void> {
  await page.evaluate((value) => {
    for (const element of document.querySelectorAll<
      HTMLInputElement | HTMLTextAreaElement
    >(
      'input[name="cf-turnstile-response"], textarea[name="cf-turnstile-response"], input[name="g-recaptcha-response"]',
    )) {
      element.value = value;
      element.dispatchEvent(new Event("input", { bubbles: true }));
      element.dispatchEvent(new Event("change", { bubbles: true }));
    }
    const scope = window as typeof window & {
      __turnstileCb?: (token: string) => void;
      turnstileCallback?: (token: string) => void;
      onTurnstileSuccess?: (token: string) => void;
    };
    const callback = scope.__turnstileCb ?? scope.turnstileCallback ?? scope.onTurnstileSuccess;
    if (typeof callback === "function") {
      try {
        callback(value);
      } catch {}
    }
    for (const form of document.querySelectorAll("form")) {
      if (!form.querySelector('[name="cf-turnstile-response"]')) continue;
      try {
        form.requestSubmit ? form.requestSubmit() : form.submit();
      } catch {}
    }
  }, token);
}

async function solveTurnstile(
  page: Page,
  target: string,
  initialHTML: string,
  preferred?: string,
): Promise<string> {
  const sitekey = initialHTML.match(turnstileSitekeyPattern)?.[1] ?? null;
  if (!sitekey) return initialHTML;

  await clickTurnstile(page).catch(() => false);
  await page.waitForTimeout(4000);
  await page.waitForLoadState("networkidle", { timeout: 8000 }).catch(() => {});
  let html = await page.content();
  if (turnstileSolved(html) || !turnstileSitekeyPattern.test(html)) return html;

  if (solverOrder(preferred).length > 0) {
    const solution = await solveToken("turnstile", target, sitekey, preferred).catch((error) => {
      console.error(`turnstile solver failed error=${firstLine(error)}`);
      return null;
    });
    if (solution?.token) {
      await injectTurnstileToken(page, solution.token);
      await page.waitForTimeout(2500);
      await page.waitForLoadState("networkidle", { timeout: 8000 }).catch(() => {});
      html = await page.content();
    }
  }
  return html;
}

async function render(
  browser: Browser,
  target: string,
  options: { useProxy: boolean; solve: boolean; solver?: string },
): Promise<string> {
  const session = options.useProxy && hasProxy ? stickySession() : null;
  const contextOptions = {
    viewport: null,
    ...(session ? { proxy: session.browserProxy } : {}),
  };
  let context = await browser.newContext(contextOptions);
  try {
    let page = await context.newPage();
    await page.goto(target, { waitUntil: "domcontentloaded", timeout: 30000 });
    await page.waitForLoadState("networkidle", { timeout: 10000 }).catch(() => {});
    for (let attempt = 0; attempt < 4 && challengePattern.test(await page.title().catch(() => "")); attempt++) {
      await page.waitForTimeout(2500);
    }
    let html = await page.content();

    if (options.solve && turnstileSitekeyPattern.test(html) && !challengePattern.test(html)) {
      html = await solveTurnstile(page, target, html, options.solver);
    }

    if (options.solve && session && capsolverKey && challengePattern.test(html)) {
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

function firstLine(error: unknown): string {
  const value = error instanceof Error ? error.message : String(error);
  return value.split("\n")[0] ?? value;
}

const browser = await launchBrowser();

const server = Bun.serve({
  hostname: "0.0.0.0",
  port,
  async fetch(request) {
    const requestURL = new URL(request.url);
    if (requestURL.pathname === "/healthz") {
      return Response.json({
        ok: true,
        proxy: hasProxy,
        solvers: solverOrder(),
      });
    }
    if (requestURL.pathname === "/extract" && request.method === "POST") {
      try {
        const body = (await request.json()) as { url?: unknown };
        if (typeof body.url !== "string") {
          return Response.json({ error: "url is required" }, { status: 400 });
        }
        const started = performance.now();
        const result = await extract(body.url, (target, options) => render(browser, target, options), {
            proxy: hasProxy,
            solver: hasSolver,
        });
        console.log(
          `extract outcome=${result.outcome} provider=${result.provider} attempts=${result.attempts.length} durationMs=${Math.round(performance.now() - started)} url=${result.url}`,
        );
        return Response.json(result);
      } catch (error) {
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
  `openextract service listening port=${server.port} proxy=${hasProxy ? "on" : "off"} solvers=${solverOrder().join(",") || "none"}`,
);
