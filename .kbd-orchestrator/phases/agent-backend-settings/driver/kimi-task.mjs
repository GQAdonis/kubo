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
// 5 Kimi failed or timed out, 6 kbd-apply failed, 7 acceptance reported not met.

import { spawn, spawnSync } from 'node:child_process';
import { createWriteStream, existsSync, mkdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  KUBO, LOG_DIR, MODEL, acceptanceNotMet, beginTask, buildPrompt, forbiddenRules, hardenedEnv,
  kbdApply, parseArgs, readTasks, remoteRefs, usage,
} from './common.mjs';

const TOOL = 'kimi-task';

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
      for (const rule of forbiddenRules(cmd)) hits.push({ rule, command: cmd.slice(0, 300) });
    }
  }
  return { hits, finalText };
}

async function main() {
  const o = parseArgs(TOOL, process.argv.slice(2));
  const tasks = readTasks(TOOL, o.change);
  const idx = tasks.findIndex((t) => t.id === o.task);
  if (idx < 0) usage(TOOL, `task ${o.task} not in ${o.change}/tasks.md`);
  const title = tasks[idx].title;
  const agentFile = `${KUBO}/.kimi-code/agents/${o.role}.md`;
  if (!existsSync(agentFile)) usage(TOOL, `no Kimi role profile ${agentFile}`);

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
    const b = beginTask(o, idx + 1, tasks.length, title);
    if (!b.ok) { console.error(`kbd-apply begin-task failed:\n${b.out}`); return 6; }
    if (b.resumed) console.log(`task ${o.change}/${o.task} was already started; resuming it`);
  }

  console.log(`${TOOL}: ${o.change}/${o.task} as ${o.role} in ${o.cwd}; log ${logPath}`);
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
  else if (acceptanceNotMet(finalText)) {
    // A clean exit is not completion: mcp-agui-a2ui-registry 1.1 (2026-10-13)
    // reported "Not met — blocked" and was still closed by end-task.
    console.error('REJECTED: the session reported its acceptance check as not met or blocked');
    rc = 7;
  }

  if (o.kbd && rc === 0) {
    const e = kbdApply('end-task', o, idx + 1, tasks.length, title);
    if (!e.ok) { console.error(`kbd-apply end-task failed:\n${e.out}`); return 6; }
  }
  if (rc !== 0) console.error('Task left open; the operator decides. Nothing was reverted.');
  return rc;
}

process.exitCode = await main();
