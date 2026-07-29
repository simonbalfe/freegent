import type { JsonObject } from "./contracts";

export function initialUrls(input: Readonly<JsonObject>): Set<string> {
  const urls = new Set<string>();
  collectUrls(input, urls);
  return urls;
}

export function normalizedHttpUrl(value: string): string | null {
  try {
    const url = new URL(value);
    if (url.protocol !== "http:" && url.protocol !== "https:") return null;
    url.hash = "";
    return url.href;
  } catch {
    return null;
  }
}

export function permitsUrl(allowed: ReadonlySet<string>, value: string): boolean {
  const normalized = normalizedHttpUrl(value);
  return normalized !== null && allowed.has(normalized);
}

export function addUrls(target: Set<string>, values: readonly string[]): void {
  for (const value of values) {
    const normalized = normalizedHttpUrl(value);
    if (normalized !== null) target.add(normalized);
  }
}

function collectUrls(value: unknown, target: Set<string>): void {
  if (typeof value === "string") {
    const normalized = normalizedHttpUrl(value);
    if (normalized !== null) target.add(normalized);
    return;
  }
  if (Array.isArray(value)) {
    for (const child of value) collectUrls(child, target);
    return;
  }
  if (typeof value !== "object" || value === null) return;
  for (const child of Object.values(value)) collectUrls(child, target);
}
