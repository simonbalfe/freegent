import { z } from "zod";
import {
  jsonObjectSchema,
  storedJobRequestSchema,
  type JobRequest,
  type RunArtifact,
  type StartRunMessage,
  type StoredJobRequest,
  type WorkflowParams,
} from "./contracts";
import type { Env } from "./env";

const runContextRowSchema = z.object({
  request_json: z.string(),
  input_json: z.string(),
  status: z.enum(["queued", "running", "completed", "failed", "skipped"]),
});

const runContextSchema = z.object({
  request: storedJobRequestSchema,
  input: jsonObjectSchema,
  status: runContextRowSchema.shape.status,
});

const pendingRunSchema = z.object({
  id: z.string().uuid(),
  job_id: z.string().uuid(),
  row_index: z.number().int().nonnegative(),
  workflow_id: z.string(),
});

const jobRowSchema = z.object({
  id: z.string().uuid(),
  name: z.string(),
  status: z.enum(["queued", "running", "completed", "partial", "failed"]),
  total: z.number().int().nonnegative(),
  completed: z.number().int().nonnegative(),
  failed: z.number().int().nonnegative(),
  created_at: z.string(),
  updated_at: z.string(),
});

const runRowSchema = z.object({
  id: z.string().uuid(),
  row_index: z.number().int().nonnegative(),
  status: z.enum(["queued", "running", "completed", "failed", "skipped"]),
  result_json: z.string().nullable(),
  error: z.string().nullable(),
  started_at: z.string().nullable(),
  finished_at: z.string().nullable(),
});

export type RunContext = {
  readonly request: StoredJobRequest;
  readonly input: Readonly<import("./contracts").JsonObject>;
  readonly status: z.infer<typeof runContextRowSchema>["status"];
};

export class JobNotFoundError extends Error {
  override readonly name = "JobNotFoundError";
}

export function parseRunContext(value: string): RunContext {
  const raw: unknown = JSON.parse(value);
  return runContextSchema.parse(raw);
}

export async function createJob(env: Env, request: JobRequest): Promise<{ jobId: string; messages: StartRunMessage[] }> {
  const jobId = crypto.randomUUID();
  const now = new Date().toISOString();
  const storedRequest = storedJobRequestSchema.parse(request);
  const runRows: {
    readonly id: string;
    readonly jobId: string;
    readonly rowIndex: number;
    readonly workflowId: string;
    readonly inputJson: string;
    readonly createdAt: string;
  }[] = [];
  const statements: D1PreparedStatement[] = [
    env.DB.prepare(
      "INSERT INTO jobs (id, name, status, request_json, total, created_at, updated_at) VALUES (?, ?, 'queued', ?, ?, ?, ?)",
    ).bind(jobId, request.name, JSON.stringify(storedRequest), request.rows.length, now, now),
  ];
  const messages = request.rows.map((row, rowIndex) => {
    const runId = crypto.randomUUID();
    const workflowId = workflowIdFor(jobId, rowIndex);
    runRows.push({
      id: runId,
      jobId,
      rowIndex,
      workflowId,
      inputJson: JSON.stringify(row),
      createdAt: now,
    });
    return {
      type: "start-run",
      workflowId,
      params: { jobId, runId, rowIndex },
    } satisfies StartRunMessage;
  });
  statements.push(
    env.DB.prepare(
      `INSERT INTO runs (id, job_id, row_index, workflow_id, status, input_json, created_at)
       SELECT
         json_extract(value, '$.id'),
         json_extract(value, '$.jobId'),
         json_extract(value, '$.rowIndex'),
         json_extract(value, '$.workflowId'),
         'queued',
         json_extract(value, '$.inputJson'),
         json_extract(value, '$.createdAt')
       FROM json_each(?)`,
    ).bind(JSON.stringify(runRows)),
  );
  statements.push(
    env.DB.prepare(
      "INSERT INTO job_events (job_id, kind, message, created_at) VALUES (?, 'job_queued', ?, ?)",
    ).bind(jobId, `${request.rows.length} runs queued`, now),
  );
  await env.DB.batch(statements);
  return { jobId, messages };
}

export async function markDispatched(db: D1Database, messages: readonly StartRunMessage[]): Promise<void> {
  if (messages.length === 0) return;
  const now = new Date().toISOString();
  await db
    .prepare(
      "UPDATE runs SET dispatched_at = ? WHERE id IN (SELECT value FROM json_each(?)) AND dispatched_at IS NULL",
    )
    .bind(now, JSON.stringify(messages.map((message) => message.params.runId)))
    .run();
}

export async function pendingRunMessages(db: D1Database, limit = 500): Promise<StartRunMessage[]> {
  const response = await db
    .prepare(
      "SELECT id, job_id, row_index, workflow_id FROM runs WHERE dispatched_at IS NULL AND status = 'queued' ORDER BY created_at LIMIT ?",
    )
    .bind(limit)
    .all();
  return z.array(pendingRunSchema).parse(response.results).map((row) => ({
    type: "start-run",
    workflowId: row.workflow_id,
    params: {
      jobId: row.job_id,
      runId: row.id,
      rowIndex: row.row_index,
    },
  }));
}

export async function loadRunContext(db: D1Database, params: WorkflowParams): Promise<RunContext> {
  const raw = await db
    .prepare(
      "SELECT jobs.request_json, runs.input_json, runs.status FROM runs JOIN jobs ON jobs.id = runs.job_id WHERE runs.id = ? AND runs.job_id = ? AND runs.row_index = ?",
    )
    .bind(params.runId, params.jobId, params.rowIndex)
    .first();
  const row = runContextRowSchema.parse(raw);
  const rawRequest: unknown = JSON.parse(row.request_json);
  const rawInput: unknown = JSON.parse(row.input_json);
  return {
    request: storedJobRequestSchema.parse(rawRequest),
    input: jsonObjectSchema.parse(rawInput),
    status: row.status,
  };
}

export async function markRunRunning(db: D1Database, params: WorkflowParams): Promise<void> {
  const now = new Date().toISOString();
  await db.batch([
    db
      .prepare("UPDATE runs SET status = 'running', started_at = COALESCE(started_at, ?) WHERE id = ? AND status = 'queued'")
      .bind(now, params.runId),
    db.prepare("UPDATE jobs SET status = 'running', updated_at = ? WHERE id = ? AND status = 'queued'").bind(
      now,
      params.jobId,
    ),
    db
      .prepare("INSERT INTO job_events (job_id, run_id, kind, message, created_at) VALUES (?, ?, 'run_started', ?, ?)")
      .bind(params.jobId, params.runId, `Row ${params.rowIndex + 1} started`, now),
  ]);
}

export async function completeRun(env: Env, params: WorkflowParams, artifact: RunArtifact): Promise<void> {
  const key = `jobs/${params.jobId}/runs/${params.rowIndex}.json`;
  await env.ARTIFACTS.put(key, JSON.stringify(artifact), {
    httpMetadata: { contentType: "application/json" },
  });
  const now = new Date().toISOString();
  const summary = {
    answer: artifact.answer,
    reasoning: artifact.reasoning,
    sources: artifact.sources,
    tokens: artifact.tokens,
  };
  await env.DB.batch([
    env.DB.prepare(
      "UPDATE runs SET status = 'completed', result_json = ?, artifact_key = ?, error = NULL, finished_at = ? WHERE id = ?",
    ).bind(JSON.stringify(summary), key, now, params.runId),
    env.DB.prepare(
      "INSERT INTO job_events (job_id, run_id, kind, message, created_at) VALUES (?, ?, 'run_completed', ?, ?)",
    ).bind(params.jobId, params.runId, `Row ${params.rowIndex + 1} completed`, now),
  ]);
  await refreshJob(env.DB, params.jobId);
}

export async function failRun(env: Env, params: WorkflowParams, error: string): Promise<void> {
  const now = new Date().toISOString();
  await env.DB.batch([
    env.DB.prepare("UPDATE runs SET status = 'failed', error = ?, finished_at = ? WHERE id = ?").bind(
      error,
      now,
      params.runId,
    ),
    env.DB.prepare(
      "INSERT INTO job_events (job_id, run_id, kind, message, created_at) VALUES (?, ?, 'run_failed', ?, ?)",
    ).bind(params.jobId, params.runId, error, now),
  ]);
  await refreshJob(env.DB, params.jobId);
}

export async function getJob(db: D1Database, jobId: string): Promise<unknown> {
  const rawJob = await db
    .prepare(
      "SELECT id, name, status, total, completed, failed, created_at, updated_at FROM jobs WHERE id = ?",
    )
    .bind(jobId)
    .first();
  if (rawJob === null) throw new JobNotFoundError(`Job ${jobId} was not found`);
  const job = jobRowSchema.parse(rawJob);
  const response = await db
    .prepare(
      "SELECT id, row_index, status, result_json, error, started_at, finished_at FROM runs WHERE job_id = ? ORDER BY row_index",
    )
    .bind(jobId)
    .all();
  const runs = z.array(runRowSchema).parse(response.results).map((run) => ({
    id: run.id,
    rowIndex: run.row_index,
    status: run.status,
    result: parseNullableJson(run.result_json),
    error: run.error,
    startedAt: run.started_at,
    finishedAt: run.finished_at,
  }));
  return {
    id: job.id,
    name: job.name,
    status: job.status,
    total: job.total,
    completed: job.completed,
    failed: job.failed,
    createdAt: job.created_at,
    updatedAt: job.updated_at,
    runs,
  };
}

export async function listJobs(db: D1Database, limit: number): Promise<unknown> {
  const response = await db
    .prepare(
      "SELECT id, name, status, total, completed, failed, created_at, updated_at FROM jobs ORDER BY created_at DESC LIMIT ?",
    )
    .bind(limit)
    .all();
  const jobs = z.array(jobRowSchema).parse(response.results).map((job) => ({
    id: job.id,
    name: job.name,
    status: job.status,
    total: job.total,
    completed: job.completed,
    failed: job.failed,
    createdAt: job.created_at,
    updatedAt: job.updated_at,
  }));
  return { jobs };
}

export function workflowIdFor(jobId: string, rowIndex: number): string {
  return `run-${jobId}-${rowIndex}`;
}

async function refreshJob(db: D1Database, jobId: string): Promise<void> {
  const now = new Date().toISOString();
  await db
    .prepare(
      `UPDATE jobs
       SET completed = (SELECT COUNT(*) FROM runs WHERE job_id = ? AND status IN ('completed', 'skipped')),
           failed = (SELECT COUNT(*) FROM runs WHERE job_id = ? AND status = 'failed'),
           status = CASE
             WHEN (SELECT COUNT(*) FROM runs WHERE job_id = ? AND status IN ('completed', 'failed', 'skipped')) < total THEN 'running'
             WHEN (SELECT COUNT(*) FROM runs WHERE job_id = ? AND status = 'failed') = 0 THEN 'completed'
             WHEN (SELECT COUNT(*) FROM runs WHERE job_id = ? AND status IN ('completed', 'skipped')) = 0 THEN 'failed'
             ELSE 'partial'
           END,
           updated_at = ?
       WHERE id = ?`,
    )
    .bind(jobId, jobId, jobId, jobId, jobId, now, jobId)
    .run();
}

function parseNullableJson(value: string | null): unknown {
  if (value === null) return null;
  const parsed: unknown = JSON.parse(value);
  return parsed;
}
