import Ajv2020 from "ajv/dist/2020";
import addFormats from "ajv-formats";
import type { JsonObject } from "./contracts";

type JsonSchema = Readonly<JsonObject>;

export type CompiledOutputSchema = {
  readonly document: JsonSchema;
  readonly validate: (value: unknown) => boolean;
  readonly errors: () => string;
};

export function compileOutputSchema(input: Readonly<JsonObject>): CompiledOutputSchema {
  rejectRemoteReferences(input);
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  addFormats(ajv);
  const validator = ajv.compile(input);
  return {
    document: input,
    validate: (value: unknown): boolean => validator(value),
    errors: (): string => ajv.errorsText(validator.errors),
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
