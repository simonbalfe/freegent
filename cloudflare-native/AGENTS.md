# Cloudflare-native Freegent

This directory is an isolated Cloudflare-native prototype. It does not inherit the Go, PostgreSQL, River, Compose, or OpenExtract architecture as implementation constraints.

- `src/index.ts` owns the HTTP API, Queue entrypoint and dispatch reconciliation schedule.
- `src/workflow.ts` owns one durable adaptive research run per input row.
- `src/dispatch.ts` owns the run admission Queue and the separately bounded browser Queue.
- `src/browser.ts` owns Browser Run Quick Actions and R2 browser artifacts.
- `src/storage.ts` owns D1 job state and R2 run artifacts.
- `src/openrouter.ts` owns model calls and schema-backed tool definitions.
- Zod validates HTTP, Queue, D1 and provider boundaries.
- Jobs accept standard JSON Schema. Do not add a shorthand schema language.
- D1 is the queryable control plane, not an evidence blob store.
- R2 stores full evidence and browser artifacts.
- Workflows own durable retries and model/tool state.
- The browser Queue is the Browser Run concurrency gate. Do not call Browser Run directly from research Workflows.
- Queue and Workflow delivery are idempotent through deterministic Workflow IDs and R2 result keys.
- Keep code comment-free. Put durable explanations in this file or `README.md`.

Run `bun run check` after TypeScript changes. Browser Run Quick Actions require remote development mode for live checks.
