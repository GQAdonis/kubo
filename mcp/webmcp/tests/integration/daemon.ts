import { spawn, spawnSync, type ChildProcess } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

// A real `ipfs` daemon in a temporary repo. The `test` init profile binds API, gateway
// and swarm to 127.0.0.1 on OS-assigned ports, so the default ports 4001, 5001 and 8080
// (and 4000) are never used, and nothing leaves the machine (no bootstrap, no mDNS,
// no autoconf). The daemon is stopped by its own PID; nothing is killed by name.

const FORBIDDEN_PORTS = new Set([4000, 4001, 5001, 8080]);

export function findIpfsBinary(): string {
  if (process.env.IPFS_BIN) return process.env.IPFS_BIN;
  // The repository build (`make build`) first, then PATH.
  const built = resolve(import.meta.dirname, '../../../../cmd/ipfs/ipfs');
  return existsSync(built) ? built : 'ipfs';
}

export interface Daemon {
  url: string;
  repo: string;
  bin: string;
  pid: number;
  stop: () => Promise<void>;
}

export async function startDaemon(): Promise<Daemon> {
  const bin = findIpfsBinary();
  const repo = mkdtempSync(join(tmpdir(), 'webmcp-ipfs-'));
  const env = { ...process.env, IPFS_PATH: repo, IPFS_TELEMETRY: 'off', DO_NOT_TRACK: '1' };
  const init = spawnSync(bin, ['init', '--profile=test'], { env, encoding: 'utf8' });
  if (init.status !== 0) {
    rmSync(repo, { recursive: true, force: true });
    throw new Error(`ipfs init failed: ${init.stderr || init.error?.message}`);
  }
  const child: ChildProcess = spawn(bin, ['daemon'], { env, stdio: ['ignore', 'pipe', 'pipe'] });
  const pid = child.pid;
  if (!pid) throw new Error('daemon did not start');
  let log = '';
  await new Promise<void>((ok, fail) => {
    const timer = setTimeout(() => fail(new Error(`daemon not ready in 90s:\n${log}`)), 90_000);
    const onData = (d: Buffer) => {
      log += d.toString();
      if (log.includes('Daemon is ready')) {
        clearTimeout(timer);
        ok();
      }
    };
    child.stdout!.on('data', onData);
    child.stderr!.on('data', onData);
    child.on('exit', (code) => {
      clearTimeout(timer);
      fail(new Error(`daemon exited early (${code}):\n${log}`));
    });
  });
  // The api file holds the multiaddr the daemon actually bound.
  const api = readFileSync(join(repo, 'api'), 'utf8').trim();
  const m = /^\/ip4\/(127\.0\.0\.1)\/tcp\/(\d+)$/.exec(api);
  if (!m) throw new Error(`unexpected API address ${api}`);
  const port = Number(m[2]);
  if (FORBIDDEN_PORTS.has(port)) throw new Error(`daemon bound a forbidden port ${port}`);
  return {
    url: `http://127.0.0.1:${port}`,
    repo,
    bin,
    pid,
    async stop() {
      const exited = new Promise<void>((done) => child.once('exit', () => done()));
      process.kill(pid, 'SIGTERM');
      const t = setTimeout(() => process.kill(pid, 'SIGKILL'), 15_000);
      await exited;
      clearTimeout(t);
      rmSync(repo, { recursive: true, force: true });
    },
  };
}
