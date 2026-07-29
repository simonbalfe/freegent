import { describe, expect, it } from "vitest";
import { compileOutputSchema } from "./schema";

describe("compileOutputSchema", () => {
  it("compiles and validates a standard JSON Schema", () => {
    const schema = compileOutputSchema({
      type: "object",
      properties: {
        name: { type: "string" },
        score: { type: ["number", "null"] },
        website: { type: "string", format: "uri" },
      },
      required: ["name", "score", "website"],
      additionalProperties: false,
    });

    expect(schema.validate({ name: "Acme", score: 9, website: "https://example.com" })).toBe(true);
    expect(schema.validate({ name: "Acme", score: null, website: "https://example.com" })).toBe(true);
    expect(schema.validate({ name: "Acme", score: "high", website: "not a URL" })).toBe(false);
  });

  it("rejects remote references", () => {
    expect(() =>
      compileOutputSchema({
        $ref: "https://example.com/schema.json",
      }),
    ).toThrow("Remote schema reference is not supported");
  });
});
