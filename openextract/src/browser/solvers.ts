import { z } from "zod";
import { firstLine } from "../support/errors.ts";

type TokenVendor = "turnstile" | "hcaptcha" | "recaptcha";
export type SolverName = "capsolver" | "twocaptcha";

const solverSolutionSchema = z.object({
  token: z.string().optional(),
  gRecaptchaResponse: z.string().optional(),
  cookies: z.record(z.string(), z.string()).optional(),
  userAgent: z.string().optional(),
});

const solverTaskResponseSchema = z.object({
  errorId: z.number().optional(),
  errorCode: z.string().optional(),
  errorDescription: z.string().optional(),
  taskId: z.union([z.string(), z.number()]).optional(),
  status: z.string().optional(),
  solution: solverSolutionSchema.optional(),
});

export type SolverSolution = z.infer<typeof solverSolutionSchema>;

type SolverDefinition = {
  key: string;
  baseURL: string;
  tasks: Record<TokenVendor, (url: string, sitekey: string) => Record<string, unknown>>;
};

const capsolverKey = process.env.CAPSOLVER_API_KEY ?? "";
const twoCaptchaKey = process.env.TWOCAPTCHA_API_KEY ?? "";
const solverNames = ["capsolver", "twocaptcha"] as const;

const solvers = {
  capsolver: {
    key: capsolverKey,
    baseURL: "https://api.capsolver.com",
    tasks: {
      turnstile: (url: string, sitekey: string) => ({
        type: "AntiTurnstileTaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      hcaptcha: (url: string, sitekey: string) => ({
        type: "HCaptchaTaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      recaptcha: (url: string, sitekey: string) => ({
        type: "ReCaptchaV2TaskProxyLess",
        websiteURL: url,
        websiteKey: sitekey,
      }),
    },
  },
  twocaptcha: {
    key: twoCaptchaKey,
    baseURL: "https://api.2captcha.com",
    tasks: {
      turnstile: (url: string, sitekey: string) => ({
        type: "TurnstileTaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      hcaptcha: (url: string, sitekey: string) => ({
        type: "HCaptchaTaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
      recaptcha: (url: string, sitekey: string) => ({
        type: "RecaptchaV2TaskProxyless",
        websiteURL: url,
        websiteKey: sitekey,
      }),
    },
  },
} satisfies Record<SolverName, SolverDefinition>;

export const hasSolver = solverNames.some((name) => Boolean(solvers[name].key));
export const hasCapsolver = Boolean(capsolverKey);

export function solverOrder(preferred?: string): SolverName[] {
  const available = solverNames.filter((name) => solvers[name].key);
  if (preferred === "capsolver" || preferred === "twocaptcha") {
    if (solvers[preferred].key) {
      return [preferred, ...available.filter((name) => name !== preferred)];
    }
  }
  return [...available];
}

async function createSolverTask(
  baseURL: string,
  clientKey: string,
  task: Record<string, unknown>,
): Promise<SolverSolution> {
  const created = solverTaskResponseSchema.parse(
    await fetch(`${baseURL}/createTask`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ clientKey, task }),
    }).then((response) => response.json()),
  );
  if (created.errorId) {
    throw new Error(`createTask: ${created.errorDescription ?? created.errorCode}`);
  }
  const taskID = created.taskId;
  if (!taskID) throw new Error("createTask returned no taskId");

  for (let attempt = 0; attempt < 40; attempt++) {
    await Bun.sleep(3000);
    const result = solverTaskResponseSchema.parse(
      await fetch(`${baseURL}/getTaskResult`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ clientKey, taskId: taskID }),
      }).then((response) => response.json()),
    );
    if (result.errorId) {
      throw new Error(`getTaskResult: ${result.errorDescription ?? result.errorCode}`);
    }
    if (result.status === "ready") {
      return result.solution ?? {};
    }
  }
  throw new Error("solver timeout");
}

export async function solveToken(
  vendor: TokenVendor,
  target: string,
  sitekey: string | null,
  preferred?: string,
): Promise<{ token: string; provider: SolverName }> {
  if (!sitekey) throw new Error(`no sitekey for ${vendor}`);
  let lastError: unknown;
  for (const name of solverOrder(preferred)) {
    try {
      const solver = solvers[name];
      const solution = await createSolverTask(solver.baseURL, solver.key, solver.tasks[vendor](target, sitekey));
      const token = solution.token ?? solution.gRecaptchaResponse;
      if (token) return { token, provider: name };
      lastError = new Error("empty solution");
    } catch (error) {
      lastError = error;
      console.error(`solver failed provider=${name} vendor=${vendor} error=${firstLine(error)}`);
    }
  }
  throw new Error(`no token for ${vendor}: ${firstLine(lastError ?? "no solver configured")}`);
}

export async function solveCloudflare(target: string, proxy: string): Promise<SolverSolution> {
  return createSolverTask(solvers.capsolver.baseURL, capsolverKey, {
    type: "AntiCloudflareTask",
    websiteURL: target,
    proxy,
  });
}
