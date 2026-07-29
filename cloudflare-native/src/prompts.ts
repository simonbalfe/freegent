import type { JsonObject } from "./contracts";

const researchPrompt = `You are a precise web-research agent enriching one row of a data table.
Use the available tools to find current evidence and answer every requested field.
Request independent tool calls together. Dependent calls must wait for an earlier result.
Never invent a URL. Fetch only URLs present in the input or returned by gathered evidence.
Prefer current first-party sources for facts that change.
If a tool fails, correct the input or use another source.
When the evidence is sufficient, call submit_answer by itself.
Unsupported fields must be null.`;

const finalizerPrompt = `Produce the requested answer immediately from the supplied evidence.
Use only supported facts. Unsupported fields must be null.
Return the answer and a short explanation naming the deciding sources.`;

export function researchInstructions(instructions: string, schema: Readonly<JsonObject>): string {
  return `${researchPrompt}

Task-specific instructions:
${instructions.trim()}

Answer JSON Schema:
${JSON.stringify(schema)}`;
}

export function finalizerInstructions(instructions: string, schema: Readonly<JsonObject>): string {
  return `${finalizerPrompt}

Task-specific instructions:
${instructions.trim()}

Answer JSON Schema:
${JSON.stringify(schema)}`;
}

export function renderTemplate(template: string, input: Readonly<JsonObject>): string {
  return template.replace(/\{\{\s*([^{}]+?)\s*\}\}/g, (_match, rawName: string) => {
    const value = input[rawName.trim()];
    if (value === undefined || value === null) return "";
    return typeof value === "string" ? value : JSON.stringify(value);
  });
}
