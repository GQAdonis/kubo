import { TOOL_META, type ToolName } from '@gqadonis/ipfs-mcp-ui';
import { describe, expect, it } from 'vitest';
import { classifyResult, identifyTool, isReadOnlyTool, TOOL_META_KEY, UNTRUSTED_META_KEY } from '../../src/core/result';
import { toolsForView, VIEW_IDS } from '../../src/views';
import { CONFIRMATIONS, OUTPUTS } from '../helpers/fixtures';
import { errorResult, okResult } from '../helpers/port';

const TOOLS = Object.keys(TOOL_META) as ToolName[];

describe('classifyResult', () => {
  it('classifies every contract fixture as a result of its own tool when the server marks the tool', () => {
    for (const tool of TOOLS) {
      for (const output of OUTPUTS[tool]) {
        const view = TOOL_META[tool].appView ?? 'content';
        const o = classifyResult(okResult(tool, output), { view: view as (typeof VIEW_IDS)[number] });
        expect(o.kind, tool).toBe('result');
        if (o.kind === 'result') expect(o.tool).toBe(tool);
      }
    }
  });

  it('identifies the tool from hostContext.toolInfo when the server sends no marker', () => {
    const o = classifyResult({ structuredContent: OUTPUTS.pin_ls[0] }, { view: 'pins', toolInfoName: 'pin_ls' });
    expect(o).toMatchObject({ kind: 'result', tool: 'pin_ls' });
  });

  it('infers the right tool from the structured content of every viewed tool, with no marker and no toolInfo', () => {
    // Pins the claim in identifyTool: inside one view the output schemas do not overlap.
    for (const view of VIEW_IDS) {
      for (const tool of toolsForView(view)) {
        for (const output of OUTPUTS[tool]) {
          expect(identifyTool({ structuredContent: output }, { view }), `${view}/${tool}`).toBe(tool);
        }
      }
    }
  });

  it('ignores a marker or toolInfo that is not a contract tool', () => {
    const r = { structuredContent: OUTPUTS.pin_ls[0], _meta: { [TOOL_META_KEY]: 'rm_rf' } };
    expect(identifyTool(r, { view: 'pins', toolInfoName: '__proto__' })).toBe('pin_ls');
  });

  it('reports structured content no tool of the view explains, never a guess', () => {
    const o = classifyResult({ structuredContent: { surprise: true }, content: [{ type: 'text', text: 'raw' }] }, { view: 'pins' });
    expect(o).toMatchObject({ kind: 'unidentified', text: 'raw' });
  });

  it('turns a confirmation_required error into a confirmation outcome', () => {
    const r = errorResult({ code: 'confirmation_required', message: 'Confirm first.', tool: 'pin_rm', confirmation: CONFIRMATIONS.humanWithToken });
    const o = classifyResult(r, { view: 'pins' });
    expect(o.kind).toBe('confirmation');
    if (o.kind === 'confirmation') expect(o.confirmation.confirmation_id).toBe('cf_3pQm8vK2xR7tLw9a');
  });

  it('treats confirmation_pending the same way', () => {
    const r = errorResult({ code: 'confirmation_pending', message: 'Still pending.', confirmation: CONFIRMATIONS.nativePrompt });
    expect(classifyResult(r, { view: 'node-status' }).kind).toBe('confirmation');
  });

  it('keeps other tool errors as failures with the server message', () => {
    const r = errorResult({ code: 'daemon_unreachable', message: 'The daemon is not running.', retryable: true });
    const o = classifyResult(r, { view: 'node-status' });
    expect(o).toMatchObject({ kind: 'failure', error: { error: { code: 'daemon_unreachable', retryable: true } } });
  });

  it('does not treat confirmation_declined or confirmation_expired as a pending confirmation', () => {
    for (const code of ['confirmation_declined', 'confirmation_expired']) {
      expect(classifyResult(errorResult({ code, message: 'Denied. Unpinning requires confirmation.' }), { view: 'pins' }).kind).toBe('failure');
    }
  });

  it('builds a failure from an isError result whose structured content is not a contract error', () => {
    const o = classifyResult({ isError: true, content: [{ type: 'text', text: 'boom' }], structuredContent: { error: 'nope' } }, { view: 'pins' });
    expect(o).toMatchObject({ kind: 'failure', error: { error: { code: 'internal', message: 'boom' } } });
  });

  it('builds a failure from isError with no structured content and no text', () => {
    const o = classifyResult({ isError: true }, { view: 'pins' });
    expect(o.kind).toBe('failure');
    if (o.kind === 'failure') expect(o.error.error.message.length).toBeGreaterThan(0);
  });

  it('falls back to the text content when there is no structured content, keeping the untrusted marker', () => {
    const plain = classifyResult({ content: [{ type: 'text', text: 'hello' }] }, { view: 'content' });
    expect(plain).toEqual({ kind: 'text', text: 'hello', untrusted: false });
    const net = classifyResult({ content: [{ type: 'text', text: 'bytes' }], _meta: { [UNTRUSTED_META_KEY]: true } }, { view: 'content' });
    expect(net).toEqual({ kind: 'text', text: 'bytes', untrusted: true });
  });

  it('reports an empty result', () => {
    expect(classifyResult({ content: [] }, { view: 'dag' })).toEqual({ kind: 'empty' });
  });

  it('clips very long text', () => {
    const o = classifyResult({ content: [{ type: 'text', text: 'x'.repeat(200_000) }] }, { view: 'content' });
    expect(o.kind === 'text' && o.text.length).toBeLessThan(70_000);
  });
});

describe('read-only invariant', () => {
  it('allows exactly the tools whose confirmation class is none', () => {
    for (const tool of TOOLS) expect(isReadOnlyTool(tool), tool).toBe(TOOL_META[tool].confirmation === 'none');
  });

  it('never allows a destructive tool', () => {
    for (const tool of ['pin_rm', 'pin_update', 'mfs_rm', 'key_rm', 'repo_gc'] as const) expect(isReadOnlyTool(tool)).toBe(false);
  });
});
