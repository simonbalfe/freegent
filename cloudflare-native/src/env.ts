import type { WorkflowParams } from "./contracts";

export type BrowserRunBinding = {
  quickAction(
    action: "markdown",
    options: {
      readonly url: string;
      readonly gotoOptions: { readonly waitUntil: "networkidle2" };
      readonly rejectResourceTypes: readonly string[];
    },
  ): Promise<Response>;
};

export type Env = {
  readonly API_TOKEN: string;
  readonly OPENROUTER_API_KEY: string;
  readonly PUBLIC_DASHBOARD?: string;
  readonly SERPER_API_KEY: string;
  readonly DB: D1Database;
  readonly ARTIFACTS: R2Bucket;
  readonly BROWSER: BrowserRunBinding;
  readonly RUN_QUEUE: Queue<unknown>;
  readonly BROWSER_QUEUE: Queue<unknown>;
  readonly RESEARCH_WORKFLOW: Workflow<WorkflowParams>;
};
