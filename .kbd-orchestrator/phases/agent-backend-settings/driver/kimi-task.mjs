#!/usr/bin/env node
// Kimi Code task driver for phase agent-backend-settings (plan.md section 10,
// WK-1 and PR-K4). Runs one builder task as a non-interactive Kimi session with
// no push, GitHub, cluster or registry credentials, then rejects the result if
// the session ran an outward action or a remote ref moved.
//
// Usage:
//   node driver/kimi-task.mjs --change <id> --task <n.m> --role <role-id>
//     [--cwd <repo>] [--add-dir <dir>]... [--timeout <seconds>] [--kbd]
//     [--prompt-extra <text>] [--dry-run]
//
// --kbd wraps the session in kbd-apply begin-task / end-task; end-task runs only
// when the session succeeded and passed the checks.
//
// Exit codes: 0 ok, 2 usage, 3 outward action detected, 4 remote ref moved,
// 5 Kimi failed or timed out, 6 kbd-apply failed.

import { spawn, spawnSync } from 'node:child_process';
import { createWriteStream, existsSync, mkdirSync, mkdtempSync, readFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const KUBO = '/Users/gqadonis/Projects/references/kubo';
const PHASE = 'agent-backend-settings';
const PLAN = `${KUBO}/.kbd-orchestrator/phases/${PHASE}/plan.md`;
const MODEL = 'kimi-code/k3';
const DEFAULT_TIMEOUT_S = 3600;
const HERE = dirname(fileURLToPath(import.meta.url));
const LOG_DIR = join(HERE, 'logs');

// Outward or privileged actions a builder session must never take (plan W2).
const FORBIDDEN = [
  { name: 'git push', re: /\bgit\b[^|;&]*\bpush\b/ },
  { name: 'git remote/credential change', re: /\bgit\b[^|;&]*\b(remote\s+(add|set-url)|config\b[^|;&]*credential)/ },
  { name: 'gh write', re: /\bgh\s+(pr|issue|release|repo|gist|secret|workflow|label)\b/ },
  { name: 'gh api write', re: /\bgh\s+api\b[^|;&]*(-X\s*|--method\s+)(POST|PUT|PATCH|DELETE)/i },
  { name: 'kubectl', re: /\bkubectl\b/ },
  { name: 'argocd', re: /\bargocd\b/ },
  { name: 'helm', re: /\bhelm\b/ },
  { name: 'docker push/login', re: /\bdocker\b[^|;&]*\b(push|login)\b/ },
  { name: 'kimi config change', re: /\.kimi-code\/config\.toml|\.kimi\/config\.toml/ },
  {
    name: 'non-loopback mutating curl',
    re: /\bcurl\b(?=[^|;&]*(-X\s*(POST|PUT|PATCH|DELETE)|--data|-d\s|--upload-file|-T\s))(?![^|;&]*(127\.0\.0\.1|localhost|\[::1\]))/i,
  },
];

function parseArgs(argv) {
  const o = { addDirs: [], timeout: DEFAULT_TIMEOUT_S, cwd: KUBO, kbd: false, dryRun: false, extra: '' };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      if (i + 1 >= argv.length) usage(`${a} needs a value`);
      return argv[++i];
    };
    if (a === '--change') o.change = next();
    else if (a === '--task') o.task = next();
    else if (a === '--role') o.role = next();
    else if (a === '--cwd') o.cwd = resolve(next());
    else if (a === '--add-dir') o.addDirs.push(resolve(next()));
    else if (a === '--timeout') o.timeout = Number(next());
    else if (a === '--prompt-extra') o.extra = next();
    else if (a === '--kbd') o.kbd = true;
    else if (a === '--dry-run') o.dryRun = true;
    else usage(`unknown argument ${a}`);
  }
  if (!o.change || !o.task || !o.role) usage('--change, --task and --role are required');
  if (!Number.isFinite(o.timeout) || o.timeout <= 0) usage('--timeout must be a positive number');
  return o;
}

function usage(msg) {
  console.error(`kimi-task: ${msg}`);
  process.exit(2);
}

function readTasks(change) {
  const f = `${KUBO}/openspec/changes/${change}/tasks.md`;
  if (!existsSync(f)) usage(`no tasks.md for change ${change}`);
  return readFileSync(f, 'utf8')
    .split('\n')
    .map((l) => l.match(/^- \[[ x/]\] (\d+\.\d+) (.*)$/))
    .filter(Boolean)
    .map((m) => ({ id: m[1], title: m[2] }));
}

function hardenedEnv() {
  const empty = mkdtempSync(join(tmpdir(), 'kimi-task-nocreds-'));
  const env = { ...process.env };
  for (const k of ['GH_TOKEN', 'GITHUB_TOKEN', 'GH_ENTERPRISE_TOKEN', 'GITHUB_ENTERPRISE_TOKEN', 'SSH_AUTH_SOCK']) delete env[k];
  Object.assign(env, {
    GH_CONFIG_DIR: empty,
    KUBECONFIG: '/dev/null',
    DOCKER_CONFIG: empty,
    GIT_CONFIG_COUNT: '1',
    GIT_CONFIG_KEY_0: 'credential.helper',
    GIT_CONFIG_VALUE_0: '',
    GIT_SSH_COMMAND: 'false',
    GIT_TERMINAL_PROMPT: '0',
  });
  return env;
}

function remoteRefs(dir) {
  const r = spawnSync('git', ['-C', dir, 'ls-remote', 'origin'], { encoding: 'utf8', timeout: 30000, stdio: ['ignore', 'pipe', 'pipe'] });
  return r.status === 0 ? r.stdout.trim() : null;
}

function kbdApply(verb, o, idx, total, title) {
  const candidates = [`${homedir()}/.kimi-code/skills/kbd-apply/kbd-apply.sh`, `${homedir()}/.claude/skills/kbd-apply/kbd-apply.sh`];
  const apply = candidates.find((p) => existsSync(p));
  if (!apply) return { ok: false, out: 'kbd-apply.sh not found' };
  const r = spawnSync('bash', [apply, verb, o.change, o.task, String(idx), String(total), title], {
    cwd: KUBO, encoding: 'utf8', timeout: 120000, stdio: ['ignore', 'pipe', 'pipe'],
  });
  return { ok: r.status === 0, out: `${r.stdout ?? ''}${r.stderr ?? ''}`.trim() };
}

function buildPrompt(o, title) {
  return [
    `You are the ${o.role} role of the kubo-ipfs-agentic team, executing one KBD task.`,
    `Phase ${PHASE}. Plan: ${PLAN} (read sections 1, 2, 4 and the row for this task in section 10).`,
    `Change: ${o.change} (openspec/changes/${o.change}/). Task ${o.task}: ${title}`,
    `Read AGENTS.md and .prometheus/gotchas.md first. Do only this task and meet its acceptance check.`,
    `Edit only paths your role owns in .agent-team/kubo-ipfs-agentic/team.json.`,
    `Do not run git push, gh, kubectl, argocd, helm, docker push or remote mutating requests; do not publish, deploy, tag or merge.`,
    `Do not tick checkboxes in tasks.md; the driver records task state.`,
    `Shell: run long commands under timeout with stdin closed; no large here-documents; never pkill.`,
    o.extra,
    `Finish with: changed files, commands run with their real output, the acceptance result, and anything unverified.`,
  ].filter(Boolean).join('\n');
}

function scan(logPath) {
  const hits = [];
  let finalText = '';
  for (const line of readFileSync(logPath, 'utf8').split('\n')) {
    if (!line.trim()) continue;
    let ev;
    try { ev = JSON.parse(line); } catch { continue; }
    if (ev.role === 'assistant' && typeof ev.content === 'string') finalText = ev.content;
    for (const call of ev.tool_calls ?? []) {
      let args = {};
      try { args = JSON.parse(call.function?.arguments ?? '{}'); } catch { /* keep empty */ }
      const cmd = typeof args.command === 'string' ? args.command : '';
      for (const f of FORBIDDEN) if (cmd && f.re.test(cmd)) hits.push({ rule: f.name, command: cmd.slice(0, 300) });
    }
  }
  return { hits, finalText };
}

async function main() {
  const o = parseArgs(process.argv.slice(2));
  const tasks = readTasks(o.change);
  const idx = tasks.findIndex((t) => t.id === o.task);
  if (idx < 0) usage(`task ${o.task} not in ${o.change}/tasks.md`);
  const title = tasks[idx].title;
  const agentFile = `${KUBO}/.kimi-code/agents/${o.role}.md`;
  if (!existsSync(agentFile)) usage(`no Kimi role profile ${agentFile}`);

  const prompt = buildPrompt(o, title);
  const args = ['--agent-file', agentFile, '-m', MODEL, '-p', prompt, '--output-format', 'stream-json'];
  for (const d of o.addDirs) args.push('--add-dir', d);
  if (o.dryRun) {
    console.log(JSON.stringify({ cwd: o.cwd, args: args.map((a) => (a === prompt ? '<prompt>' : a)), prompt }, null, 2));
    return 0;
  }

  mkdirSync(LOG_DIR, { recursive: true });
  const stamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d+Z$/, 'Z');
  const logPath = join(LOG_DIR, `${o.change}-${o.task}-${stamp}.jsonl`);
  const repos = [o.cwd, ...o.addDirs].filter((d) => existsSync(join(d, '.git')) || existsSync(d));
  const before = Object.fromEntries(repos.map((d) => [d, remoteRefs(d)]));

  if (o.kbd) {
    const b = kbdApply('begin-task', o, idx + 1, tasks.length, title);
    if (!b.ok) { console.error(`kbd-apply begin-task failed:\n${b.out}`); return 6; }
  }

  console.log(`kimi-task: ${o.change}/${o.task} as ${o.role} in ${o.cwd}; log ${logPath}`);
  const code = await new Promise((res) => {
    const child = spawn('kimi', args, { cwd: o.cwd, env: hardenedEnv(), stdio: ['ignore', 'pipe', 'pipe'] });
    const out = createWriteStream(logPath);
    const err = createWriteStream(`${logPath}.stderr`);
    child.stdout.pipe(out);
    child.stderr.pipe(err);
    const timer = setTimeout(() => { child.kill('SIGTERM'); }, o.timeout * 1000);
    child.on('close', (c, sig) => { clearTimeout(timer); res(sig ? 124 : c); });
  });

  const { hits, finalText } = scan(logPath);
  const moved = repos.filter((d) => before[d] !== null && remoteRefs(d) !== before[d]);
  const status = spawnSync('git', ['-C', o.cwd, 'status', '--porcelain'], { encoding: 'utf8' }).stdout.trim();

  console.log(`--- final message ---\n${finalText}\n--- changed files in ${o.cwd} ---\n${status || '(none)'}`);
  let rc = 0;
  if (hits.length) { console.error(`REJECTED: outward actions detected:\n${JSON.stringify(hits, null, 2)}`); rc = 3; }
  else if (moved.length) { console.error(`REJECTED: remote refs changed in ${moved.join(', ')}`); rc = 4; }
  else if (code !== 0) { console.error(`Kimi exited ${code}${code === 124 ? ' (timeout)' : ''}; see ${logPath}.stderr`); rc = 5; }

  if (o.kbd && rc === 0) {
    const e = kbdApply('end-task', o, idx + 1, tasks.length, title);
    if (!e.ok) { console.error(`kbd-apply end-task failed:\n${e.out}`); return 6; }
  }
  if (rc !== 0) console.error('Task left open; the operator decides. Nothing was reverted.');
  return rc;
}

process.exitCode = await main();
