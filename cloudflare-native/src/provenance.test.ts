import { describe, expect, it } from "vitest";
import { addUrls, initialUrls, permitsUrl } from "./provenance";

describe("URL provenance", () => {
  it("collects input URLs recursively and removes fragments", () => {
    const allowed = initialUrls({
      website: "https://example.com/#about",
      nested: { source: "https://example.org/path" },
    });

    expect(permitsUrl(allowed, "https://example.com/")).toBe(true);
    expect(permitsUrl(allowed, "https://example.org/path")).toBe(true);
    expect(permitsUrl(allowed, "https://example.net/")).toBe(false);
  });

  it("allows URLs discovered by tools", () => {
    const allowed = initialUrls({});
    addUrls(allowed, ["https://example.com/about"]);

    expect(permitsUrl(allowed, "https://example.com/about")).toBe(true);
  });
});
