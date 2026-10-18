export class FlowController {
  private credit: number;
  private closed = false;
  private waiters: Array<{
    bytes: number;
    isPartial: boolean;
    resolve: (val: number) => void;
    reject: (err: Error) => void;
    cleanup?: () => void;
  }> = [];

  constructor(initialCredit: number) {
    this.credit = Math.max(0, initialCredit);
  }

  public getCredit(): number {
    return this.credit;
  }

  public isClosed(): boolean {
    return this.closed;
  }

  public acquire(n: number, signal?: AbortSignal): Promise<void> {
    if (n <= 0) {
      return Promise.resolve();
    }
    if (this.closed) {
      return Promise.reject(new Error('flow controller closed'));
    }
    if (signal?.aborted) {
      return Promise.reject(signal.reason ?? new Error('aborted'));
    }

    if (this.waiters.length === 0 && this.credit >= n) {
      this.credit -= n;
      return Promise.resolve();
    }

    return new Promise<number>((resolve, reject) => {
      const waiter = {
        bytes: n,
        isPartial: false,
        resolve,
        reject,
        cleanup: undefined as (() => void) | undefined,
      };

      if (signal) {
        const onAbort = () => {
          const idx = this.waiters.indexOf(waiter);
          if (idx !== -1) {
            this.waiters.splice(idx, 1);
            this.drain();
          }
          reject(signal.reason ?? new Error('aborted'));
        };
        signal.addEventListener('abort', onAbort, {once: true});
        waiter.cleanup = () => signal.removeEventListener('abort', onAbort);
      }

      this.waiters.push(waiter);
    }).then(() => undefined);
  }

  public acquirePartial(max: number, signal?: AbortSignal): Promise<number> {
    if (max <= 0) {
      return Promise.resolve(0);
    }
    if (this.closed) {
      return Promise.reject(new Error('flow controller closed'));
    }
    if (signal?.aborted) {
      return Promise.reject(signal.reason ?? new Error('aborted'));
    }

    if (this.waiters.length === 0 && this.credit > 0) {
      const take = Math.min(this.credit, max);
      this.credit -= take;
      return Promise.resolve(take);
    }

    return new Promise<number>((resolve, reject) => {
      const waiter = {
        bytes: max,
        isPartial: true,
        resolve,
        reject,
        cleanup: undefined as (() => void) | undefined,
      };

      if (signal) {
        const onAbort = () => {
          const idx = this.waiters.indexOf(waiter);
          if (idx !== -1) {
            this.waiters.splice(idx, 1);
            this.drain();
          }
          reject(signal.reason ?? new Error('aborted'));
        };
        signal.addEventListener('abort', onAbort, {once: true});
        waiter.cleanup = () => signal.removeEventListener('abort', onAbort);
      }

      this.waiters.push(waiter);
    });
  }

  public addCredit(n: number): void {
    if (this.closed || !Number.isFinite(n) || n <= 0) {
      return;
    }
    this.credit += n;
    this.drain();
  }

  public close(): void {
    if (this.closed) {
      return;
    }
    this.closed = true;
    const err = new Error('flow controller closed');
    const oldWaiters = this.waiters;
    this.waiters = [];
    for (const w of oldWaiters) {
      if (w.cleanup) {
        w.cleanup();
      }
      w.reject(err);
    }
  }

  private drain(): void {
    while (this.waiters.length > 0 && !this.closed) {
      const next = this.waiters[0];
      if (next.isPartial) {
        if (this.credit <= 0) {
          break;
        }
        const take = Math.min(this.credit, next.bytes);
        this.credit -= take;
        this.waiters.shift();
        if (next.cleanup) {
          next.cleanup();
        }
        next.resolve(take);
      } else {
        if (this.credit < next.bytes) {
          break;
        }
        this.credit -= next.bytes;
        this.waiters.shift();
        if (next.cleanup) {
          next.cleanup();
        }
        next.resolve(next.bytes);
      }
    }
  }
}
