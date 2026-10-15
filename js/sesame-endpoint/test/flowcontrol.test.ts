import {describe, it, expect} from 'bun:test';
import {FlowController} from '../src/rc/flowcontrol';

describe('FlowController', () => {
  it('initializes with credit', () => {
    const fc = new FlowController(100);
    expect(fc.getCredit()).toBe(100);
    expect(fc.isClosed()).toBe(false);
  });

  it('acquires immediate credit without blocking', async () => {
    const fc = new FlowController(100);
    await fc.acquire(50);
    expect(fc.getCredit()).toBe(50);
    await fc.acquire(50);
    expect(fc.getCredit()).toBe(0);
  });

  it('acquire blocks until credit is added', async () => {
    const fc = new FlowController(0);
    let resolved = false;

    const promise = fc.acquire(50).then(() => {
      resolved = true;
    });

    expect(resolved).toBe(false);
    fc.addCredit(20);
    expect(resolved).toBe(false);
    fc.addCredit(30);

    await promise;
    expect(resolved).toBe(true);
    expect(fc.getCredit()).toBe(0);
  });

  it('acquirePartial takes available credit up to max', async () => {
    const fc = new FlowController(40);
    const taken = await fc.acquirePartial(100);
    expect(taken).toBe(40);
    expect(fc.getCredit()).toBe(0);

    // When 0 credit, it blocks until credit arrives
    const partialPromise = fc.acquirePartial(100);
    fc.addCredit(25);
    const taken2 = await partialPromise;
    expect(taken2).toBe(25);
    expect(fc.getCredit()).toBe(0);
  });

  it('respects abort signal', async () => {
    const fc = new FlowController(0);
    const ac = new AbortController();

    const promise = fc.acquire(100, ac.signal);
    ac.abort(new Error('user abort'));

    await expect(promise).rejects.toThrow('user abort');
  });

  it('drains remaining waiters when an earlier waiter is aborted', async () => {
    const fc = new FlowController(50);
    const ac1 = new AbortController();
    const p1 = fc.acquire(100, ac1.signal).catch((err: Error) => err);
    let p2Resolved = false;
    const p2 = fc.acquire(20).then(() => {
      p2Resolved = true;
    });

    await new Promise(r => setTimeout(r, 10));
    ac1.abort();
    await p1;
    await p2;
    expect(p2Resolved).toBe(true);
    expect(fc.getCredit()).toBe(30);
  });

  it('rejects waiters on close', async () => {
    const fc = new FlowController(0);
    const p1 = fc.acquire(50).catch((err: Error) => err);
    const p2 = fc.acquirePartial(50).catch((err: Error) => err);

    fc.close();
    expect(fc.isClosed()).toBe(true);

    const err1 = await p1;
    const err2 = await p2;
    expect(err1).toBeInstanceOf(Error);
    expect((err1 as Error).message).toBe('flow controller closed');
    expect(err2).toBeInstanceOf(Error);
    expect((err2 as Error).message).toBe('flow controller closed');
  });
});
