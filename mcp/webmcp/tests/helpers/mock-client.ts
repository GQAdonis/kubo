import type { RpcClient } from '../../src/types.js';

export type Handler = (...args: any[]) => unknown;

export interface Recorder {
  client: RpcClient;
  /** Every call made on the client, as `path` + arguments. Property reads are not calls. */
  calls: Array<{ path: string; args: unknown[] }>;
}

/**
 * A client whose every method is a recorded call. Methods without a handler throw, so a
 * test fails loudly if an executor reaches for an RPC method it should not.
 */
export function recorder(handlers: Record<string, Handler> = {}): Recorder {
  const calls: Recorder['calls'] = [];
  const make = (path: string): any =>
    new Proxy(function () {}, {
      get(_t, prop) {
        if (typeof prop === 'symbol') return undefined;
        if (prop === 'then') return undefined;
        return make(path ? `${path}.${prop}` : prop);
      },
      apply(_t, _this, args) {
        calls.push({ path, args });
        const h = handlers[path];
        if (!h) throw new Error(`unexpected RPC call: ${path}`);
        return h(...args);
      },
    });
  return { client: make('') as RpcClient, calls };
}

export async function* gen<T>(...items: T[]): AsyncGenerator<T> {
  for (const i of items) yield i;
}
