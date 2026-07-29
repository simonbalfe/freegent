import { z } from "zod";
import type { ToolResult } from "./contracts";

const serperResponseSchema = z.object({
  organic: z
    .array(
      z.object({
        title: z.string().default(""),
        link: z.string().url(),
        snippet: z.string().default(""),
        date: z.string().optional(),
      }),
    )
    .default([]),
});

export async function searchWeb(apiKey: string, query: string): Promise<ToolResult> {
  if (apiKey.length === 0) throw new Error("SERPER_API_KEY is not configured");
  const response = await fetch("https://google.serper.dev/search", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-API-KEY": apiKey,
    },
    body: JSON.stringify({ q: query, num: 10 }),
  });
  if (!response.ok) throw new Error(`Serper returned ${response.status}`);
  const payload = serperResponseSchema.parse(await response.json());
  const results = payload.organic.map((result) => ({
    title: result.title,
    url: result.link,
    snippet: result.snippet,
    ...(result.date === undefined ? {} : { date: result.date }),
  }));
  const urls = results.map((result) => result.url);
  return {
    text: JSON.stringify(results),
    urls,
    seenUrls: urls,
    provider: "serper",
  };
}
