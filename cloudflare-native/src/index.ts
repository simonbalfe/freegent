import { z, ZodError } from "zod";
import { jobRequestSchema } from "./contracts";
import { dashboardResponse } from "./dashboard";
import { consumeQueue, dispatchRuns, reconcileDispatch } from "./dispatch";
import type { Env } from "./env";
import { compileOutputSchema } from "./schema";
import { createJob, getJob, JobNotFoundError, listJobs } from "./storage";
import { ResearchWorkflow } from "./workflow";

const jobPathSchema = z.string().uuid();
const listLimitSchema = z.coerce.number().int().min(1).max(100).default(50);

export { ResearchWorkflow };

export const worker = {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (request.method === "GET" && url.pathname === "/health") {
      return Response.json({ ok: true });
    }
    if (request.method === "GET" && (url.pathname === "/" || url.pathname === "/dashboard")) {
      return dashboardResponse(env.PUBLIC_DASHBOARD === "true");
    }
    if (request.method === "GET" && url.pathname === "/favicon.ico") {
      return new Response(null, { status: 204 });
    }
    if (!isPublicDashboardRead(request, url, env)) {
      const unauthorized = authorize(request, env);
      if (unauthorized !== null) return unauthorized;
    }
    try {
      if (request.method === "GET" && url.pathname === "/jobs") {
        const limit = listLimitSchema.parse(url.searchParams.get("limit") ?? undefined);
        return Response.json(await listJobs(env.DB, limit));
      }
      if (request.method === "POST" && url.pathname === "/jobs") {
        const body: unknown = await request.json();
        const job = jobRequestSchema.parse(body);
        compileOutputSchema(job.schema);
        const created = await createJob(env, job);
        let dispatchPending = false;
        try {
          await dispatchRuns(env, created.messages);
        } catch {
          dispatchPending = true;
        }
        return Response.json(
          {
            jobId: created.jobId,
            status: "queued",
            runs: created.messages.length,
            dispatchPending,
          },
          { status: 202 },
        );
      }
      const match = url.pathname.match(/^\/jobs\/([^/]+)$/);
      if (request.method === "GET" && match !== null) {
        const jobId = jobPathSchema.parse(match[1]);
        return Response.json(await getJob(env.DB, jobId));
      }
      return Response.json({ error: "Not found" }, { status: 404 });
    } catch (error) {
      if (error instanceof JobNotFoundError) {
        return Response.json({ error: error.message }, { status: 404 });
      }
      if (error instanceof ZodError) {
        return Response.json({ error: "Invalid request", issues: error.issues }, { status: 400 });
      }
      return Response.json(
        { error: error instanceof Error ? error.message : String(error) },
        { status: 500 },
      );
    }
  },

  async queue(batch: MessageBatch<unknown>, env: Env): Promise<void> {
    await consumeQueue(batch, env);
  },

  async scheduled(_controller: ScheduledController, env: Env): Promise<void> {
    await reconcileDispatch(env);
  },
} satisfies ExportedHandler<Env>;

export default worker;

function authorize(request: Request, env: Env): Response | null {
  if (!env.API_TOKEN) {
    return Response.json({ error: "API_TOKEN is not configured" }, { status: 503 });
  }
  if (request.headers.get("Authorization") !== `Bearer ${env.API_TOKEN}`) {
    return Response.json({ error: "Unauthorized" }, { status: 401 });
  }
  return null;
}

function isPublicDashboardRead(request: Request, url: URL, env: Env): boolean {
  if (env.PUBLIC_DASHBOARD !== "true" || request.method !== "GET") return false;
  return url.pathname === "/jobs" || /^\/jobs\/[^/]+$/.test(url.pathname);
}
