#!/usr/bin/env node
// Kimi Code task driver over the Agent Client Protocol (ACP v1, `kimi acp` on
// stdio). Same task contract as kimi-task.mjs, with three differences:
//
//   1. Every tool call that Kimi asks permission for is checked against the
//      FORBIDDEN rules before it runs; a match is rejected, never executed.
//   2. Session events stream live: one compact line per tool call on stdout,
//      the full JSON-RPC traffic in the log.
//   3. A session that ends its turn with a line starting "QUESTION:" is paused:
//      the question goes to <log>.question.md and the driver waits for
//      <log>.answer.md (written by the lead), then continues the same session.
//
// Operator decision 2026-10-13 ("yes, build the ACP driver after Round 2");
// kimi-task.mjs stays the default until this one has been trialled.
//
// Usage:
//   node driver/kimi-acp.mjs --change <id> --task <n.m> --role <role-id>
//     [--cwd <repo>] [--add-dir <dir>]... [--timeout <seconds>] [--kbd]
//     [--prompt-extra <text>] [--answer-timeout <seconds>] [--dry-run]
//   node driver/kimi-acp.mjs --self-test --cwd <scratch-git-repo>
//
// Exit codes: as kimi-task.mjs (0 ok, 2 usage, 3 outward action executed,
// 4 remote ref moved, 5 Kimi failed or timed out, 6 kbd-apply failed,
// 7 acceptance reported not met), plus 8 question left unanswered.

import { spawn, spawnSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  KUBO, LOG_DIR, MODEL, acceptanceNotMet, buildPrompt, forbiddenRules, hardenedEnv,
  kbdApply, parseArgs, readTasks, remoteRefs, usage,
} from './common.mjs';

const TOOL = 'kimi-acp';
const ACP_PROTOCOL_VERSION = 1;
const DEFAULT_ANSWER_TIMEOUT_S = 1800;
const ANSWER_POLL_MS = 5000;
const QUESTION_RE = /^QUESTION:\s*(.+)$/m;

const QUESTION_RULE = [
  'If you are blocked on a decision you cannot make from the repository, the plan or the ADRs,',
  'end your turn with one line that starts with "QUESTION:" followed by the question, and stop.',
  'You will receive the answer in the same session. Do not guess a decision that belongs to the lead or the operator.',
].join(' ');

const SELF_TEST_PROMPT = [
  'Driver self-test in a scratch git repository. Do exactly these steps:',
  '1. Run the shell command: echo acp-ok > step1.txt',
  '2. Run the shell command: git push origin HEAD (it is expected to be refused; report the refusal).',
  '3. End your turn with exactly one line: QUESTION: What word should go in step3.txt?',
  'After the answer arrives, write that word into step3.txt with a shell command, then finish with a section',
  '"Acceptance result" that says Met if step1.txt and step3.txt exist and the push was refused.',
].join('\n');

function main() {
  const o = parseArgs(TOOL, process.argv.slice(2), {
    '--self-test': (opt) => { opt.selfTest = true; },
    '--answer-timeout': (opt, next) => { opt.answerTimeout = Number(next()); },
  });
  o.answerTimeout ??= DEFAULT_ANSWER_TIMEOUT_S;
  if (o.selfTest && o.kbd) usage(TOOL, '--self-test cannot be combined with --kbd');

  let idx = 0;
  let tasks = [];
  let title = 'self-test';
  let prompt = SELF_TEST_PROMPT;
  if (!o.selfTest) {
    tasks = readTasks(TOOL, o.change);
    idx = tasks.findIndex((t) => t.id === o.task);
    if (idx < 0) usage(TOOL, `task ${o.task} not in ${o.change}/tasks.md`);
    title = tasks[idx].title;
    const agentFile = `${KUBO}/.kimi-code/agents/${o.role}.md`;
    if (!existsSync(agentFile)) usage(TOOL, `no Kimi role profile ${agentFile}`);
    // ACP has no agent-file option, so the role definition opens the prompt.
    prompt = [readFileSync(agentFile, 'utf8'), '---', buildPrompt(o, title), QUESTION_RULE].join('\n\n');
  }
  if (o.dryRun) {
    console.log(JSON.stringify({ cwd: o.cwd, addDirs: o.addDirs, model: MODEL, prompt }, null, 2));
    return Promise.resolve(0);
  }
  return run(o, tasks, idx, title, prompt);
}

async function run(o, tasks, idx, title, prompt) {
  mkdirSync(LOG_DIR, { recursive: true });
  const stamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d+Z$/, 'Z');
  const base = o.selfTest ? `acp-self-test-${stamp}` : `${o.change}-${o.task}-acp-${stamp}`;
  const logPath = join(LOG_DIR, `${base}.jsonl`);
  const repos = [o.cwd, ...o.addDirs].filter((d) => existsSync(join(d, '.git')));
  const before = Object.fromEntries(repos.map((d) => [d, remoteRefs(d)]));

  if (o.kbd) {
    const b = kbdApply('begin-task', o, idx + 1, tasks.length, title);
    if (!b.ok) { console.error(`kbd-apply begin-task failed:\n${b.out}`); return 6; }
  }
  console.log(`${TOOL}: ${o.selfTest ? 'self-test' : `${o.change}/${o.task} as ${o.role}`} in ${o.cwd}; log ${logPath}`);

  const session = new AcpSession(o.cwd, logPath);
  const deadline = setTimeout(() => session.abort('timeout'), o.timeout * 1000);
  let rc = 0;
  try {
    rc = await converse(session, o, prompt, logPath);
  } catch (err) {
    console.error(`${TOOL}: session failed: ${err.message}`);
    rc = 5;
  } finally {
    clearTimeout(deadline);
    session.close();
  }

  const moved = repos.filter((d) => before[d] !== null && remoteRefs(d) !== before[d]);
  const status = spawnSync('git', ['-C', o.cwd, 'status', '--porcelain'], { encoding: 'utf8' }).stdout.trim();
  console.log(`--- final message ---\n${session.lastTurnText}\n--- changed files in ${o.cwd} ---\n${status || '(none)'}`);
  if (session.denied.length) console.log(`denied before running: ${JSON.stringify(session.denied, null, 2)}`);

  if (rc === 0 && session.executedForbidden.length) {
    console.error(`REJECTED: outward actions executed:\n${JSON.stringify(session.executedForbidden, null, 2)}`);
    rc = 3;
  } else if (rc === 0 && moved.length) {
    console.error(`REJECTED: remote refs changed in ${moved.join(', ')}`);
    rc = 4;
  } else if (rc === 0 && acceptanceNotMet(session.lastTurnText)) {
    console.error('REJECTED: the session reported its acceptance check as not met or blocked');
    rc = 7;
  }
  if (o.kbd && rc === 0) {
    const e = kbdApply('end-task', o, idx + 1, tasks.length, title);
    if (!e.ok) { console.error(`kbd-apply end-task failed:\n${e.out}`); return 6; }
  }
  if (rc !== 0) console.error('Task left open; the lead decides. Nothing was reverted.');
  return rc;
}

// converse runs the prompt turn and any question-and-answer turns after it.
async function converse(session, o, prompt, logPath) {
  await session.start(o.addDirs);
  let text = prompt;
  for (let round = 1; ; round++) {
    const stop = await session.prompt(text);
    if (stop !== 'end_turn') {
      console.error(`${TOOL}: turn ${round} ended with stopReason ${stop}`);
      return 5;
    }
    const q = QUESTION_RE.exec(session.lastTurnText);
    if (!q) return 0;
    const answer = await waitForAnswer(logPath, q[1].trim(), round, o.answerTimeout);
    if (answer === null) return 8;
    text = `Answer from the lead: ${answer}\nContinue the task.`;
  }
}

async function waitForAnswer(logPath, question, round, timeoutS) {
  const qPath = logPath.replace(/\.jsonl$/, `.question-${round}.md`);
  const aPath = logPath.replace(/\.jsonl$/, `.answer-${round}.md`);
  writeFileSync(qPath, `${question}\n`);
  console.log(`QUESTION ${round}: ${question}\n  answer by writing ${aPath}`);
  const until = Date.now() + timeoutS * 1000;
  while (Date.now() < until) {
    if (existsSync(aPath)) {
      const a = readFileSync(aPath, 'utf8').trim();
      if (a) { console.log(`ANSWER ${round}: ${a}`); return a; }
    }
    await new Promise((r) => setTimeout(r, ANSWER_POLL_MS));
  }
  console.error(`${TOOL}: question ${round} unanswered after ${timeoutS}s`);
  return null;
}

// AcpSession is a minimal ACP v1 client for `kimi acp`: JSON-RPC 2.0, one
// message per line on stdio.
class AcpSession {
  constructor(cwd, logPath) {
    this.cwd = cwd;
    this.logPath = logPath;
    this.nextId = 0;
    this.pending = new Map();
    this.buf = '';
    this.turnText = '';
    this.lastTurnText = '';
    this.denied = [];
    this.deniedIds = new Set();
    this.executedForbidden = [];
    this.commands = new Map();
    this.child = spawn('kimi', ['acp'], { cwd, env: hardenedEnv(), stdio: ['pipe', 'pipe', 'pipe'] });
    this.child.stderr.on('data', (d) => appendFileSync(`${logPath}.stderr`, d));
    this.child.stdout.on('data', (d) => this.onData(d));
    this.exited = new Promise((res) => this.child.on('close', (c, sig) => res(sig ?? c)));
    this.exited.then((why) => {
      for (const { reject } of this.pending.values()) reject(new Error(`kimi acp exited (${why})`));
      this.pending.clear();
    });
  }

  log(dir, msg) { appendFileSync(this.logPath, `${JSON.stringify({ dir, at: new Date().toISOString(), msg })}\n`); }

  write(msg) { this.log('out', msg); this.child.stdin.write(`${JSON.stringify(msg)}\n`); }

  request(method, params) {
    const id = this.nextId++;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.write({ jsonrpc: '2.0', id, method, params });
    });
  }

  async start(addDirs) {
    const init = await this.request('initialize', {
      protocolVersion: ACP_PROTOCOL_VERSION,
      clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false },
    });
    if (init.protocolVersion !== ACP_PROTOCOL_VERSION) throw new Error(`agent speaks ACP ${init.protocolVersion}`);
    const params = { cwd: this.cwd, mcpServers: [] };
    if (addDirs.length) params.additionalDirectories = addDirs;
    const s = await this.request('session/new', params);
    this.sessionId = s.sessionId;
    const model = (s.configOptions ?? []).find((c) => c.id === 'model')?.currentValue;
    if (model !== MODEL) throw new Error(`session model is ${model}, expected ${MODEL}`);
    const mode = (s.configOptions ?? []).find((c) => c.id === 'mode')?.currentValue;
    if (mode !== 'default') throw new Error(`session mode is ${mode}, expected default (manual approvals)`);
  }

  async prompt(text) {
    this.turnText = '';
    const r = await this.request('session/prompt', { sessionId: this.sessionId, prompt: [{ type: 'text', text }] });
    this.lastTurnText = this.turnText;
    return r.stopReason;
  }

  onData(d) {
    this.buf += d;
    for (let i = this.buf.indexOf('\n'); i >= 0; i = this.buf.indexOf('\n')) {
      const line = this.buf.slice(0, i).trim();
      this.buf = this.buf.slice(i + 1);
      if (!line) continue;
      let m;
      try { m = JSON.parse(line); } catch { this.log('in-raw', line); continue; }
      this.log('in', m);
      this.dispatch(m);
    }
  }

  dispatch(m) {
    if (m.method === undefined && this.pending.has(m.id)) {
      const p = this.pending.get(m.id);
      this.pending.delete(m.id);
      if (m.error) p.reject(new Error(`${m.error.code} ${m.error.message}`));
      else p.resolve(m.result ?? {});
      return;
    }
    if (m.method === 'session/update') return this.onUpdate(m.params?.update ?? {});
    if (m.method === 'session/request_permission') return this.onPermission(m);
    if (m.id !== undefined && m.method) {
      this.write({ jsonrpc: '2.0', id: m.id, error: { code: -32601, message: `${TOOL} does not provide ${m.method}` } });
    }
  }

  onUpdate(u) {
    switch (u.sessionUpdate) {
      case 'agent_message_chunk':
        if (u.content?.type === 'text') this.turnText += u.content.text;
        break;
      case 'tool_call':
        console.log(`  tool ${u.title ?? ''} (${u.kind ?? '?'})`);
        break;
      case 'tool_call_update': {
        const cmd = u.rawInput?.command;
        if (typeof cmd === 'string') this.commands.set(u.toolCallId, cmd);
        if (u.status === 'completed' || u.status === 'failed') {
          const ran = this.commands.get(u.toolCallId);
          // A denied call still reports status "failed" with its rawInput; it never ran.
          if (this.deniedIds.has(u.toolCallId)) {
            console.log(`  not run (denied): ${(ran ?? '').slice(0, 160)}`);
          } else if (ran) {
            console.log(`  ${u.status === 'failed' ? 'failed' : 'ran'}: ${ran.slice(0, 160)}`);
            const rules = forbiddenRules(ran);
            if (rules.length) this.executedForbidden.push({ rules, command: ran.slice(0, 300) });
          }
        }
        break;
      }
      default:
        break;
    }
  }

  // onPermission rejects a tool call whose command matches a FORBIDDEN rule
  // and approves everything else once. Kimi sends the command text in the
  // request ("Requesting approval to Running: <cmd>"), verified 2026-10-13.
  onPermission(m) {
    const p = m.params ?? {};
    const tc = p.toolCall ?? {};
    const shown = (tc.content ?? []).map((c) => c.content?.text ?? '').join('\n');
    const cmd = typeof tc.rawInput?.command === 'string' ? tc.rawInput.command : shown.replace(/^Requesting approval to Running:\s*/, '');
    const rules = forbiddenRules(cmd);
    const opts = p.options ?? [];
    const pick = (kind) => opts.find((x) => x.kind === kind);
    let option;
    if (rules.length) {
      option = pick('reject_once') ?? pick('reject_always');
      this.denied.push({ rules, command: cmd.slice(0, 300) });
      if (tc.toolCallId) this.deniedIds.add(tc.toolCallId);
      console.log(`  DENIED (${rules.join(', ')}): ${cmd.slice(0, 160)}`);
    } else {
      option = pick('allow_once');
    }
    const result = option
      ? { outcome: { outcome: 'selected', optionId: option.optionId } }
      : { outcome: { outcome: 'cancelled' } };
    this.write({ jsonrpc: '2.0', id: m.id, result });
  }

  abort(why) {
    console.error(`${TOOL}: aborting session (${why})`);
    if (this.sessionId) this.write({ jsonrpc: '2.0', method: 'session/cancel', params: { sessionId: this.sessionId } });
    setTimeout(() => this.child.kill('SIGTERM'), 5000);
  }

  close() {
    this.child.stdin.end();
    this.child.kill('SIGTERM');
  }
}

process.exitCode = await main();
