import { queueMessageSchema, startRunMessageSchema, type StartRunMessage } from "./contracts";
import type { Env } from "./env";
import { executeBrowserTask } from "./browser";
import { markDispatched, pendingRunMessages } from "./storage";

export async function dispatchRuns(env: Env, messages: readonly StartRunMessage[]): Promise<void> {
  for (const chunk of chunks(messages, 100)) {
    await env.RUN_QUEUE.sendBatch(chunk.map((body) => ({ body })));
    await markDispatched(env.DB, chunk);
  }
}

export async function reconcileDispatch(env: Env): Promise<void> {
  await dispatchRuns(env, await pendingRunMessages(env.DB));
}

export async function consumeQueue(batch: MessageBatch<unknown>, env: Env): Promise<void> {
  const parsed = batch.messages.map((message) => ({
    message,
    body: queueMessageSchema.safeParse(message.body),
  }));
  const starts = parsed.flatMap((item) =>
    item.body.success && item.body.data.type === "start-run"
      ? [{ message: item.message, body: startRunMessageSchema.parse(item.body.data) }]
      : [],
  );
  if (starts.length > 0) {
    try {
      await env.RESEARCH_WORKFLOW.createBatch(
        starts.map(({ body }) => ({
          id: body.workflowId,
          params: body.params,
        })),
      );
      for (const { message } of starts) message.ack();
    } catch {
      for (const { message } of starts) message.retry();
    }
  }
  await Promise.all(
    parsed.map(async ({ message, body }) => {
      if (!body.success) {
        message.retry();
        return;
      }
      if (body.data.type !== "browser-fetch") return;
      try {
        await executeBrowserTask(env, body.data);
        message.ack();
      } catch {
        message.retry();
      }
    }),
  );
}

function chunks<T>(values: readonly T[], size: number): T[][] {
  const result: T[][] = [];
  for (let index = 0; index < values.length; index += size) {
    result.push(values.slice(index, index + size));
  }
  return result;
}
