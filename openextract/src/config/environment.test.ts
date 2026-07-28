import { describe, expect, test } from "bun:test";
import { parseEnvironment } from "./environment.ts";

describe("parseEnvironment", () => {
  test("applies concurrency defaults", () => {
    expect(parseEnvironment({})).toEqual({
      PORT: 8081,
      OPENEXTRACT_MAX_CONCURRENCY: 20,
      OPENEXTRACT_BROWSER_CONCURRENCY: 4,
      OPENEXTRACT_MAX_WAITING: 100,
    });
  });

  test("parses integer strings", () => {
    expect(
      parseEnvironment({
        PORT: "9000",
        OPENEXTRACT_MAX_CONCURRENCY: "12",
        OPENEXTRACT_BROWSER_CONCURRENCY: "3",
        OPENEXTRACT_MAX_WAITING: "50",
      }),
    ).toEqual({
      PORT: 9000,
      OPENEXTRACT_MAX_CONCURRENCY: 12,
      OPENEXTRACT_BROWSER_CONCURRENCY: 3,
      OPENEXTRACT_MAX_WAITING: 50,
    });
  });

  test("rejects invalid concurrency relationships", () => {
    expect(() =>
      parseEnvironment({
        OPENEXTRACT_MAX_CONCURRENCY: "2",
        OPENEXTRACT_BROWSER_CONCURRENCY: "3",
      }),
    ).toThrow();
  });

  test("rejects fractional values", () => {
    expect(() => parseEnvironment({ OPENEXTRACT_MAX_CONCURRENCY: "2.5" })).toThrow();
  });
});
