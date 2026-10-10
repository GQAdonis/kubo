import { readFile } from 'node:fs/promises';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { extname, resolve } from 'node:path';

// A static server for the browser tests: the stand-in Web UI page, Tachyons, and the harness build of
// the island (dist-harness, shadow root open so Playwright can pierce it). It listens on a free port
// the OS chooses. It records every request path so a spec can prove what a page did and did not load.

const ROOT = resolve(import.meta.dirname, '../..');

const TYPES: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.map': 'application/json',
};

export interface HarnessConfig {
  /** The ipfs mcp /agui URL the page hands to the element. */
  readonly endpoint: string;
  readonly token: string;
  /** Origins the CSP's connect-src names. */
  readonly connectOrigins: readonly string[];
}

export interface HarnessServer {
  readonly origin: string;
  readonly requests: string[];
  close(): Promise<void>;
}

/** The deny-by-default policy ADR-0005 proposes for Desktop's webui://localhost, adapted to a loopback origin. */
export function strictCsp(connectOrigins: readonly string[]): string {
  return [
    "default-src 'none'",
    "script-src 'self'", // no 'unsafe-eval', no 'unsafe-inline'
    "style-src 'self'", // no inline <style>: the island adopts constructable stylesheets
    "img-src 'self' data:",
    `connect-src ${connectOrigins.join(' ')}`,
    "base-uri 'none'",
    "form-action 'none'",
    "frame-src 'none'",
    "object-src 'none'",
  ].join('; ');
}

export async function startHarnessServer(config: HarnessConfig): Promise<HarnessServer> {
  const requests: string[] = [];
  const server: Server = createServer(async (req, res) => {
    const url = new URL(req.url ?? '/', 'http://harness.invalid');
    requests.push(url.pathname);
    try {
      let file: string | undefined;
      if (url.pathname === '/host.html' || url.pathname === '/') {
        const island = url.searchParams.get('island') ?? 'none';
        const theme = url.searchParams.get('theme') ?? 'auto';
        const html = (await readFile(resolve(ROOT, 'harness/host.html'), 'utf8')).replace(
          '{{CONFIG}}',
          JSON.stringify({ island, theme, endpoint: config.endpoint, token: config.token }),
        );
        const headers: Record<string, string> = { 'content-type': TYPES['.html'] as string, 'cache-control': 'no-store' };
        if (url.searchParams.get('csp') === 'strict') headers['content-security-policy'] = strictCsp(config.connectOrigins);
        res.writeHead(200, headers).end(html);
        return;
      }
      if (url.pathname === '/host.js') file = resolve(ROOT, 'harness/host.js');
      else if (url.pathname === '/host.css') file = resolve(ROOT, 'harness/host.css');
      else if (url.pathname === '/vendor/tachyons.css') file = resolve(ROOT, 'node_modules/tachyons/css/tachyons.min.css');
      else if (url.pathname.startsWith('/island/')) {
        const rel = url.pathname.slice('/island/'.length);
        if (rel.includes('..')) throw new Error('bad path');
        file = resolve(ROOT, 'dist-harness', rel);
      }
      if (!file) {
        res.writeHead(404).end();
        return;
      }
      const body = await readFile(file);
      res.writeHead(200, { 'content-type': TYPES[extname(file)] ?? 'application/octet-stream', 'cache-control': 'no-store' }).end(body);
    } catch {
      res.writeHead(404).end();
    }
  });
  await new Promise<void>((r) => server.listen(0, '127.0.0.1', r));
  const { port } = server.address() as AddressInfo;
  return {
    origin: `http://127.0.0.1:${port}`,
    requests,
    close: () =>
      new Promise<void>((r) => {
        server.closeAllConnections();
        server.close(() => r());
      }),
  };
}
