import { z } from "zod";
import {
  jsonObjectSchema,
  modelDecisionSchema,
  type AgentMessage,
  type Evidence,
  type JsonObject,
  type ModelDecision,
  type ToolCall,
} from "./contracts";

const responseSchema = z.object({
  choices: z
    .array(
      z.object({
        message: z.object({
          content: z.string().nullable().default(""),
          tool_calls: z
            .array(
              z.object({
                id: z.string(),
                function: z.object({
                  name: z.string(),
                  arguments: z.string(),
                }),
              }),
            )
            .default([]),
        }),
      }),
    )
    .min(1),
  usage: z
    .object({
      prompt_tokens: z.number().int().nonnegative().default(0),
      completion_tokens: z.number().int().nonnegative().default(0),
      cost: z.number().nonnegative().nullable().optional(),
    })
    .default({ prompt_tokens: 0, completion_tokens: 0 }),
});

type ModelConfig = {
  readonly apiKey: string;
  readonly model: string;
  readonly maxOutputTokens: number;
  readonly outputSchema: Readonly<JsonObject>;
};

export async function decide(
  config: ModelConfig,
  messages: readonly AgentMessage[],
): Promise<ModelDecision> {
  return chat(config, messages, true);
}

export async function finalize(
  config: ModelConfig,
  system: string,
  task: string,
  evidence: readonly Evidence[],
): Promise<ModelDecision> {
  const messages: AgentMessage[] = [
    { role: "system", content: system },
    { role: "user", content: `${task}\n\nEvidence:\n${JSON.stringify(evidence)}` },
  ];
  return chat(config, messages, false);
}

async function chat(
  config: ModelConfig,
  messages: readonly AgentMessage[],
  enableTools: boolean,
): Promise<ModelDecision> {
  if (config.apiKey.length === 0) throw new Error("OPENROUTER_API_KEY is not configured");
  const body: Record<string, unknown> = {
    model: config.model,
    messages: messages.map(openRouterMessage),
    temperature: 0,
    max_tokens: config.maxOutputTokens,
    provider: {
      require_parameters: true,
    },
  };
  if (enableTools) {
    body["tools"] = toolDefinitions(config.outputSchema);
    body["tool_choice"] = "required";
    body["parallel_tool_calls"] = true;
  } else {
    body["response_format"] = {
      type: "json_schema",
      json_schema: {
        name: "freegent_answer",
        strict: true,
        schema: answerEnvelopeSchema(config.outputSchema),
      },
    };
  }
  const response = await fetch("https://openrouter.ai/api/v1/chat/completions", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${config.apiKey}`,
      "Content-Type": "application/json",
      "HTTP-Referer": "https://github.com/simonbalfe/freegent",
    },
    body: JSON.stringify(body),
  });
  if (!response.ok) throw new Error(`OpenRouter returned ${response.status}: ${await response.text()}`);
  const payload = responseSchema.parse(await response.json());
  const message = payload.choices[0]?.message;
  if (message === undefined) throw new Error("OpenRouter returned no message");
  const toolCalls = message.tool_calls.map(parseToolCall);
  const content = message.content ?? "";
  const final = toolCalls.length === 0 ? parseFinal(content) : null;
  const decision = {
    toolCalls,
    final: final?.answer ?? null,
    reasoning: final?.reasoning ?? "",
    usage: {
      input: payload.usage.prompt_tokens,
      output: payload.usage.completion_tokens,
      costUsd: payload.usage.cost ?? null,
    },
  };
  return modelDecisionSchema.parse(decision);
}

function openRouterMessage(message: AgentMessage): Readonly<Record<string, unknown>> {
  switch (message.role) {
    case "system":
    case "user":
      return { role: message.role, content: message.content };
    case "assistant":
      return {
        role: "assistant",
        tool_calls: message.toolCalls.map((call) => ({
          id: call.id,
          type: "function",
          function: { name: call.name, arguments: JSON.stringify(call.input) },
        })),
      };
    case "tool":
      return {
        role: "tool",
        tool_call_id: message.toolCall.id,
        name: message.toolCall.name,
        content: message.content,
      };
  }
}

function parseToolCall(raw: z.infer<typeof responseSchema>["choices"][number]["message"]["tool_calls"][number]): ToolCall {
  const input: unknown = JSON.parse(raw.function.arguments);
  return {
    id: raw.id,
    name: raw.function.name,
      input: jsonObjectSchema.parse(input),
  };
}

function parseFinal(content: string): { answer: Readonly<JsonObject>; reasoning: string } | null {
  try {
    const cleaned = content.trim().replace(/^```json\s*/, "").replace(/^```\s*/, "").replace(/\s*```$/, "");
    const raw: unknown = JSON.parse(cleaned);
    return z
      .object({
        answer: jsonObjectSchema,
        reasoning: z.string().default(""),
      })
      .parse(raw);
  } catch {
    return null;
  }
}

function toolDefinitions(outputSchema: Readonly<JsonObject>): readonly unknown[] {
  return [
    {
      type: "function",
      function: {
        name: "web_search",
        description: "Search the current web. Returns titles, URLs and snippets.",
        strict: true,
        parameters: {
          type: "object",
          properties: { query: { type: "string" } },
          required: ["query"],
          additionalProperties: false,
        },
      },
    },
    {
      type: "function",
      function: {
        name: "fetch_page",
        description: "Render and extract a verified web page as Markdown.",
        strict: true,
        parameters: {
          type: "object",
          properties: { url: { type: "string", format: "uri" } },
          required: ["url"],
          additionalProperties: false,
        },
      },
    },
    {
      type: "function",
      function: {
        name: "submit_answer",
        description: "Submit the final evidence-backed answer. Call this by itself.",
        strict: true,
        parameters: answerEnvelopeSchema(outputSchema),
      },
    },
  ];
}

function answerEnvelopeSchema(outputSchema: Readonly<JsonObject>): Readonly<Record<string, unknown>> {
  return {
    type: "object",
    properties: {
      answer: outputSchema,
      reasoning: { type: "string" },
    },
    required: ["answer", "reasoning"],
    additionalProperties: false,
  };
}
