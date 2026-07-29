import { z } from "zod";
import { browserArtifactSchema, type BrowserTaskMessage } from "./contracts";
import type { Env } from "./env";

const browserResponseSchema = z.object({
  success: z.literal(true),
  result: z.string(),
});

export async function executeBrowserTask(env: Env, task: BrowserTaskMessage): Promise<void> {
  const existing = await env.ARTIFACTS.get(task.resultKey);
  if (existing === null) {
    const response = await env.BROWSER.quickAction("markdown", {
      url: task.url,
      gotoOptions: { waitUntil: "networkidle2" },
      rejectResourceTypes: ["image", "media", "font"],
    });
    if (!response.ok) throw new Error(`Browser Run returned ${response.status}: ${await response.text()}`);
    const payload = browserResponseSchema.parse(await response.json());
    const browserMsHeader = response.headers.get("X-Browser-Ms-Used");
    const parsedBrowserMs = browserMsHeader === null ? Number.NaN : Number.parseInt(browserMsHeader, 10);
    const artifact = browserArtifactSchema.parse({
      url: task.url,
      text: bounded(payload.result, 50_000),
      links: markdownLinks(payload.result),
      provider: "cloudflare-browser-run",
      browserMs: Number.isFinite(parsedBrowserMs) ? parsedBrowserMs : null,
    });
    await env.ARTIFACTS.put(task.resultKey, JSON.stringify(artifact), {
      httpMetadata: { contentType: "application/json" },
    });
  }
  const workflow = await env.RESEARCH_WORKFLOW.get(task.workflowId);
  try {
    await workflow.sendEvent({
      type: task.eventType,
      payload: { resultKey: task.resultKey },
    });
  } catch (error) {
    const status = await workflow.status();
    if (status.status !== "complete" && status.status !== "errored" && status.status !== "terminated") {
      throw error;
    }
  }
}

export function markdownLinks(markdown: string): string[] {
  const values = new Set<string>();
  for (const match of markdown.matchAll(/\[[^\]]*]\((https?:\/\/[^)\s]+)\)/g)) {
    const value = match[1];
    if (value === undefined) continue;
    try {
      const url = new URL(value);
      url.hash = "";
      values.add(url.href);
    } catch {
      continue;
    }
  }
  return [...values];
}

function bounded(value: string, limit: number): string {
  return value.length <= limit ? value : `${value.slice(0, limit)}\n\n[truncated]`;
}
