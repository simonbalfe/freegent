import { Validator, type OutputUnit, type Schema } from "@cfworker/json-schema";
import type { JsonObject } from "./contracts";

type JsonSchema = Readonly<JsonObject>;

export type CompiledOutputSchema = {
  readonly document: JsonSchema;
  readonly validate: (value: unknown) => boolean;
  readonly errors: () => string;
};

export function compileOutputSchema(input: Readonly<JsonObject>): CompiledOutputSchema {
  rejectRemoteReferences(input);
  const document = structuredClone(input);
  const validator = new Validator(document as Schema, "2020-12", false);
  let lastErrors: readonly OutputUnit[] = [];
  return {
    document,
    validate: (value: unknown): boolean => {
      const result = validator.validate(value);
      lastErrors = result.errors;
      return result.valid;
    },
    errors: (): string =>
      lastErrors.map((error) => `${error.instanceLocation || "/"} ${error.error}`).join("; "),
  };
}

function rejectRemoteReferences(value: unknown): void {
  if (Array.isArray(value)) {
    for (const child of value) rejectRemoteReferences(child);
    return;
  }
  if (!isRecord(value)) return;
  for (const [key, child] of Object.entries(value)) {
    if (key === "$ref" && typeof child === "string" && /^https?:\/\//.test(child)) {
      throw new Error(`Remote schema reference is not supported: ${child}`);
    }
    rejectRemoteReferences(child);
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
