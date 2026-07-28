export class QueueSaturatedError extends Error {
  constructor() {
    super("OpenExtract queue is full");
    this.name = "QueueSaturatedError";
  }
}

export class QueueAbortedError extends Error {
  constructor() {
    super("OpenExtract request was aborted");
    this.name = "QueueAbortedError";
  }
}

type Waiter = {
  resolve: () => void;
  reject: (error: Error) => void;
  signal?: AbortSignal;
  abort?: () => void;
};

export type LimiterStats = {
  active: number;
  waiting: number;
  concurrency: number;
  maxWaiting: number;
};

export class ConcurrencyLimiter {
  readonly concurrency: number;
  readonly maxWaiting: number;
  #active = 0;
  #queue: Waiter[] = [];

  constructor(concurrency: number, maxWaiting: number) {
    if (!Number.isInteger(concurrency) || concurrency < 1) {
      throw new Error("concurrency must be a positive integer");
    }
    if (!Number.isInteger(maxWaiting) || maxWaiting < 0) {
      throw new Error("maxWaiting must be a non-negative integer");
    }
    this.concurrency = concurrency;
    this.maxWaiting = maxWaiting;
  }

  get stats(): LimiterStats {
    return {
      active: this.#active,
      waiting: this.#queue.length,
      concurrency: this.concurrency,
      maxWaiting: this.maxWaiting,
    };
  }

  async run<T>(task: () => Promise<T>, signal?: AbortSignal): Promise<T> {
    await this.#acquire(signal);
    try {
      return await task();
    } finally {
      this.#release();
    }
  }

  #acquire(signal?: AbortSignal): Promise<void> {
    if (signal?.aborted) {
      return Promise.reject(new QueueAbortedError());
    }
    if (this.#active < this.concurrency) {
      this.#active++;
      return Promise.resolve();
    }
    if (this.#queue.length >= this.maxWaiting) {
      return Promise.reject(new QueueSaturatedError());
    }
    return new Promise<void>((resolve, reject) => {
      const waiter: Waiter = { resolve, reject, ...(signal ? { signal } : {}) };
      if (signal) {
        waiter.abort = () => {
          const index = this.#queue.indexOf(waiter);
          if (index >= 0) {
            this.#queue.splice(index, 1);
            reject(new QueueAbortedError());
          }
        };
        signal.addEventListener("abort", waiter.abort, { once: true });
      }
      this.#queue.push(waiter);
    });
  }

  #release(): void {
    const waiter = this.#queue.shift();
    if (!waiter) {
      this.#active--;
      return;
    }
    if (waiter.signal && waiter.abort) {
      waiter.signal.removeEventListener("abort", waiter.abort);
    }
    waiter.resolve();
  }
}
