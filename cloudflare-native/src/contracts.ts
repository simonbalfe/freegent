import { z } from "zod";

export type JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };
export type JsonObject = { [key: string]: JsonValue };

export const jsonValueSchema: z.ZodType<JsonValue> = z.lazy(() =>
  z.union([
    z.string(),
    z.number(),
    z.boolean(),
    z.null(),
    z.array(jsonValueSchema),
    z.record(z.string(), jsonValueSchema),
  ]),
);

export const jsonObjectSchema: z.ZodType<JsonObject> = z.record(z.string(), jsonValueSchema);

export const jobRequestSchema = z.object({
  name: z.string().trim().min(1).default("Research job"),
  instructions: z.string().default(""),
  template: z.string().min(1),
  schema: jsonObjectSchema,
  rows: z.array(jsonObjectSchema).min(1).max(1_000),
  model: z.string().trim().min(1).default("google/gemini-3.1-flash-lite"),
  maxSteps: z.number().int().min(1).max(20).default(5),
  maxOutputTokens: z.number().int().min(128).max(16_384).default(1_500),
});

export const storedJobRequestSchema = jobRequestSchema.omit({ rows: true });

export const workflowParamsSchema = z.object({
  jobId: z.string().uuid(),
  runId: z.string().uuid(),
  rowIndex: z.number().int().nonnegative(),
});

export const startRunMessageSchema = z.object({
  type: z.literal("start-run"),
  workflowId: z.string().min(1).max(100),
  params: workflowParamsSchema,
});

export const browserTaskMessageSchema = z.object({
  type: z.literal("browser-fetch"),
  workflowId: z.string().min(1).max(100),
  eventType: z.string().regex(/^[a-zA-Z0-9_][a-zA-Z0-9-_]*$/).max(100),
  requestId: z.string().min(1),
  url: z.string().url(),
  resultKey: z.string().min(1),
});

export const queueMessageSchema = z.discriminatedUnion("type", [
  startRunMessageSchema,
  browserTaskMessageSchema,
]);

export const browserArtifactSchema = z.object({
  url: z.string().url(),
  text: z.string(),
  links: z.array(z.string().url()),
  provider: z.literal("cloudflare-browser-run"),
  browserMs: z.number().int().nonnegative().nullable(),
});

export const browserEventPayloadSchema = z.object({
  resultKey: z.string().min(1),
});

export const toolCallSchema = z.object({
  id: z.string().min(1),
  name: z.string().min(1),
  input: jsonObjectSchema,
});

export const tokenUsageSchema = z.object({
  input: z.number().int().nonnegative(),
  output: z.number().int().nonnegative(),
  costUsd: z.number().nonnegative().nullable(),
});

export const modelDecisionSchema = z.object({
  toolCalls: z.array(toolCallSchema),
  final: jsonObjectSchema.nullable(),
  reasoning: z.string(),
  usage: tokenUsageSchema,
});

export type JobRequest = z.infer<typeof jobRequestSchema>;
export type StoredJobRequest = z.infer<typeof storedJobRequestSchema>;
export type WorkflowParams = z.infer<typeof workflowParamsSchema>;
export type StartRunMessage = z.infer<typeof startRunMessageSchema>;
export type BrowserTaskMessage = z.infer<typeof browserTaskMessageSchema>;
export type QueueMessage = z.infer<typeof queueMessageSchema>;
export type BrowserArtifact = z.infer<typeof browserArtifactSchema>;
export type ToolCall = z.infer<typeof toolCallSchema>;
export type TokenUsage = z.infer<typeof tokenUsageSchema>;
export type ModelDecision = z.infer<typeof modelDecisionSchema>;

export type AgentMessage =
  | { readonly role: "system" | "user"; readonly content: string }
  | { readonly role: "assistant"; readonly toolCalls: readonly ToolCall[] }
  | { readonly role: "tool"; readonly toolCall: ToolCall; readonly content: string };

export type Evidence = {
  readonly tool: string;
  readonly text: string;
  readonly urls: readonly string[];
  readonly provider: string;
};

export type ToolResult = {
  readonly text: string;
  readonly urls: readonly string[];
  readonly seenUrls: readonly string[];
  readonly provider: string;
};

export type RunArtifact = {
  readonly answer: Readonly<JsonObject>;
  readonly reasoning: string;
  readonly sources: readonly string[];
  readonly evidence: readonly Evidence[];
  readonly tokens: TokenUsage;
};
