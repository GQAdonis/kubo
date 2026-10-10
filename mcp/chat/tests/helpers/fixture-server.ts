import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';
import { isDeepStrictEqual } from 'node:util';
import { loadFixtures, sseText, type AguiFixture } from './fixtures';

// A local stand-in for `ipfs mcp` /agui, built from the recorded contract fixtures. It listens on a
// free port the OS picks (never a hardcoded one: the operator's port rule), and answers a POST only
// when the request body equals a fixture's `input` exactly. That makes every replay also a check of the
// island's wire format against the contract, not just of its event handling.
//
// It is not `ipfs mcp`. It does not run a model, a tool or a confirmation; the live run is the
// webui-agent-chat change, after mcp-agui-endpoint lands /agui.

export interface RecordedRequest {
  readonly method: string;
  readonly url: string;
  readonly headers: IncomingMessage['headers'];
  readonly body: unknown;
  readonly matched: string | undefined;
}

export interface FixtureServerOptions {
  /** The bearer the server accepts. */
  readonly token: string;
  /** Page origins the server allows (CORS and the allowlist). Default: any origin is refused with 403 when an Origin header is sent. */
  readonly allowedOrigins?: readonly string[];
  readonly fixtures?: readonly AguiFixture[];
  /** Write the stream in chunks of this many bytes, to exercise frames split across reads. */
  readonly fragmentBytes?: number;
  /** Insert an SSE comment keep-alive between events. */
  readonly keepAlive?: boolean;
  /** Delay between events, ms. */
  readonly eventDelayMs?: number;
  /**
   * 'exact' (default): the body must equal a fixture input, ids included. 'conversation': for browser
   * tests, where the island's thread, run and message ids are random. A run is matched by its last user
   * message text, a resume by the interrupt id it answers; the response echoes the request's thread and
   * run ids. The wire shape is still asserted by the exact mode in tests/unit/replay.test.ts.
   */
  readonly matching?: 'exact' | 'conversation';
  /**
   * Serve GET /__test/requests (the recorded requests as JSON) and DELETE /__test/requests (clear them),
   * for browser specs whose workers cannot see this process's memory.
   */
  readonly controlEndpoint?: boolean;
  /**
   * A `{wait: true}` resume opens a run that, with a real system prompt, ends only when a person
   * decides. When set, the server answers it with RUN_STARTED and keep-alives and holds the stream open
   * until the client disconnects, so a browser test can see the "Waiting for system prompt" state and
   * deny while it is open. Without it, the matched wait fixture is replayed whole.
   */
  readonly holdWait?: boolean;
}

export interface FixtureServer {
  readonly origin: string;
  readonly endpoint: string;
  readonly requests: RecordedRequest[];
  /** Allow one more page origin (CORS and the allowlist). Browser tests learn the page origin after the server starts. */
  allow(origin: string): void;
  close(): Promise<void>;
}

type Payload = { approved?: boolean; wait?: boolean };
type Body = { threadId?: string; runId?: string; messages?: { role?: string; content?: string }[]; resume?: { interruptId?: string; payload?: Payload }[] };

const isWait = (body: unknown): boolean => ((body ?? {}) as Body).resume?.some((r) => r.payload?.wait === true) === true;

function matchConversation(fixtures: readonly AguiFixture[], body: unknown): AguiFixture | undefined {
  const b = (body ?? {}) as Body;
  const answering = b.resume?.[0];
  if (answering) {
    // Same interrupt and the same payload ({approved: true}, {approved: false} or {wait: true}). Several
    // wait fixtures share one input (approved, denied, expired); the first one in name order answers.
    return fixtures.find((f) => f.input.resume?.some((r) => r['interruptId'] === answering.interruptId && isDeepStrictEqual(r['payload'], answering.payload)));
  }
  const last = [...(b.messages ?? [])].reverse().find((m) => m.role === 'user')?.content;
  return fixtures.find((f) => !f.input.resume && f.input.messages.some((m) => m['role'] === 'user' && m['content'] === last));
}

/** The fixture's events with the request's thread and run ids, so the island's state stays consistent. */
function echoIds(matched: AguiFixture, body: unknown): readonly AguiFixture['events'][number][] {
  const b = (body ?? {}) as Body;
  return matched.events.map((e) =>
    e.type === 'RUN_STARTED' || e.type === 'RUN_FINISHED' ? { ...e, ...(b.threadId && { threadId: b.threadId }), ...(b.runId && { runId: b.runId }) } : e,
  );
}

async function readBody(req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString('utf8');
}

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export async function startFixtureServer(options: FixtureServerOptions): Promise<FixtureServer> {
  const fixtures = options.fixtures ?? loadFixtures();
  const allowed = new Set(options.allowedOrigins ?? []);
  const requests: RecordedRequest[] = [];

  const cors = (req: IncomingMessage, res: ServerResponse): boolean => {
    const origin = req.headers.origin;
    if (origin === undefined) return true; // not a browser: no origin to judge
    if (!allowed.has(origin)) return false;
    res.setHeader('access-control-allow-origin', origin);
    res.setHeader('vary', 'Origin');
    res.setHeader('access-control-allow-headers', 'authorization, content-type');
    res.setHeader('access-control-allow-methods', 'POST, OPTIONS');
    return true;
  };

  const server: Server = createServer(async (req, res) => {
    try {
      if (options.controlEndpoint && req.url === '/__test/requests') {
        if (req.method === 'DELETE') requests.length = 0;
        res.writeHead(200, { 'content-type': 'application/json' }).end(JSON.stringify(requests));
        return;
      }
      if (!cors(req, res)) {
        res.writeHead(403, { 'content-type': 'application/json' }).end('{"error":"origin not allowed"}');
        return;
      }
      if (req.method === 'OPTIONS') {
        res.writeHead(204).end();
        return;
      }
      const url = req.url ?? '';
      if (req.method !== 'POST' || url !== '/agui') {
        requests.push({ method: req.method ?? '', url, headers: req.headers, body: undefined, matched: undefined });
        res.writeHead(404, { 'content-type': 'application/json' }).end('{"error":"not mounted"}');
        return;
      }
      const text = await readBody(req);
      let body: unknown;
      try {
        body = JSON.parse(text);
      } catch {
        body = text;
      }
      const matched = options.matching === 'conversation' ? matchConversation(fixtures, body) : fixtures.find((f) => isDeepStrictEqual(f.input, body));
      requests.push({ method: 'POST', url, headers: req.headers, body, matched: matched?.name });

      if (req.headers.authorization !== `Bearer ${options.token}`) {
        res.writeHead(401, { 'content-type': 'application/json' }).end('{"error":"unauthorized"}');
        return;
      }
      if (!matched) {
        res.writeHead(422, { 'content-type': 'application/json' }).end('{"error":"request body matches no recorded fixture input"}');
        return;
      }

      res.writeHead(200, { 'content-type': 'text/event-stream; charset=utf-8', 'cache-control': 'no-store' });
      const all = options.matching === 'conversation' ? echoIds(matched, body) : matched.events;
      const hold = options.holdWait === true && isWait(body);
      const events = hold ? all.slice(0, 1) : all;
      for (const [i, event] of events.entries()) {
        if (options.keepAlive && i > 0) res.write(': keep-alive\n\n');
        const frame = sseText([event]);
        if (options.fragmentBytes) {
          for (let o = 0; o < frame.length; o += options.fragmentBytes) {
            res.write(frame.slice(o, o + options.fragmentBytes));
            await sleep(1);
          }
        } else res.write(frame);
        if (options.eventDelayMs) await sleep(options.eventDelayMs);
      }
      if (hold) {
        // The system prompt has not been answered: keep the run open, as the server does, until the client leaves.
        const timer = setInterval(() => res.write(': keep-alive\n\n'), 1000);
        res.on('close', () => clearInterval(timer));
        return;
      }
      res.end();
    } catch {
      if (!res.headersSent) res.writeHead(500);
      res.end();
    }
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address() as AddressInfo;
  const origin = `http://127.0.0.1:${port}`;
  return {
    origin,
    endpoint: `${origin}/agui`,
    requests,
    allow: (o) => void allowed.add(o),
    close: () =>
      new Promise<void>((resolve) => {
        server.closeAllConnections();
        server.close(() => resolve());
      }),
  };
}
