import { StrictMode, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";

type JSONObject = Record<string, unknown>;

type Tokens = {
  readonly input: number;
  readonly output: number;
};

type Costs = {
  readonly openRouterUSD: number;
  readonly apifyUSD: number;
  readonly openRouterRecorded: boolean;
  readonly apifyRuns: number;
};

type Step = {
  readonly kind: string;
  readonly name: string;
  readonly input: JSONObject;
};

type Result = {
  readonly result: JSONObject;
  readonly sources: readonly string[];
  readonly agentLog: readonly Step[];
  readonly tokens: Tokens;
  readonly costs: Costs;
  readonly model: string;
  readonly error: string;
};

type JobRow = {
  readonly index: number;
  readonly input: JSONObject;
  readonly status: string;
  readonly result: Result;
};

type JobEvent = {
  readonly at: string;
  readonly row: number;
  readonly message: string;
};

type Job = {
  readonly id: string;
  readonly name: string;
  readonly template: string;
  readonly schema: unknown;
  readonly status: string;
  readonly total: number;
  readonly completed: number;
  readonly createdAt: string;
  readonly latestEvent: string;
  readonly rows: readonly JobRow[];
  readonly events: readonly JobEvent[];
};

type ModelStats = {
  readonly model: string;
  readonly inputTokens: number;
  readonly outputTokens: number;
  readonly openRouterUSD: number;
  readonly unpricedInputTokens: number;
  readonly unpricedOutputTokens: number;
};

type JobStats = {
  readonly rows: number;
  readonly completed: number;
  readonly failed: number;
  readonly skipped: number;
  readonly tokens: Tokens;
  readonly agentSteps: number;
  readonly sources: number;
  readonly durationMS: number;
  readonly costs: Costs;
  readonly unpricedApifyRuns: number;
  readonly serperQueries: number;
  readonly models: readonly ModelStats[];
};

type ModelPrice = { readonly prompt: number; readonly completion: number };

const defaultInstructions =
  "Use current reliable evidence. Follow the requested task exactly and do not guess unsupported facts.";
const defaultTemplate =
  "Research {{subject}} using {{url}} when supplied. Return a concise factual answer.";
const defaultSchema = '{"answer":"string"}';
const fieldClass = "w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-xs text-slate-900 outline-none placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-600/15";
const codeFieldClass = `${fieldClass} font-mono leading-relaxed`;
const textareaClass = `${fieldClass} min-h-24 resize-y`;

function isObject(value: unknown): value is JSONObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function object(value: unknown): JSONObject {
  if (!isObject(value)) throw new Error("Freegent returned an invalid response");
  return value;
}

function optionalObject(value: unknown): JSONObject {
  return isObject(value) ? value : {};
}

function text(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function number(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function bool(value: unknown): boolean {
  return value === true;
}

function array(value: unknown): readonly unknown[] {
  return Array.isArray(value) ? value : [];
}

function parseStep(value: unknown): Step {
  const data = object(value);
  return { kind: text(data.kind), name: text(data.name), input: optionalObject(data.input) };
}

function safeSource(value: unknown): string | null {
  if (typeof value !== "string") return null;
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:" ? value : null;
  } catch {
    return null;
  }
}

function parseResult(value: unknown): Result {
  const data = optionalObject(value);
  const tokenData = optionalObject(data.tokens);
  const costData = optionalObject(data.costs);
  return {
    result: optionalObject(data.result),
    sources: array(data.sources).flatMap((source) => {
      const url = safeSource(source);
      return url === null ? [] : [url];
    }),
    agentLog: array(data.agentLog).map(parseStep),
    tokens: { input: number(tokenData.input), output: number(tokenData.output) },
    costs: {
      openRouterUSD: number(costData.openRouterUsd),
      apifyUSD: number(costData.apifyUsd),
      openRouterRecorded: bool(costData.openRouterRecorded),
      apifyRuns: number(costData.apifyRuns),
    },
    model: text(data.model),
    error: text(data.error),
  };
}

function parseModelStats(value: unknown): ModelStats {
  const data = object(value);
  return {
    model: text(data.model),
    inputTokens: number(data.inputTokens),
    outputTokens: number(data.outputTokens),
    openRouterUSD: number(data.openRouterUsd),
    unpricedInputTokens: number(data.unpricedInputTokens),
    unpricedOutputTokens: number(data.unpricedOutputTokens),
  };
}

function parseJobStats(value: unknown): JobStats {
  const data = object(value);
  const tokens = optionalObject(data.tokens);
  const costs = optionalObject(data.costs);
  return {
    rows: number(data.rows),
    completed: number(data.completed),
    failed: number(data.failed),
    skipped: number(data.skipped),
    tokens: { input: number(tokens.input), output: number(tokens.output) },
    agentSteps: number(data.agentSteps),
    sources: number(data.sources),
    durationMS: number(data.durationMs),
    costs: {
      openRouterUSD: number(costs.openRouterUsd),
      apifyUSD: number(costs.apifyUsd),
      openRouterRecorded: bool(costs.openRouterRecorded),
      apifyRuns: number(costs.apifyRuns),
    },
    unpricedApifyRuns: number(data.unpricedApifyRuns),
    serperQueries: number(data.serperQueries),
    models: array(data.models).map(parseModelStats),
  };
}

function parseRow(value: unknown): JobRow {
  const data = object(value);
  return {
    index: number(data.index),
    input: optionalObject(data.input),
    status: text(data.status),
    result: parseResult(data.result),
  };
}

function parseEvent(value: unknown): JobEvent {
  const data = object(value);
  return { at: text(data.at), row: number(data.row), message: text(data.message) };
}

function parseJob(value: unknown): Job {
  const data = object(value);
  const id = text(data.id);
  if (id === "") throw new Error("Freegent returned a job without an ID");
  return {
    id,
    name: text(data.name),
    template: text(data.template),
    schema: data.schema,
    status: text(data.status),
    total: number(data.total),
    completed: number(data.completed),
    createdAt: text(data.createdAt),
    latestEvent: text(data.latestEvent),
    rows: array(data.rows).map(parseRow),
    events: array(data.events).map(parseEvent),
  };
}

async function requestJSON(url: string, options?: RequestInit): Promise<unknown> {
  const response = await fetch(url, options);
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const message = text(optionalObject(payload).error);
    throw new Error(message || `${response.status} ${response.statusText}`);
  }
  return payload;
}

function initialJobID(): string {
  const raw = location.pathname.match(/^\/dashboard\/jobs\/([^/]+)\/?$/)?.[1];
  if (raw === undefined) return "";
  try {
    return decodeURIComponent(raw);
  } catch {
    return "";
  }
}

function active(job: Job): boolean {
  return job.status === "queued" || job.status === "running";
}

function pretty(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value;
  return JSON.stringify(value, null, 2) ?? "";
}

function sheetValue(value: unknown): string {
  if (value === null || value === undefined) return "Not found";
  if (Array.isArray(value)) {
    if (value.length === 0) return "None found";
    const names = value.map((item) => isObject(item) ? text(item.name) : text(item)).filter(Boolean);
    return names.length === value.length ? `${value.length} · ${names.join(", ")}` : `${value.length} items`;
  }
  if (typeof value !== "string") return pretty(value);
  const trimmed = value.trim();
  if (trimmed === "" || trimmed === "null") return "Not found";
  if (trimmed === "[]") return "None found";
  return value;
}

function label(value: string): string {
  return value.replaceAll("_", " ").replace(/^./, (letter) => letter.toUpperCase());
}

function formatDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString();
}

function eventMessage(message: string): string {
  if (message.startsWith("run start")) return "Agent started research";
  const requested = message.match(/model requested tool=([^ ]+)/)?.[1];
  if (requested !== undefined) return `Agent selected ${requested}`;
  const completed = message.match(/tool=([^ ]+).*completed/)?.[1];
  if (completed !== undefined) return `${completed} completed`;
  if (message.includes("schema-valid final answer")) return "Answer passed schema validation";
  if (message.startsWith("finalizer start")) return "Agent is finalizing the answer";
  return message;
}

function statusClass(status: string): string {
  const tone = status === "running" || status === "queued"
    ? "bg-amber-100 text-amber-800"
    : status === "completed"
      ? "bg-emerald-100 text-emerald-800"
      : status === "failed"
        ? "bg-red-100 text-red-800"
        : "bg-slate-100 text-slate-700";
  return `inline-flex w-fit items-center rounded-full px-1.5 py-0.5 text-[10px] font-bold capitalize ${tone}`;
}

function columns(rows: readonly JobRow[], pick: (row: JobRow) => JSONObject): readonly string[] {
  const names = new Set<string>();
  for (const row of rows) {
    for (const name of Object.keys(pick(row))) names.add(name);
  }
  return [...names].sort((left, right) => left.localeCompare(right));
}

function NewJob({ submitting, error, onSubmit, onClose }: {
  readonly submitting: boolean;
  readonly error: string;
  readonly onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  readonly onClose: () => void;
}): React.JSX.Element {
  const [fileName, setFileName] = useState("");
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-slate-950/40 p-4 backdrop-blur-[1px]" role="dialog" aria-modal="true" aria-labelledby="new-job-title" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }} onKeyDown={(event) => {
      if (event.key === "Escape") onClose();
    }}>
      <form className="max-h-[90vh] w-full max-w-3xl overflow-auto rounded-xl border border-slate-200 bg-white shadow-2xl" onSubmit={(event) => void onSubmit(event)}>
        <header className="flex items-start justify-between border-b border-slate-200 px-5 py-4">
          <div><h2 id="new-job-title" className="m-0 text-lg font-black tracking-tight">New research job</h2><p className="m-0 mt-0.5 text-xs text-slate-500">Upload a CSV, then tell Freegent what to research for every row.</p></div>
          <button className="cursor-pointer rounded p-1 text-lg leading-none text-slate-400 hover:bg-slate-100 hover:text-slate-700" type="button" onClick={onClose} aria-label="Close">×</button>
        </header>
        <div className="grid gap-5 px-5 py-4">
          <section className="grid gap-2">
            <div><h3 className="m-0 text-xs font-bold">1. Choose your data</h3><p className="m-0 mt-0.5 text-[10px] text-slate-500">The first CSV row must contain column names. Every following row becomes one operation.</p></div>
            <label className="flex min-h-20 cursor-pointer items-center justify-center rounded-lg border border-dashed border-slate-300 bg-slate-50 px-4 text-center hover:border-blue-500 hover:bg-blue-50/40 focus-within:border-blue-600 focus-within:ring-2 focus-within:ring-blue-600/15">
              <input className="sr-only" type="file" name="csv" accept=".csv,text/csv" required onChange={(event) => setFileName(event.currentTarget.files?.[0]?.name ?? "")} />
              <span>{fileName === "" ? <><strong className="text-blue-700">Choose CSV file</strong><small className="mt-1 block text-[10px] text-slate-500">CSV only</small></> : <><strong className="text-emerald-700">✓ {fileName}</strong><small className="mt-1 block text-[10px] text-slate-500">Click to replace</small></>}</span>
            </label>
          </section>
          <section className="grid gap-3 border-t border-slate-200 pt-4">
            <div><h3 className="m-0 text-xs font-bold">2. Define the research</h3><p className="m-0 mt-0.5 text-[10px] text-slate-500">Use CSV column names inside double braces, for example <code>{"{{company}}"}</code>.</p></div>
            <label className="grid gap-1.5 text-[11px] font-bold">Job name <span className="font-normal text-slate-400">Optional</span><input className={fieldClass} name="name" placeholder="e.g. Company research" autoComplete="off" autoFocus /></label>
            <label className="grid gap-1.5 text-[11px] font-bold">Prompt template<textarea className={`${codeFieldClass} min-h-20 resize-y`} name="template" defaultValue={defaultTemplate} /></label>
          </section>
          <details className="group border-t border-slate-200 pt-3">
            <summary className="flex cursor-pointer list-none items-center justify-between text-[11px] font-bold"><span>Advanced settings <small className="ml-1 font-normal text-slate-400">Instructions and output schema</small></span><span className="text-slate-400 group-open:hidden">＋</span><span className="hidden text-slate-400 group-open:inline">−</span></summary>
            <div className="mt-3 grid grid-cols-2 gap-3 max-sm:grid-cols-1">
              <label className="grid gap-1.5 text-[11px] font-bold">Instructions<textarea className={textareaClass} name="instructions" defaultValue={defaultInstructions} /></label>
              <label className="grid gap-1.5 text-[11px] font-bold">Output schema<textarea className={`${textareaClass} font-mono`} name="schema" defaultValue={defaultSchema} /><small className="font-normal text-slate-500">Use full JSON Schema for arrays or objects. Allow null when a value may be unknown.</small></label>
            </div>
          </details>
          {error !== "" && <p className="m-0 whitespace-pre-wrap rounded-md bg-red-50 p-2 text-xs text-red-700" role="alert">{error}</p>}
        </div>
        <div className="flex items-center justify-between border-t border-slate-200 bg-slate-50 px-5 py-3">
          <span className="text-[10px] text-slate-500">You can monitor every row after starting.</span>
          <div className="flex gap-2"><button className="cursor-pointer rounded-md border border-slate-300 bg-white px-3 py-2 text-xs font-bold hover:bg-slate-50" type="button" onClick={onClose}>Cancel</button>
          <button className="cursor-pointer rounded-md bg-blue-700 px-4 py-2 text-xs font-bold text-white hover:bg-blue-800 disabled:cursor-not-allowed disabled:opacity-40" type="submit" disabled={submitting || fileName === ""}>{submitting ? "Starting…" : "Start research"}</button></div>
        </div>
      </form>
    </div>
  );
}

function RowAnalytics({ row, events }: {
  readonly row: JobRow;
  readonly events: readonly JobEvent[];
}): React.JSX.Element {
  return (
    <div className="flex min-h-0 flex-1 flex-col bg-white text-[11px]">
      <div className="flex min-h-10 flex-wrap items-center gap-x-6 gap-y-1 border-b border-slate-200 px-3 py-1.5">
        <Metric label="Input tokens" value={row.result.tokens.input.toLocaleString()} />
        <Metric label="Output tokens" value={row.result.tokens.output.toLocaleString()} />
        <Metric label="Agent steps" value={row.result.agentLog.length.toString()} />
        <Metric label="Sources" value={row.result.sources.length.toString()} />
      </div>
      {row.result.error !== "" && <p className="m-0 whitespace-pre-wrap border-b border-red-200 bg-red-50 px-3 py-2 text-red-800"><strong>Error:</strong> {row.result.error}</p>}
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(260px,28%)_minmax(360px,42%)_minmax(280px,30%)] max-lg:block max-lg:overflow-auto">
        <div className="grid min-h-0 grid-rows-2 border-r border-slate-200 max-lg:min-h-[600px]">
          <section className="flex min-h-0 flex-col border-b border-slate-200">
            <h3 className="m-0 border-b border-slate-200 bg-slate-50 px-3 py-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Input row</h3>
            <pre className="min-h-0 flex-1 p-3">{pretty(row.input)}</pre>
          </section>
          <section className="flex min-h-0 flex-col">
            <h3 className="m-0 border-b border-blue-100 bg-blue-50 px-3 py-2 text-[10px] font-extrabold tracking-wide text-blue-700 uppercase">Research output</h3>
            <pre className="min-h-0 flex-1 bg-blue-50/30 p-3">{pretty(row.result.result)}</pre>
          </section>
        </div>
        <section className="flex min-h-0 flex-col border-r border-slate-200 max-lg:min-h-[500px]">
          <h3 className="m-0 border-b border-slate-200 bg-slate-50 px-3 py-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Agent run</h3>
          {row.result.agentLog.length === 0 ? <p className="m-0 text-slate-500">No completed steps yet.</p> : (
            <ol className="m-0 min-h-0 flex-1 list-none overflow-auto p-0">
              {row.result.agentLog.map((step, index) => (
                <li className="border-b border-slate-100 px-3 py-2" key={`${step.kind}-${step.name}-${index}`}>
                  <div className="flex gap-2"><span className="w-5 shrink-0 tabular-nums text-slate-400">{index + 1}</span><strong>{label(step.kind)}{step.name === "" ? "" : ` · ${step.name}`}</strong></div>
                  {Object.keys(step.input).length > 0 && <pre className="mt-1 pl-7 text-[11px] text-slate-600">{pretty(step.input)}</pre>}
                </li>
              ))}
            </ol>
          )}
        </section>
        <div className="grid min-h-0 grid-rows-2 max-lg:min-h-[600px]">
          <section className="flex min-h-0 flex-col border-b border-slate-200">
            <h3 className="m-0 border-b border-slate-200 bg-slate-50 px-3 py-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Sources</h3>
            <div className="min-h-0 flex-1 overflow-auto">
              {row.result.sources.map((source) => <a className="block truncate border-b border-slate-100 px-3 py-2" href={source} target="_blank" rel="noreferrer" key={source}>{source}</a>)}
              {row.result.sources.length === 0 && <p className="m-0 p-3 text-slate-500">No sources.</p>}
            </div>
          </section>
          <section className="flex min-h-0 flex-col">
            <h3 className="m-0 border-b border-slate-200 bg-slate-50 px-3 py-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Activity</h3>
            <div className="min-h-0 flex-1 overflow-auto">
              {events.map((event, index) => <p className="m-0 border-b border-slate-100 px-3 py-2" key={`${event.at}-${index}`}><small className="mr-2 text-slate-500">{formatTime(event.at)}</small>{eventMessage(event.message)}</p>)}
              {events.length === 0 && <p className="m-0 p-3 text-slate-500">No activity.</p>}
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

function Metric({ label: metricLabel, value }: { readonly label: string; readonly value: string }): React.JSX.Element {
  return <div className="flex items-baseline gap-1.5"><strong className="text-sm tabular-nums">{value}</strong><span className="text-[10px] text-slate-500">{metricLabel}</span></div>;
}

function Spreadsheet({ job, onSelectRow }: {
  readonly job: Job;
  readonly onSelectRow: (index: number) => void;
}): React.JSX.Element {
  const inputColumns = useMemo(() => columns(job.rows, (row) => row.input), [job.rows]);
  const outputColumns = useMemo(() => columns(job.rows, (row) => row.result.result), [job.rows]);

  if (job.rows.length === 0) return <div className="grid min-h-80 place-items-center text-slate-500">Waiting for rows…</div>;

  return (
    <div className="min-h-0 flex-1 overflow-auto border-t border-slate-200 bg-white">
      <table className="w-max min-w-full border-separate border-spacing-0 text-left text-[11px]">
        <thead className="sticky top-0 z-20">
          <tr>
            <th className="sticky left-0 z-30 min-w-11 border-r border-b border-slate-300 bg-slate-100 px-2 py-2">#</th>
            <th className="min-w-24 border-r border-b border-slate-300 bg-slate-100 px-2 py-2">Status</th>
            {inputColumns.map((name) => <th className="min-w-44 max-w-56 border-r border-b border-slate-300 bg-slate-100 px-2 py-2 font-bold" key={`input-${name}`}><span className="mr-2 text-[10px] text-slate-400">T</span>{label(name)}</th>)}
            {outputColumns.map((name) => <th className="min-w-44 max-w-56 border-r border-b border-blue-200 bg-blue-50 px-2 py-2 font-bold text-blue-950" key={`output-${name}`}><span className="mr-2 text-[10px] text-blue-600">✦</span>{label(name)}</th>)}
          </tr>
        </thead>
        <tbody>
          {job.rows.map((row) => (
            <tr className="hover:bg-slate-50" key={row.index}>
              <td className="sticky left-0 z-10 border-r border-b border-slate-200 bg-inherit p-0">
                <button className="flex h-8 w-full cursor-pointer items-center gap-1.5 px-2 font-medium text-blue-700 hover:underline" type="button" onClick={() => onSelectRow(row.index)}>
                  ↗ {row.index + 1}
                </button>
              </td>
              <td className="border-r border-b border-slate-200 px-2 py-1"><span className={statusClass(row.status)}>{row.status}</span></td>
              {inputColumns.map((name) => <td className="max-w-56 border-r border-b border-slate-200 px-2 py-1.5 align-middle" key={`input-${name}`}><div className="truncate whitespace-nowrap text-slate-700">{pretty(row.input[name])}</div></td>)}
              {outputColumns.map((name) => <td className="max-w-56 border-r border-b border-blue-100 bg-blue-50/20 p-0 align-middle" key={`output-${name}`}><button aria-label={`Open row ${row.index + 1} details for ${label(name)}`} className="block h-8 w-full cursor-pointer truncate whitespace-nowrap px-2 py-1.5 text-left text-slate-900 hover:bg-blue-100/70 hover:text-blue-800" title="Open full row details" type="button" onClick={() => onSelectRow(row.index)}>{sheetValue(row.result.result[name])}</button></td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function RowDetail({ job, row, onBack }: {
  readonly job: Job;
  readonly row: JobRow;
  readonly onBack: () => void;
}): React.JSX.Element {
  const events = job.events.filter((event) => event.row === row.index + 1);
  return (
    <main className="flex min-h-0 flex-1 flex-col bg-white">
      <div className="flex h-11 items-center gap-3 border-b border-slate-200 bg-white px-3">
        <button className="cursor-pointer text-xs font-bold text-blue-700 hover:underline" type="button" onClick={onBack}>← Spreadsheet</button>
        <strong className="text-sm">Row {row.index + 1} run</strong>
        <span className={statusClass(row.status)}>{row.status}</span>
      </div>
      <RowAnalytics row={row} events={events} />
    </main>
  );
}

function JobDefinition({ job, onClose }: {
  readonly job: Job;
  readonly onClose: () => void;
}): React.JSX.Element {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-slate-950/30 p-4" role="dialog" aria-modal="true" aria-labelledby="job-definition-title" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }} onKeyDown={(event) => {
      if (event.key === "Escape") onClose();
    }}>
      <div className="w-full max-w-4xl rounded-lg border border-slate-200 bg-white shadow-2xl">
        <header className="flex h-11 items-center justify-between border-b border-slate-200 px-3">
          <strong id="job-definition-title" className="text-sm">Job setup</strong>
          <button className="cursor-pointer text-xs font-bold text-blue-700" type="button" onClick={onClose} autoFocus>Close</button>
        </header>
        <div className="grid grid-cols-2 gap-3 p-3 max-md:grid-cols-1">
        <section>
          <h2 className="mb-1 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Prompt template</h2>
          <pre className="max-h-72 rounded border border-slate-200 bg-slate-50 p-2 text-[11px]">{job.template || "Not available"}</pre>
        </section>
        <section>
          <h2 className="mb-1 text-[10px] font-extrabold tracking-wide text-blue-700 uppercase">Output target</h2>
          <pre className="max-h-72 rounded border border-blue-200 bg-blue-50 p-2 text-[11px]">{pretty(job.schema) || "Not available"}</pre>
        </section>
        </div>
      </div>
    </div>
  );
}

function openRouterPrices(value: unknown): Readonly<Record<string, ModelPrice>> {
  const prices: Record<string, ModelPrice> = {};
  for (const item of array(object(value).data)) {
    const model = object(item);
    const pricing = optionalObject(model.pricing);
    const id = text(model.id);
    const prompt = Number.parseFloat(text(pricing.prompt));
    const completion = Number.parseFloat(text(pricing.completion));
    if (id !== "" && Number.isFinite(prompt) && Number.isFinite(completion)) prices[id] = { prompt, completion };
  }
  return prices;
}

function usd(value: number): string {
  return new Intl.NumberFormat("en-US", { style: "currency", currency: "USD", minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(value);
}

function duration(value: number): string {
  const seconds = Math.round(value / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  return `${minutes}m ${seconds % 60}s`;
}

function DetailedStats({ job, onClose }: {
  readonly job: Job;
  readonly onClose: () => void;
}): React.JSX.Element {
  const [stats, setStats] = useState<JobStats | null>(null);
  const [prices, setPrices] = useState<Readonly<Record<string, ModelPrice>>>({});
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    void requestJSON(`/jobs/${encodeURIComponent(job.id)}/stats`)
      .then((value) => { if (!cancelled) setStats(parseJobStats(value)); })
      .catch((reason: unknown) => { if (!cancelled) setError(reason instanceof Error ? reason.message : "Could not load stats"); });
    void requestJSON("https://openrouter.ai/api/v1/models")
      .then((value) => { if (!cancelled) setPrices(openRouterPrices(value)); })
      .catch(() => undefined);
    return () => { cancelled = true; };
  }, [job.id]);

  const estimates = stats?.models.map((model) => {
    const price = prices[model.model];
    return {
      ...model,
      estimate: price === undefined ? 0 : model.unpricedInputTokens * price.prompt + model.unpricedOutputTokens * price.completion,
      missingPrice: price === undefined && model.unpricedInputTokens + model.unpricedOutputTokens > 0,
    };
  }) ?? [];
  const estimatedOpenRouter = estimates.reduce((sum, model) => sum + model.estimate, 0);
  const serperCost = (stats?.serperQueries ?? 0) * 0.001;
  const missingModels = estimates.filter((model) => model.missingPrice).length;
  const openRouterCost = (stats?.costs.openRouterUSD ?? 0) + estimatedOpenRouter;
  const apifyCost = stats?.costs.apifyUSD ?? 0;
  const totalCost = openRouterCost + apifyCost + serperCost;

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-slate-950/30 p-4" role="dialog" aria-modal="true" aria-labelledby="stats-title" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose();
    }} onKeyDown={(event) => {
      if (event.key === "Escape") onClose();
    }}>
      <div className="max-h-[90vh] w-full max-w-5xl overflow-auto rounded-lg border border-slate-200 bg-white shadow-2xl">
        <header className="flex h-11 items-center justify-between border-b border-slate-200 px-3">
          <div><strong id="stats-title" className="text-sm">Detailed stats</strong><span className="ml-2 text-[10px] text-slate-500">Entire sheet</span></div>
          <button className="cursor-pointer text-xs font-bold text-blue-700" type="button" onClick={onClose} autoFocus>Close</button>
        </header>
        {error !== "" && <p className="m-3 rounded bg-red-50 p-2 text-xs text-red-800" role="alert">{error}</p>}
        {stats === null && error === "" ? <p className="m-0 p-6 text-center text-xs text-slate-500">Calculating…</p> : stats !== null && (
          <div className="grid gap-4 p-4 text-xs">
            <div className="grid grid-cols-4 border border-slate-200 max-md:grid-cols-2">
              <Stat label={estimatedOpenRouter > 0 || serperCost > 0 ? "Estimated total cost" : "Recorded total cost"} value={usd(totalCost)} />
              <Stat label="Total tokens" value={(stats.tokens.input + stats.tokens.output).toLocaleString()} />
              <Stat label="Rows finished" value={`${stats.completed + stats.failed + stats.skipped}/${stats.rows}`} />
              <Stat label="Total runtime" value={duration(stats.durationMS)} />
            </div>
            <section>
              <h3 className="m-0 border-b border-slate-200 pb-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Cost breakdown</h3>
              <div className="grid grid-cols-2 gap-x-8 max-md:grid-cols-1">
                <p className="m-0 flex justify-between border-b border-slate-100 py-2"><span>OpenRouter</span><strong>{usd(openRouterCost)}</strong></p>
                <p className="m-0 flex justify-between border-b border-slate-100 py-2"><span>Apify</span><strong>{usd(apifyCost)}</strong></p>
                <p className="m-0 flex justify-between border-b border-slate-100 py-2"><span>Serper</span><strong>{usd(serperCost)}</strong></p>
                <p className="m-0 flex justify-between border-b border-slate-100 py-2 text-slate-500"><span>OpenRouter recorded</span><span>{usd(stats.costs.openRouterUSD)}</span></p>
                <p className="m-0 flex justify-between border-b border-slate-100 py-2 text-slate-500"><span>OpenRouter legacy estimate</span><span>{usd(estimatedOpenRouter)}</span></p>
                <p className="m-0 flex justify-between border-b border-slate-100 py-2 text-slate-500"><span>Serper Starter rate</span><span>{stats.serperQueries.toLocaleString()} × $0.001</span></p>
              </div>
              {(missingModels > 0 || stats.unpricedApifyRuns > 0) && <p className="m-0 mt-2 text-[10px] text-amber-700">Total excludes {missingModels > 0 ? `${missingModels} model price${missingModels === 1 ? "" : "s"}` : ""}{missingModels > 0 && stats.unpricedApifyRuns > 0 ? " and " : ""}{stats.unpricedApifyRuns > 0 ? `${stats.unpricedApifyRuns} legacy Apify run${stats.unpricedApifyRuns === 1 ? "" : "s"}` : ""} that could not be priced.</p>}
            </section>
            <section className="overflow-auto">
              <h3 className="m-0 border-b border-slate-200 pb-2 text-[10px] font-extrabold tracking-wide text-slate-500 uppercase">Model and token breakdown</h3>
              <table className="w-full min-w-[620px] border-collapse text-left">
                <thead><tr className="text-[10px] text-slate-500"><th className="py-2">Model</th><th>Input</th><th>Output</th><th>Total</th><th className="text-right">Cost</th></tr></thead>
                <tbody>{estimates.map((model) => <tr className="border-t border-slate-100" key={model.model}><td className="py-2 font-semibold">{model.model || "Unknown"}</td><td>{model.inputTokens.toLocaleString()}</td><td>{model.outputTokens.toLocaleString()}</td><td>{(model.inputTokens + model.outputTokens).toLocaleString()}</td><td className="text-right font-semibold">{usd(model.openRouterUSD + model.estimate)}{model.estimate > 0 && <span className="ml-1 text-[9px] font-normal text-slate-500">est.</span>}</td></tr>)}</tbody>
              </table>
            </section>
            <div className="grid grid-cols-4 border border-slate-200 max-md:grid-cols-2">
              <Stat label="Input tokens" value={stats.tokens.input.toLocaleString()} />
              <Stat label="Output tokens" value={stats.tokens.output.toLocaleString()} />
              <Stat label="Agent steps" value={stats.agentSteps.toLocaleString()} />
              <Stat label="Source references" value={stats.sources.toLocaleString()} />
            </div>
            <p className="m-0 text-[10px] text-slate-500">New runs use provider-reported charges. Legacy OpenRouter rows use current model rates. Apify uses each authenticated run’s billed total. Serper uses the current Starter rate; larger credit packs cost less. <a href="https://openrouter.ai/docs/cookbook/administration/usage-accounting" target="_blank" rel="noreferrer">OpenRouter accounting</a> · <a href="https://docs.apify.com/api/v2/actor-run-get" target="_blank" rel="noreferrer">Apify run costs</a> · <a href="https://serper.dev/" target="_blank" rel="noreferrer">Serper pricing</a></p>
          </div>
        )}
      </div>
    </div>
  );
}

function Stat({ label: statLabel, value }: { readonly label: string; readonly value: string }): React.JSX.Element {
  return <div className="grid gap-0.5 border-r border-slate-200 p-3 last:border-r-0"><span className="text-[10px] text-slate-500">{statLabel}</span><strong className="text-lg tabular-nums">{value}</strong></div>;
}

function App(): React.JSX.Element {
  const [jobs, setJobs] = useState<readonly Job[]>([]);
  const [selectedID, setSelectedID] = useState(initialJobID);
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const [submitError, setSubmitError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [selectedRow, setSelectedRow] = useState<number | null>(null);
  const [showDefinition, setShowDefinition] = useState(false);
  const [showNewJob, setShowNewJob] = useState(false);
  const [showStats, setShowStats] = useState(false);

  useEffect(() => {
    void requestJSON("/jobs")
      .then((payload) => {
        const next = array(object(payload).jobs).map(parseJob);
        setJobs(next);
        setSelectedID((current) => current || next[0]?.id || "");
      })
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "Could not load jobs"));
  }, []);

  useEffect(() => {
    setSelectedRow(null);
    if (selectedID === "") {
      setJob(null);
      return;
    }
    let cancelled = false;
    let timer = 0;
    async function refresh(): Promise<void> {
      try {
        const next = parseJob(await requestJSON(`/jobs/${encodeURIComponent(selectedID)}?limit=200`));
        if (cancelled) return;
        setJob(next);
        setJobs((current) => current.map((item) => item.id === next.id ? next : item));
        setError("");
        if (active(next)) timer = window.setTimeout(() => void refresh(), 1000);
      } catch (reason: unknown) {
        if (!cancelled) setError(reason instanceof Error ? reason.message : "Could not load job");
      }
    }
    void refresh();
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [selectedID]);

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setSubmitError("");
    setSubmitting(true);
    try {
      const form = new FormData(event.currentTarget);
      const file = form.get("csv");
      if (file instanceof File && file.size === 0) form.delete("csv");
      const payload = object(await requestJSON("/jobs", { method: "POST", body: form }));
      const jobID = text(payload.jobId);
      if (jobID === "") throw new Error("Freegent did not return a job ID");
      history.replaceState({}, "", `/dashboard/jobs/${encodeURIComponent(jobID)}`);
      setShowNewJob(false);
      setSelectedID(jobID);
      setSubmitting(false);
    } catch (reason: unknown) {
      setSubmitError(reason instanceof Error ? reason.message : "Could not start research");
      setSubmitting(false);
    }
  }

  const selectableJobs = job !== null && !jobs.some((item) => item.id === job.id) ? [job, ...jobs] : jobs;
  const detailRow = selectedRow === null ? undefined : job?.rows.find((row) => row.index === selectedRow);
  const tokens = job?.rows.reduce((total, row) => ({
    input: total.input + row.result.tokens.input,
    output: total.output + row.result.tokens.output,
  }), { input: 0, output: 0 }) ?? { input: 0, output: 0 };

  return (
    <div className="flex h-screen min-h-[480px] flex-col bg-white">
      <header className="border-b border-slate-200 bg-white">
        <div className="flex h-11 w-full items-center gap-3 px-3 max-md:h-auto max-md:flex-wrap max-md:py-2">
          <div className="mr-1">
            <h1 className="m-0 text-base font-black tracking-tight">Freegent</h1>
            <p className="m-0 text-[8px] font-bold tracking-wider text-blue-700 uppercase">Research spreadsheet</p>
          </div>
          <select className="h-8 min-w-56 rounded-md border border-slate-300 bg-white px-2 text-xs font-semibold outline-none focus:border-blue-600" value={selectedID} onChange={(event) => {
            const id = event.currentTarget.value;
            setSelectedID(id);
            if (id !== "") history.replaceState({}, "", `/dashboard/jobs/${encodeURIComponent(id)}`);
          }}>
            {selectableJobs.length === 0 && <option value="">No jobs yet</option>}
            {selectableJobs.map((item) => <option value={item.id} key={item.id}>{item.name || item.id} · {item.completed}/{item.total}</option>)}
          </select>
          <button className="cursor-pointer text-[11px] font-bold whitespace-nowrap text-blue-700" type="button" onClick={() => {
            setSubmitError("");
            setShowNewJob(true);
          }}>＋ New job</button>
          {job !== null && <>
            <span className={statusClass(job.status)}>{job.status}</span>
            <span className="text-[11px] text-slate-500">{job.completed}/{job.total} rows</span>
            <span className="text-[11px] text-slate-500">{tokens.input.toLocaleString()} in · {tokens.output.toLocaleString()} out</span>
            <span className="min-w-0 flex-1 truncate text-[11px] text-slate-500">{job.latestEvent}</span>
            <button className="cursor-pointer text-[11px] font-bold whitespace-nowrap text-blue-700" type="button" onClick={() => setShowStats(true)}>Detailed stats</button>
            <button className="cursor-pointer text-[11px] font-bold whitespace-nowrap text-blue-700" type="button" onClick={() => setShowDefinition(true)}>Job setup</button>
            <a className="text-[11px] font-bold whitespace-nowrap" href={`/jobs/${encodeURIComponent(job.id)}/results.csv`}>Download CSV</a>
          </>}
        </div>
      </header>
      {error !== "" && <p className="m-0 border-b border-red-200 bg-red-50 px-3 py-1.5 text-xs text-red-800" role="alert">{error}</p>}
      {job === null ? (
        <div className="grid flex-1 place-items-center p-8 text-center text-slate-500">
          <div><strong className="block text-lg text-slate-700">No spreadsheet selected</strong><span>Open “New job” to upload a CSV.</span></div>
        </div>
      ) : detailRow === undefined ? (
        <Spreadsheet job={job} key={job.id} onSelectRow={setSelectedRow} />
      ) : <RowDetail job={job} row={detailRow} onBack={() => setSelectedRow(null)} />}
      {showNewJob && <NewJob submitting={submitting} error={submitError} onSubmit={submit} onClose={() => setShowNewJob(false)} />}
      {job !== null && showStats && <DetailedStats job={job} onClose={() => setShowStats(false)} />}
      {job !== null && showDefinition && <JobDefinition job={job} onClose={() => setShowDefinition(false)} />}
      {job !== null && <footer className="flex h-6 items-center justify-between border-t border-slate-200 bg-white px-3 text-[9px] text-slate-500"><span>{job.id}</span><span>{formatDate(job.createdAt)}</span></footer>}
    </div>
  );
}

const root = document.getElementById("root");
if (root === null) throw new Error("dashboard root is missing");
createRoot(root).render(<StrictMode><App /></StrictMode>);
