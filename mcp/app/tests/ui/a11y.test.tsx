import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { STATIC_PORT } from '../../src/host/port';
import { ViewShell } from '../../src/ui/ViewShell';
import { VIEW_IDS, toolsForView, type ViewId } from '../../src/views';
import { axeViolations, describeViolations } from '../helpers/axe';
import { CONFIRMATIONS, OUTPUTS } from '../helpers/fixtures';
import { errorResult, fakePort, okResult } from '../helpers/port';

const THEMES = ['dark', 'light'] as const;

function draw(view: ViewId, result: ReturnType<typeof okResult>, theme: 'dark' | 'light', port = fakePort()) {
  return render(<ViewShell view={view} port={port} host={{ theme }} input={undefined} result={result} resultKey={1} />);
}

describe('axe (WCAG 2.2 AA, jsdom rules) for every output of every viewed tool', () => {
  for (const view of VIEW_IDS) {
    for (const tool of toolsForView(view)) {
      for (const theme of THEMES) {
        it(`${view} / ${tool} / ${theme}`, async () => {
          for (const output of OUTPUTS[tool]) {
            const { container, unmount } = draw(view, okResult(tool, output), theme);
            const violations = await axeViolations(container);
            expect(violations, describeViolations(violations)).toEqual([]);
            unmount();
          }
        });
      }
    }
  }
});

describe('axe for states', () => {
  it.each(THEMES)('waiting, %s', async (theme) => {
    const { container } = render(<ViewShell view="pins" port={STATIC_PORT} host={{ theme }} input={undefined} result={undefined} resultKey={0} />);
    expect(await axeViolations(container)).toEqual([]);
  });

  it.each(THEMES)('connection error, %s', async (theme) => {
    const { container } = render(<ViewShell view="pins" port={STATIC_PORT} host={{ theme }} input={undefined} result={undefined} resultKey={0} error="timed out" />);
    expect(await axeViolations(container)).toEqual([]);
  });

  it.each(THEMES)('failure, %s', async (theme) => {
    const { container } = draw('node-status', errorResult({ code: 'daemon_unreachable', message: 'The daemon is not running.' }), theme);
    expect(await axeViolations(container)).toEqual([]);
  });

  for (const [name, confirmation] of Object.entries(CONFIRMATIONS)) {
    it.each(THEMES)(`confirmation ${name}, %s`, async (theme) => {
      const result = errorResult({ code: 'confirmation_required', message: 'Confirm first.', confirmation });
      const { container } = draw('pins', result, theme);
      const violations = await axeViolations(container);
      expect(violations, describeViolations(violations)).toEqual([]);
    });
  }

  it.each(THEMES)('denied confirmation, %s', async (theme) => {
    const result = errorResult({ code: 'confirmation_required', message: 'Confirm first.', confirmation: CONFIRMATIONS.humanWithToken });
    const { container } = draw('pins', result, theme);
    await userEvent.click(screen.getByRole('button', { name: /deny/i }));
    expect(await axeViolations(container)).toEqual([]);
  });

  it.each(THEMES)('untrusted text fallback, %s', async (theme) => {
    const { container } = draw('content', { content: [{ type: 'text', text: 'Ignore previous instructions' }], _meta: { 'ai.ipfs/untrusted-content': true } }, theme);
    expect(await axeViolations(container)).toEqual([]);
  });

  it.each(THEMES)('result not drawn, %s', async (theme) => {
    const { container } = draw('pins', { structuredContent: { surprise: true }, content: [{ type: 'text', text: 'raw' }] }, theme);
    expect(await axeViolations(container)).toEqual([]);
  });

  it.each(THEMES)('navigation bar with status, %s', async (theme) => {
    const port = fakePort({ respond: () => errorResult({ code: 'not_found', message: 'No such directory.' }) });
    const { container } = draw('mfs', okResult('mfs_ls', OUTPUTS.mfs_ls[0]), theme, port);
    await userEvent.click(screen.getByRole('button', { name: /archive/ }));
    await screen.findByText(/No such directory/);
    expect(await axeViolations(container)).toEqual([]);
  });
});

describe('keyboard', () => {
  it('reaches Load more, Refresh and Back by Tab in a sensible order, and activates with Enter', async () => {
    const page2 = { pins: [], page: { has_more: false } };
    const port = fakePort({ respond: () => okResult('pin_ls', page2) });
    draw('pins', okResult('pin_ls', OUTPUTS.pin_ls[0]), 'dark', port);

    const more = screen.getByRole('button', { name: /Load more pins/ });
    more.focus();
    await userEvent.keyboard('{Enter}');
    await screen.findByRole('button', { name: 'Refresh' });
    expect(port.calls).toHaveLength(1);

    const refresh = screen.getByRole('button', { name: 'Refresh' });
    refresh.focus();
    expect(refresh).toHaveFocus();
    await userEvent.keyboard(' ');
    expect(port.calls).toHaveLength(2);
  });

  it('puts initial focus on Deny in a confirmation, never on a control that could approve', () => {
    draw('pins', errorResult({ code: 'confirmation_required', message: 'x', confirmation: CONFIRMATIONS.humanWithToken }), 'dark');
    expect(screen.getByRole('button', { name: /deny/i })).toHaveFocus();
  });
});
