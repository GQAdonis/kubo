import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApprovalCard } from '../../src/chat/ApprovalCard';
import { EgressBadge } from '../../src/chat/EgressBadge';
import { MessageStream } from '../../src/chat/MessageStream';
import { ToolCallCard } from '../../src/chat/ToolCallCard';
import { UiScope } from '../../src/scope/UiScope';
import { UI_ROOT } from '../helpers/contract';
import { OUTPUTS } from '../fixtures/outputs';

// The chat island mounts these inside a shadow root next to Tachyons pages (ADR-0005). These tests
// mount into a real ShadowRoot and check the three ways a component usually breaks there: styles
// that live in document.head, ids that cross the root, and code that reaches for document/window.

let mounted: { root: Root; host: HTMLElement } | undefined;

function mountInShadow(node: React.ReactNode) {
  const host = document.createElement('div');
  document.body.append(host);
  const shadow = host.attachShadow({ mode: 'open' });
  const mountPoint = document.createElement('div');
  shadow.append(mountPoint);
  const root = createRoot(mountPoint);
  act(() => root.render(<UiScope>{node}</UiScope>));
  mounted = { root, host };
  return { host, shadow, mountPoint };
}

afterEach(() => {
  if (mounted) {
    act(() => mounted?.root.unmount());
    mounted.host.remove();
    mounted = undefined;
  }
});

const interrupt = {
  id: 'int-cf_Zr4Yt1nB6kq0HdUe',
  reason: 'tool_call' as const,
  message: 'Pin bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku (recursive=true)',
  toolCallId: 'tc-7',
  expiresAt: '2026-10-09T12:02:00Z',
  metadata: { 'ipfs-mcp': { confirmation_id: 'cf_Zr4Yt1nB6kq0HdUe', tool: 'pin_add', class: 'write' as const, requires_native_prompt: false } },
};

describe('chat primitives inside a shadow root', () => {
  it('carry their own styles inside the root, and add nothing to the document', () => {
    const { shadow } = mountInShadow(
      <>
        <MessageStream messages={[{ id: 'a', role: 'assistant', content: 'Hello' }]} />
        <EgressBadge tool="ipfs_cat" acknowledged_hosts={['model.example.net']} />
      </>,
    );
    const styles = shadow.querySelectorAll('style[data-ipfs-ui-styles]');
    expect(styles).toHaveLength(1);
    expect(styles[0]?.textContent).toContain('[data-ipfs-ui]');
    // React 19 hoists <style href precedence> to document.head; a plain <style> must not be hoisted.
    expect(document.head.querySelector('style[data-ipfs-ui-styles]')).toBeNull();
    expect(document.querySelector('[data-ipfs-ui]')).toBeNull();
    expect(shadow.querySelector('[data-ipfs-ui]')).not.toBeNull();
  });

  it('keeps one live region per scope, inside the root', () => {
    const { shadow } = mountInShadow(
      <>
        <MessageStream messages={[{ id: 'a', role: 'assistant', content: 'Hello' }]} />
        <ToolCallCard call={{ id: 't', name: 'pin_ls', arguments: '{}', status: 'done', result: JSON.stringify(OUTPUTS.pin_ls[0]) }} />
      </>,
    );
    expect(shadow.querySelectorAll('[data-ipfs-ui-live]')).toHaveLength(1);
  });

  it('wires aria-labelledby and aria-describedby within the shadow tree, and the buttons work', async () => {
    const onApprove = vi.fn();
    const { shadow } = mountInShadow(<ApprovalCard interrupt={interrupt} onApprove={onApprove} onDeny={vi.fn()} />);
    const card = shadow.querySelector('[data-phase]') as HTMLElement;
    for (const attr of ['aria-labelledby', 'aria-describedby']) {
      const id = card.getAttribute(attr) as string;
      expect(id, attr).toBeTruthy();
      // The referenced element is found in the same root, not in the document.
      expect(shadow.getElementById(id), attr).not.toBeNull();
      expect(document.getElementById(id), attr).toBeNull();
    }
    const approve = [...shadow.querySelectorAll('button')].find((b) => b.textContent === 'Approve pin') as HTMLButtonElement;
    await userEvent.click(approve);
    expect(onApprove).toHaveBeenCalledOnce();
  });

  it('shows the native-prompt state inside the root with no approve control', () => {
    const native = { ...interrupt, metadata: { 'ipfs-mcp': { ...interrupt.metadata['ipfs-mcp'], class: 'destructive' as const, requires_native_prompt: true } } };
    const { shadow } = mountInShadow(<ApprovalCard interrupt={native} onApprove={vi.fn()} onDeny={vi.fn()} />);
    expect(shadow.textContent).toContain('Waiting for system prompt');
    expect([...shadow.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['Deny']);
  });
});

describe('shadow-DOM safety, statically', () => {
  const walk = (dir: string, out: string[] = []): string[] => {
    for (const n of readdirSync(dir)) {
      const p = join(dir, n);
      if (statSync(p).isDirectory()) walk(p, out);
      else if (/\.(ts|tsx)$/.test(n)) out.push(p);
    }
    return out;
  };

  it('no component reaches for document, window or global lookups', () => {
    const files = ['chat', 'approval', 'components', 'ui', 'scope'].flatMap((d) => walk(join(UI_ROOT, 'src', d)));
    const offenders: string[] = [];
    for (const f of files) {
      const code = readFileSync(f, 'utf8')
        .replace(/\/\*[\s\S]*?\*\//g, '')
        .replace(/(^|[^:])\/\/.*$/gm, '$1');
      for (const re of [/\bdocument\./, /\bwindow\./, /getElementById\(/, /\bdocument\.activeElement/, /querySelector(All)?\(/, /addEventListener\(/]) {
        if (re.test(code)) offenders.push(`${f.replace(UI_ROOT, '')}: ${re}`);
      }
    }
    expect(offenders).toEqual([]);
  });

  it('every stylesheet rule is keyed on the library root or an iu- class', () => {
    const css = ['tokens', 'base', 'components', 'views', 'chat']
      .map((f) => readFileSync(join(UI_ROOT, 'src', 'styles', `${f}.css`), 'utf8'))
      .join('\n')
      .replace(/\/\*[\s\S]*?\*\//g, '');
    const selectors: string[] = [];
    // crude but sufficient: take the text before each "{" that is not an at-rule or inside keyframes
    for (const m of css.matchAll(/([^{}]+)\{/g)) {
      const head = (m[1] as string).trim();
      if (!head || head.startsWith('@')) continue;
      let depth = 0;
      let cur = '';
      for (const ch of head) {
        if (ch === '(') depth++;
        if (ch === ')') depth--;
        if (ch === ',' && depth === 0) {
          selectors.push(cur.trim());
          cur = '';
        } else cur += ch;
      }
      selectors.push(cur.trim());
    }
    expect(selectors.length).toBeGreaterThan(50);
    const bad = selectors.filter(
      (s) => !(s.includes('[data-ipfs-ui]') || /\.iu-/.test(s) || /^(th|td)\.iu-/.test(s)) || /(^|\s)(html|body|:root)\b/.test(s),
    );
    expect(bad).toEqual([]);
  });
});
