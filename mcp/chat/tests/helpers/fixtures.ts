import { readFileSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import type { AguiEvent } from '@gqadonis/ipfs-mcp-ui';

// The recorded contract AG-UI fixtures (mcp/contract/fixtures/agui). Provenance today is
// "authored-from-spec": mcp-agui-endpoint replaces each file with a recording from a real run, and
// these tests then replay the recordings unchanged.

/** Thirty seconds into the fixtures' world: before every interrupt's expiresAt (12:02:00Z), whatever the real clock says. */
export const FIXTURE_NOW = Date.parse('2026-10-09T12:00:30Z');

export const FIXTURE_DIR = resolve(import.meta.dirname, '../../../contract/fixtures/agui');

export interface AguiFixture {
  readonly name: string;
  readonly description: string;
  readonly provenance: 'authored-from-spec' | 'recorded';
  readonly principalKind: 'ui';
  readonly input: {
    readonly threadId: string;
    readonly runId: string;
    readonly messages: readonly Record<string, unknown>[];
    readonly tools: readonly unknown[];
    readonly context: readonly unknown[];
    readonly resume?: readonly Record<string, unknown>[];
  };
  readonly events: readonly AguiEvent[];
}

export function loadFixtures(): AguiFixture[] {
  return readdirSync(FIXTURE_DIR)
    .filter((f) => f.endsWith('.json'))
    .sort()
    .map((file) => {
      const parsed = JSON.parse(readFileSync(join(FIXTURE_DIR, file), 'utf8')) as Omit<AguiFixture, 'name'>;
      return { ...parsed, name: file.replace(/\.json$/, '') };
    });
}

export function fixture(name: string): AguiFixture {
  const f = loadFixtures().find((x) => x.name === name);
  if (!f) throw new Error(`no AG-UI fixture named ${name}`);
  return f;
}

/** The SSE wire form of an event list, one `data:` frame per event. */
export function sseText(events: readonly AguiEvent[]): string {
  return events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join('');
}
