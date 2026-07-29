import { WorkflowEntrypoint, type WorkflowEvent, type WorkflowStep } from "cloudflare:workers";
import { z } from "zod";
import {
  browserArtifactSchema,
  browserEventPayloadSchema,
  jsonObjectSchema,
  modelDecisionSchema,
  workflowParamsSchema,
  type AgentMessage,
  type Evidence,
  type JsonObject,
  type RunArtifact,
  type TokenUsage,
  type ToolCall,
  type ToolResult,
  type WorkflowParams,
} from "./contracts";
import type { Env } from "./env";
import { decide, finalize } from "./openrouter";
import { finalizerInstructions, renderTemplate, researchInstructions } from "./prompts";
import { addUrls, initialUrls, permitsUrl } from "./provenance";
import { compileOutputSchema, type CompiledOutputSchema } from "./schema";
import { searchWeb } from "./search";
import { completeRun, failRun, loadRunContext, markRunRunning, parseRunContext } from "./storage";

type ToolOutcome =
  | { readonly kind: "success"; readonly call: ToolCall; readonly result: ToolResult }
  | { readonly kind: "error"; readonly call: ToolCall; readonly error: string };

const emptyUsage: TokenUsage = { input: 0, output: 0, costUsd: 0 };

export class ResearchWorkflow extends WorkflowEntrypoint<Env, WorkflowParams> {
  override async run(event: WorkflowEvent<WorkflowParams>, step: WorkflowStep): Promise<unknown> {
    const params = workflowParamsSchema.parse(event.payload);
    try {
      const contextJson = await step.do("load run context", async () =>
        JSON.stringify(await loadRunContext(this.env.DB, params)),
      );
      const context = parseRunContext(contextJson);
      if (context.status === "completed" || context.status === "skipped") return { status: context.status };
      await step.do("mark run running", async () => markRunRunning(this.env.DB, params));
      const schema = compileOutputSchema(context.request.schema);
      const task = renderTemplate(context.request.template, context.input);
      const messages: AgentMessage[] = [
        { role: "system", content: researchInstructions(context.request.instructions, schema.document) },
        { role: "user", content: task },
      ];
      const evidence: Evidence[] = [];
      const allowedUrls = initialUrls(context.input);
      const sources = new Set<string>();
      let tokens = emptyUsage;

      for (let round = 0; round < context.request.maxSteps; round++) {
        const decisionJson = await step.do(
          `model decision ${round}`,
          { retries: { limit: 4, delay: "5 seconds", backoff: "exponential" }, timeout: "3 minutes" },
          async () =>
            JSON.stringify(
              await decide(
                {
                  apiKey: this.env.OPENROUTER_API_KEY,
                  model: context.request.model,
                  maxOutputTokens: context.request.maxOutputTokens,
                  outputSchema: schema.document,
                },
                messages,
              ),
            ),
        );
        const rawDecision: unknown = JSON.parse(decisionJson);
        const decision = modelDecisionSchema.parse(rawDecision);
        tokens = addUsage(tokens, decision.usage);
        if (decision.final !== null && schema.validate(decision.final)) {
          const artifact = createArtifact(decision.final, decision.reasoning, sources, evidence, tokens);
          await step.do("save direct answer", async () => completeRun(this.env, params, artifact));
          return { status: "completed" };
        }
        if (decision.toolCalls.length === 0) break;
        messages.push({ role: "assistant", toolCalls: decision.toolCalls });
        const submitted = submittedAnswer(decision.toolCalls, evidence, schema);
        if (submitted !== null) {
          if (submitted.kind === "answer") {
            const artifact = createArtifact(submitted.answer, submitted.reasoning, sources, evidence, tokens);
            await step.do("save submitted answer", async () => completeRun(this.env, params, artifact));
            return { status: "completed" };
          }
          messages.push({
            role: "tool",
            toolCall: submitted.call,
            content: submitted.error,
          });
          continue;
        }
        const outcomes = await Promise.all(
          decision.toolCalls.map((call, index) =>
            this.runTool(step, event.instanceId, round, index, call, allowedUrls),
          ),
        );
        for (const outcome of outcomes) {
          if (outcome.kind === "error") {
            messages.push({ role: "tool", toolCall: outcome.call, content: outcome.error });
            continue;
          }
          addUrls(allowedUrls, outcome.result.seenUrls);
          addUrls(sources, outcome.result.urls);
          evidence.push({
            tool: outcome.call.name,
            text: outcome.result.text,
            urls: outcome.result.urls,
            provider: outcome.result.provider,
          });
          messages.push({ role: "tool", toolCall: outcome.call, content: outcome.result.text });
        }
      }

      const finalDecisionJson = await step.do(
        "finalize answer",
        { retries: { limit: 4, delay: "5 seconds", backoff: "exponential" }, timeout: "3 minutes" },
        async () =>
          JSON.stringify(
            await finalize(
              {
                apiKey: this.env.OPENROUTER_API_KEY,
                model: context.request.model,
                maxOutputTokens: context.request.maxOutputTokens,
                outputSchema: schema.document,
              },
              finalizerInstructions(context.request.instructions, schema.document),
              task,
              evidence,
            ),
          ),
      );
      const rawFinalDecision: unknown = JSON.parse(finalDecisionJson);
      const finalDecision = modelDecisionSchema.parse(rawFinalDecision);
      tokens = addUsage(tokens, finalDecision.usage);
      if (finalDecision.final === null || !schema.validate(finalDecision.final)) {
        throw new Error(`Final answer failed schema validation: ${schema.errors()}`);
      }
      const artifact = createArtifact(finalDecision.final, finalDecision.reasoning, sources, evidence, tokens);
      await step.do("save finalized answer", async () => completeRun(this.env, params, artifact));
      return { status: "completed" };
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      await step.do("record run failure", async () => failRun(this.env, params, message));
      return { status: "failed", error: message };
    }
  }

  private async runTool(
    step: WorkflowStep,
    workflowId: string,
    round: number,
    index: number,
    call: ToolCall,
    allowedUrls: ReadonlySet<string>,
  ): Promise<ToolOutcome> {
    try {
      switch (call.name) {
        case "web_search": {
          const input = z.object({ query: z.string().trim().min(1) }).parse(call.input);
          const result = await step.do(
            `search ${round}-${index}`,
            { retries: { limit: 3, delay: "5 seconds", backoff: "exponential" }, timeout: "1 minute" },
            async () => searchWeb(this.env.SERPER_API_KEY, input.query),
          );
          return { kind: "success", call, result };
        }
        case "fetch_page": {
          const input = z.object({ url: z.string().url() }).parse(call.input);
          if (!permitsUrl(allowedUrls, input.url)) {
            return {
              kind: "error",
              call,
              error: `Tool call rejected: ${input.url} was not present in the input or gathered evidence.`,
            };
          }
          const eventType = `browser_${round}_${index}`;
          const resultKey = `browser/${workflowId}/${round}-${index}.json`;
          await step.do(`queue browser ${round}-${index}`, async () => {
            await this.env.BROWSER_QUEUE.send({
              type: "browser-fetch",
              workflowId,
              eventType,
              requestId: `${workflowId}-${round}-${index}`,
              url: input.url,
              resultKey,
            });
          });
          const browserEvent = await step.waitForEvent(`wait for browser ${round}-${index}`, {
            type: eventType,
            timeout: "10 minutes",
          });
          const eventPayload = browserEventPayloadSchema.parse(browserEvent.payload);
          const artifact = await step.do(`read browser ${round}-${index}`, async () => {
            const object = await this.env.ARTIFACTS.get(eventPayload.resultKey);
            if (object === null) throw new Error("Browser result was not found in R2");
            return browserArtifactSchema.parse(await object.json());
          });
          return {
            kind: "success",
            call,
            result: {
              text: artifact.text,
              urls: [artifact.url],
              seenUrls: [artifact.url, ...artifact.links],
              provider: artifact.provider,
            },
          };
        }
        case "submit_answer":
          return {
            kind: "error",
            call,
            error: "submit_answer must be called by itself.",
          };
        default:
          return {
            kind: "error",
            call,
            error: `Unknown tool ${call.name}.`,
          };
      }
    } catch (error) {
      return {
        kind: "error",
        call,
        error: `Tool call failed: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  }
}

function submittedAnswer(
  calls: readonly ToolCall[],
  evidence: readonly Evidence[],
  schema: CompiledOutputSchema,
):
  | { readonly kind: "answer"; readonly answer: Readonly<JsonObject>; readonly reasoning: string }
  | { readonly kind: "error"; readonly call: ToolCall; readonly error: string }
  | null {
  const submission = calls.find((call) => call.name === "submit_answer");
  if (submission === undefined) return null;
  if (calls.length !== 1) {
    return {
      kind: "error",
      call: submission,
      error: "submit_answer must be called without research tools in the same decision.",
    };
  }
  if (evidence.length === 0) {
    return {
      kind: "error",
      call: submission,
      error: "Gather current evidence before submitting an answer.",
    };
  }
  const parsed = z
    .object({
      answer: jsonObjectSchema,
      reasoning: z.string().default(""),
    })
    .safeParse(submission.input);
  if (!parsed.success) {
    return { kind: "error", call: submission, error: parsed.error.message };
  }
  if (!schema.validate(parsed.data.answer)) {
    return { kind: "error", call: submission, error: schema.errors() };
  }
  return {
    kind: "answer",
    answer: parsed.data.answer,
    reasoning: parsed.data.reasoning,
  };
}

function createArtifact(
  answer: Readonly<JsonObject>,
  reasoning: string,
  sources: ReadonlySet<string>,
  evidence: readonly Evidence[],
  tokens: TokenUsage,
): RunArtifact {
  return {
    answer,
    reasoning,
    sources: [...sources],
    evidence,
    tokens,
  };
}

function addUsage(current: TokenUsage, additional: TokenUsage): TokenUsage {
  return {
    input: current.input + additional.input,
    output: current.output + additional.output,
    costUsd:
      current.costUsd === null || additional.costUsd === null
        ? null
        : current.costUsd + additional.costUsd,
  };
}
