import { describe, expect, it } from "vitest";
import { markdownLinks } from "./browser";

describe("markdownLinks", () => {
  it("extracts unique HTTP links and removes fragments", () => {
    const links = markdownLinks(
      "[About](https://example.com/about#team) [Again](https://example.com/about) [Mail](mailto:test@example.com)",
    );

    expect(links).toEqual(["https://example.com/about"]);
  });
});
