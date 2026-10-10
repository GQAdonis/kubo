import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { screen } from '@testing-library/react';
import '../../src/island/zod-config';
import { fixture } from '../helpers/fixtures';
import { idsFor, mountIsland, say, untilIdle } from '../helpers/island';

// G11-4, no unsafe-eval, at run time: with the island's zod configuration in place, a full chat turn
// (contract validation of every tool result, the approval card, the A2UI surface) never constructs a
// function from a string. A page CSP without 'unsafe-eval' would report each such construction.

describe('no code generation during a chat turn', () => {
  const real = globalThis.Function;
  let constructed: string[];

  beforeEach(() => {
    constructed = [];
    globalThis.Function = new Proxy(real, {
      construct(target, args, newTarget) {
        constructed.push(args.map(String).join(','));
        return Reflect.construct(target, args, newTarget);
      },
      apply(target, thisArg, args) {
        constructed.push(args.map(String).join(','));
        return Reflect.apply(target, thisArg, args);
      },
    });
  });
  afterEach(() => {
    globalThis.Function = real;
  });

  it.each(['read-pin-ls', 'egress-refused', 'write-interrupt'])('constructs no function while replaying %s', async (name) => {
    const f = fixture(name);
    const m = mountIsland({ trustAll: true, props: { ids: idsFor(f, ['run-1']) } });
    await say(m, String(f.input.messages[0]?.['content']));
    await untilIdle();
    await screen.findByRole('log', { name: 'Conversation' });
    // jsdom compiles CSS selectors with nwsapi, which builds `function resolver(` through the Function constructor.
    // That is the test environment, not the island; a browser has native selectors. Everything else must be absent.
    const fromIsland = constructed.filter((code) => !code.includes("function resolver("));
    expect(fromIsland).toEqual([]);
    expect(constructed.length).toBeGreaterThanOrEqual(0);
  });
});
