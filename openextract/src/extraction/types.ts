export type ExtractProvider = "impit" | "patchright" | "patchright+proxy" | "patchright+solver" | "tavily";

export type ExtractOutcome = "ok" | "dead" | "failed";

export type BrowserOptions = {
  useProxy: boolean;
  solve: boolean;
  solver?: string;
};

export type ExtractAttempt = {
  provider: ExtractProvider;
  outcome: "ok" | "empty" | "blocked" | "http-error" | "error" | "skipped";
  status?: number;
  durationMs: number;
  detail?: string;
};

export type ExtractResult = {
  url: string;
  content: string;
  contentType: "html" | "pdf" | "text" | "unknown";
  provider: ExtractProvider;
  outcome: ExtractOutcome;
  links: string[];
  attempts: ExtractAttempt[];
};
