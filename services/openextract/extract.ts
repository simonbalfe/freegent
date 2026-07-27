import { tavily } from "@tavily/core";
import { Impit } from "impit";
import { extractHTML } from "./html.ts";
import { pdfToText } from "./pdf.ts";
import type { BrowserOptions, ExtractAttempt, ExtractProvider, ExtractResult } from "./types.ts";

type Retrieved = {
  content: string;
  contentType: ExtractResult["contentType"];
  links: string[];
  status?: number;
};

type Rung = {
  provider: ExtractProvider;
  enabled: boolean;
  retrieve: (url: string) => Promise<Retrieved>;
};

type BrowserRender = (url: string, options: BrowserOptions) => Promise<string>;

type Capabilities = {
  proxy: boolean;
  solver: boolean;
};

const MAX_CHARACTERS = 12_000;
const MIN_USABLE_CHARACTERS = 200;
const DEAD_STATUSES = new Set([401, 404, 410]);
const SHELL_MARKERS = [
  "enable javascript",
  "please enable js",
  "you need to enable javascript",
  "checking your browser",
  "captcha",
  "are you a human",
  "access denied",
  "cf-browser-verification",
  "request unsuccessful",
  "ddos protection",
];

const impit = new Impit({ browser: "chrome", timeout: 15_000 });

function debug(message: string): void {
  if (process.env.OPEN_EXTRACT_DEBUG === "1") console.error(`[openextract] ${message}`);
}

function validateURL(input: string): string {
  const url = new URL(input);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error("URL must use http or https");
  }
  return url.href;
}

function isUsable(content: string, contentType: ExtractResult["contentType"]): boolean {
  if (contentType === "pdf") return content.trim().length >= 10;
  if (content.length < MIN_USABLE_CHARACTERS) return false;
  const head = content.slice(0, 4000).toLowerCase();
  return !SHELL_MARKERS.some((marker) => head.includes(marker));
}

function bounded(content: string): string {
  const value = content.trim();
  if (value.length <= MAX_CHARACTERS) return value;
  return `${value.slice(0, MAX_CHARACTERS)}\n\n[truncated]`;
}

function classify(result: Retrieved): ExtractAttempt["outcome"] {
  if (result.status !== undefined && result.status >= 400) {
    return result.status === 403 || result.status === 429 ? "blocked" : "http-error";
  }
  if (!result.content) return "empty";
  return isUsable(result.content, result.contentType) ? "ok" : "blocked";
}

function detail(result: Retrieved): string | undefined {
  if (result.status !== undefined && result.status >= 400) return `HTTP ${result.status}`;
  if (!result.content) return "No content returned";
  if (result.contentType !== "pdf" && result.content.length < MIN_USABLE_CHARACTERS) {
    return `Only ${result.content.length} characters returned`;
  }
  if (!isUsable(result.content, result.contentType)) return "Response appears to be a JavaScript shell or block page";
  return undefined;
}

async function retrieveWithImpit(url: string): Promise<Retrieved> {
  const response = await impit.fetch(url);
  if (!response.ok) return { content: "", contentType: "unknown", links: [], status: response.status };
  const header = response.headers.get("content-type")?.toLowerCase() ?? "";
  if (header.includes("pdf") || new URL(url).pathname.toLowerCase().endsWith(".pdf")) {
    return {
      content: await pdfToText(await response.arrayBuffer()),
      contentType: "pdf",
      links: [],
      status: response.status,
    };
  }
  if (header.includes("html") || !header) {
    const extracted = extractHTML(await response.text(), url);
    return { ...extracted, contentType: "html", status: response.status };
  }
  if (header.includes("text")) {
    return { content: (await response.text()).trim(), contentType: "text", links: [], status: response.status };
  }
  return { content: "", contentType: "unknown", links: [], status: response.status };
}

function browserRetriever(render: BrowserRender, useProxy: boolean, solve: boolean): (url: string) => Promise<Retrieved> {
  return async (url) => {
    const html = await render(url, { useProxy, solve });
    const extracted = extractHTML(html, url);
    return { ...extracted, contentType: "html", status: 200 };
  };
}

function tavilyRetriever(apiKey: string): (url: string) => Promise<Retrieved> {
  const client = tavily({ apiKey });
  return async (url) => {
    const response = await client.extract([url], { extractDepth: "advanced", format: "markdown" });
    return {
      content: response.results[0]?.rawContent?.trim() ?? "",
      contentType: "text",
      links: [],
    };
  };
}

function ladder(render: BrowserRender, capabilities: Capabilities): Rung[] {
  const tavilyAPIKey = process.env.TAVILY_API_KEY ?? "";
  return [
    { provider: "impit", enabled: true, retrieve: retrieveWithImpit },
    { provider: "patchright", enabled: true, retrieve: browserRetriever(render, false, false) },
    {
      provider: "patchright+proxy",
      enabled: capabilities.proxy,
      retrieve: browserRetriever(render, true, false),
    },
    {
      provider: "patchright+solver",
      enabled: capabilities.solver,
      retrieve: browserRetriever(render, capabilities.proxy, true),
    },
    { provider: "tavily", enabled: Boolean(tavilyAPIKey), retrieve: tavilyRetriever(tavilyAPIKey) },
  ];
}

export async function extract(input: string, render: BrowserRender, capabilities: Capabilities): Promise<ExtractResult> {
  const url = validateURL(input);
  const attempts: ExtractAttempt[] = [];
  let lastProvider: ExtractProvider = "impit";
  let lastType: ExtractResult["contentType"] = "unknown";

  debug(`start ${url}`);
  for (const rung of ladder(render, capabilities)) {
    lastProvider = rung.provider;
    if (!rung.enabled) {
      attempts.push({ provider: rung.provider, outcome: "skipped", durationMs: 0, detail: "Not configured" });
      continue;
    }
    const started = performance.now();
    try {
      const result = await rung.retrieve(url);
      lastType = result.contentType;
      const outcome = classify(result);
      const attemptDetail = detail(result);
      attempts.push({
        provider: rung.provider,
        outcome,
        ...(result.status === undefined ? {} : { status: result.status }),
        durationMs: Math.round(performance.now() - started),
        ...(attemptDetail ? { detail: attemptDetail } : {}),
      });
      if (result.status !== undefined && DEAD_STATUSES.has(result.status)) {
        return {
          url,
          content: "",
          contentType: result.contentType,
          provider: rung.provider,
          outcome: "dead",
          links: [],
          attempts,
        };
      }
      if (outcome === "ok") {
        return {
          url,
          content: bounded(result.content),
          contentType: result.contentType,
          provider: rung.provider,
          outcome: "ok",
          links: result.links,
          attempts,
        };
      }
    } catch (error) {
      attempts.push({
        provider: rung.provider,
        outcome: "error",
        durationMs: Math.round(performance.now() - started),
        detail: error instanceof Error ? error.message : String(error),
      });
    }
  }

  return {
    url,
    content: "",
    contentType: lastType,
    provider: lastProvider,
    outcome: "failed",
    links: [],
    attempts,
  };
}
