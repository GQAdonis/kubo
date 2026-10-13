// Shared parts of the Kimi Code task drivers for phase agent-backend-settings:
// kimi-task.mjs (prompt mode, scans afterwards) and kimi-acp.mjs (ACP client,
// gates each tool call before it runs). Both enforce the same rules (plan
// section 10, WK-1 and PR-K4).

import { spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync } from 'node:fs';
import { homedir, tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

export const KUBO = '/Users/gqadonis/Projects/references/kubo';
export const PHASE = 'agent-backend-settings';
export const PLAN = `${KUBO}/.kbd-orchestrator/phases/${PHASE}/plan.md`;
export const MODEL = 'kimi-code/k3';
export const DEFAULT_TIMEOUT_S = 3600;
export const LOG_DIR = join(dirname(fileURLToPath(import.meta.url)), 'logs');

// Outward or privileged actions a builder session must never take (plan W2).
// Cluster tools match only in command position: a path such as helm-values/
// in a read-only grep is not a helm call (lke-origin-isolation 1.2, 2026-10-13).
export const FORBIDDEN = [
  { name: 'git push', re: /\bgit\b[^|;&]*\bpush\b/ },
  { name: 'git remote/credential change', re: /\bgit\b[^|;&]*\b(remote\s+(add|set-url)|config\b[^|;&]*credential)/ },
  { name: 'gh write', re: /\bgh\s+(pr|issue|release|repo|gist|secret|workflow|label)\b/ },
  { name: 'gh api write', re: /\bgh\s+api\b[^|;&]*(-X\s*|--method\s+)(POST|PUT|PATCH|DELETE)/i },
  { name: 'kubectl', re: /(?:^|[;&|(`"']\s*|\$\(\s*|\b(?:sudo|env|xargs|exec|command|nohup|timeout\s+\S+)\s+)(?:\S*\/)?kubectl(?=\s|$)/m },
  { name: 'argocd', re: /(?:^|[;&|(`"']\s*|\$\(\s*|\b(?:sudo|env|xargs|exec|command|nohup|timeout\s+\S+)\s+)(?:\S*\/)?argocd(?=\s|$)/m },
  { name: 'helm', re: /(?:^|[;&|(`"']\s*|\$\(\s*|\b(?:sudo|env|xargs|exec|command|nohup|timeout\s+\S+)\s+)(?:\S*\/)?helm(?=\s|$)/m },
  { name: 'docker push/login', re: /\bdocker\b[^|;&]*\b(push|login)\b/ },
  { name: 'kimi config change', re: /\.kimi-code\/config\.toml|\.kimi\/config\.toml/ },
  {
    name: 'non-loopback mutating curl',
    re: /\bcurl\b(?=[^|;&]*(-X\s*(POST|PUT|PATCH|DELETE)|--data|-d\s|--upload-file|-T\s))(?![^|;&]*(127\.0\.0\.1|localhost|\[::1\]))/i,
  },
];

// forbiddenRules returns the names of the FORBIDDEN rules a command matches.
export function forbiddenRules(cmd) {
  if (!cmd) return [];
  return FORBIDDEN.filter((f) => f.re.test(cmd)).map((f) => f.name);
}

export function usage(tool, msg) {
  console.error(`${tool}: ${msg}`);
  process.exit(2);
}

export function parseArgs(tool, argv, extraFlags = {}) {
  const o = { addDirs: [], timeout: DEFAULT_TIMEOUT_S, cwd: KUBO, kbd: false, dryRun: false, extra: '' };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    const next = () => {
      if (i + 1 >= argv.length) usage(tool, `${a} needs a value`);
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
    else if (extraFlags[a]) extraFlags[a](o, next);
    else usage(tool, `unknown argument ${a}`);
  }
  if (!o.selfTest && (!o.change || !o.task || !o.role)) usage(tool, '--change, --task and --role are required');
  if (!Number.isFinite(o.timeout) || o.timeout <= 0) usage(tool, '--timeout must be a positive number');
  return o;
}

export function readTasks(tool, change) {
  const f = `${KUBO}/openspec/changes/${change}/tasks.md`;
  if (!existsSync(f)) usage(tool, `no tasks.md for change ${change}`);
  return readFileSync(f, 'utf8')
    .split('\n')
    .map((l) => l.match(/^- \[[ x/]\] (\d+\.\d+) (.*)$/))
    .filter(Boolean)
    .map((m) => ({ id: m[1], title: m[2] }));
}

export function hardenedEnv() {
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

export function remoteRefs(dir) {
  const r = spawnSync('git', ['-C', dir, 'ls-remote', 'origin'], { encoding: 'utf8', timeout: 30000, stdio: ['ignore', 'pipe', 'pipe'] });
  return r.status === 0 ? r.stdout.trim() : null;
}

export function kbdApply(verb, o, idx, total, title) {
  const candidates = [`${homedir()}/.kimi-code/skills/kbd-apply/kbd-apply.sh`, `${homedir()}/.claude/skills/kbd-apply/kbd-apply.sh`];
  const apply = candidates.find((p) => existsSync(p));
  if (!apply) return { ok: false, out: 'kbd-apply.sh not found' };
  const r = spawnSync('bash', [apply, verb, o.change, o.task, String(idx), String(total), title], {
    cwd: KUBO, encoding: 'utf8', timeout: 120000, stdio: ['ignore', 'pipe', 'pipe'],
  });
  return { ok: r.status === 0, out: `${r.stdout ?? ''}${r.stderr ?? ''}`.trim() };
}

export function buildPrompt(o, title) {
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

// The prompt asks for an "Acceptance result" section; a verdict of not met,
// blocked or partially met in the last such section keeps the task open.
// Checked against all 20 session logs of 2026-10-13: only
// mcp-agui-a2ui-registry 1.1 matches.
export function acceptanceNotMet(text) {
  const t = text ?? '';
  const i = t.search(/acceptance result[^\n]*$(?![\s\S]*acceptance result)/im);
  if (i < 0) return false;
  const section = t.slice(i, i + 500).split(/\n#{1,4} /)[0];
  return /\bnot met\b|\bpartially met\b|\bunmet\b|\*\*\s*blocked|^\W*blocked\b/im.test(section);
}
