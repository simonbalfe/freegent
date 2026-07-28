import { describe, expect, test } from "bun:test";
import { ConcurrencyLimiter, QueueAbortedError, QueueSaturatedError } from "./limiter.ts";

function deferred(): { promise: Promise<void>; resolve: () => void } {
  let resolve = () => {};
  const promise = new Promise<void>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

describe("ConcurrencyLimiter", () => {
  test("limits active work and drains waiting work", async () => {
    const limiter = new ConcurrencyLimiter(2, 2);
    const gates = [deferred(), deferred(), deferred(), deferred()];
    let active = 0;
    let peak = 0;
    const tasks = gates.map((gate) =>
      limiter.run(async () => {
        active++;
        peak = Math.max(peak, active);
        await gate.promise;
        active--;
      }),
    );
    await Bun.sleep(1);
    expect(limiter.stats).toEqual({ active: 2, waiting: 2, concurrency: 2, maxWaiting: 2 });
    expect(peak).toBe(2);
    gates[0]?.resolve();
    gates[1]?.resolve();
    await Bun.sleep(1);
    expect(limiter.stats.active).toBe(2);
    expect(limiter.stats.waiting).toBe(0);
    gates[2]?.resolve();
    gates[3]?.resolve();
    await Promise.all(tasks);
    expect(limiter.stats.active).toBe(0);
  });

  test("rejects work when the waiting queue is full", async () => {
    const limiter = new ConcurrencyLimiter(1, 1);
    const first = deferred();
    const second = deferred();
    const active = limiter.run(() => first.promise);
    const waiting = limiter.run(() => second.promise);
    await expect(limiter.run(async () => {})).rejects.toBeInstanceOf(QueueSaturatedError);
    first.resolve();
    second.resolve();
    await Promise.all([active, waiting]);
  });

  test("removes aborted waiting work", async () => {
    const limiter = new ConcurrencyLimiter(1, 1);
    const gate = deferred();
    const active = limiter.run(() => gate.promise);
    const controller = new AbortController();
    const waiting = limiter.run(async () => {}, controller.signal);
    controller.abort();
    await expect(waiting).rejects.toBeInstanceOf(QueueAbortedError);
    expect(limiter.stats.waiting).toBe(0);
    gate.resolve();
    await active;
  });
});
