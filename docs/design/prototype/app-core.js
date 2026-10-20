/* Promethity node + agent — Praxis-aligned prototype. Core: icons, data, decision model v2, assistant pane, composer palette. */
(function () {
  'use strict';

  /* ---------- Icons (Lucide, 1.5 stroke) ---------- */
  const P = {
    home: '<path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8"/><path d="M3 10a2 2 0 0 1 .71-1.53l7-6a2 2 0 0 1 2.58 0l7 6A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
    folder: '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>',
    pin: '<path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/>',
    compass: '<circle cx="12" cy="12" r="10"/><path d="m16.24 7.76-2.12 6.36-6.36 2.12 2.12-6.36 6.36-2.12z"/>',
    network: '<rect x="16" y="16" width="6" height="6" rx="1"/><rect x="2" y="16" width="6" height="6" rx="1"/><rect x="9" y="2" width="6" height="6" rx="1"/><path d="M5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3"/><path d="M12 12V8"/>',
    pulse: '<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>',
    list: '<path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/><path d="M13 6h8"/><path d="M13 12h8"/><path d="M13 18h8"/>',
    settings: '<path d="M20 7h-9"/><path d="M14 17H5"/><circle cx="17" cy="17" r="3"/><circle cx="7" cy="7" r="3"/>',
    search: '<circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>',
    moon: '<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>',
    x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    copy: '<rect width="14" height="14" x="8" y="8" rx="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/>',
    flame: '<path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.072-2.143-.224-4.054 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.153.433-2.294 1-3a2.5 2.5 0 0 0 2.5 2.5z"/>',
    alert: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
    clock: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
    chevron: '<path d="m9 18 6-6-6-6"/>',
    file: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>',
    panel: '<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M15 3v18"/>',
    panelWide: '<rect width="18" height="18" x="3" y="3" rx="2"/><path d="M9 3v18"/>',
    full: '<path d="M15 3h6v6"/><path d="M9 21H3v-6"/><path d="M21 3l-7 7"/><path d="M3 21l7-7"/>',
    send: '<path d="M14.536 21.686a.5.5 0 0 0 .937-.024l6.5-19a.496.496 0 0 0-.635-.635l-19 6.5a.5.5 0 0 0-.024.937l7.93 3.18a2 2 0 0 1 1.112 1.11z"/><path d="m21.854 2.147-10.94 10.939"/>',
    plus: '<path d="M5 12h14"/><path d="M12 5v14"/>',
    trash: '<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>',
    refresh: '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
    drive: '<path d="M22 12H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><path d="M6 16h.01"/><path d="M10 16h.01"/>',
    key: '<circle cx="7.5" cy="15.5" r="5.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/>',
    external: '<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>',
    monitor: '<rect width="20" height="14" x="2" y="3" rx="2"/><path d="M8 21h8"/><path d="M12 17v4"/>',
    globe: '<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>',
    upload: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m17 8-5-5-5 5"/><path d="M12 3v12"/>',
    link: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
    arrow: '<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>',
    msg: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    layers: '<path d="m12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z"/><path d="m22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65"/><path d="m22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65"/>',
    cpu: '<rect width="16" height="16" x="4" y="4" rx="2"/><rect width="6" height="6" x="9" y="9" rx="1"/><path d="M15 2v2"/><path d="M15 20v2"/><path d="M2 15h2"/><path d="M2 9h2"/><path d="M20 15h2"/><path d="M20 9h2"/><path d="M9 2v2"/><path d="M9 20v2"/>',
    wrench: '<path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/>',
    eye: '<path d="M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0"/><circle cx="12" cy="12" r="3"/>',
    dots: '<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>',
    pause: '<rect x="14" y="4" width="4" height="16" rx="1"/><rect x="6" y="4" width="4" height="16" rx="1"/>',
    help: '<circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>'
  };
  const icon = (n, label) => `<svg class="od-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" ${label ? `role="img" aria-label="${label}"` : 'aria-hidden="true"'}>${P[n] || ''}</svg>`;
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const short = (cid) => cid.length > 20 ? cid.slice(0, 10) + '…' + cid.slice(-6) : cid;

  /* ---------- Real content (CIDs) + labelled samples ---------- */
  const CID = {
    praxis: 'bafybeianggntnigojmuqpsn2bskslb3mob5lkbaagd3wcgtfjokgv2dazi',
    wiki: 'bafybeiaysi4s6lnjev27ln5icwm6tueaw2vdykrtjkwiphwekaywqhcjze',
    xkcd: 'QmdmQXB2mzChmMeKY47C43LxUdg1NDJ5MWcKMKxDu7RgQm',
    lampas: 'bafybeid66k5dxzfvruqwdyehqusxe3z3v2pqn5jdyvsofncgqzutqmyqqe',
    opModel: 'bafybeifxdjjraggewgjqtrwh22i63kvwnhwd4jie4jqa646kxhieaujpz4',
    opBiz: 'bafkreiatn36lkd47xghupcvcv7sclbc7loanueke7yjj4a6byuxu6ae53i',
    opService: 'bafkreifathw2rdw7ovxaqtwwmecmnuhjpjjdxo4ofl26xjvyyfhtker3xm',
    whyOp: 'bafkreigq3ozhsloxe3yo7p5gpefzbit4kviyzq4hl64lzid4k2m32rblgu'
  };
  const FILES = [
    { name: 'promethity-lampas-model-gateway.pdf', cid: CID.lampas, size: '2.4 MB', kind: 'PDF' },
    { name: 'promethity-the-operator-model.pdf', cid: CID.opModel, size: '1.9 MB', kind: 'PDF' },
    { name: 'promethity-the-operator-service.pdf', cid: CID.opService, size: '846 KB', kind: 'PDF' },
    { name: 'promethity-the-operator-service-business-model.pdf', cid: CID.opBiz, size: '912 KB', kind: 'PDF' },
    { name: 'promethity-why-the-operator-model.pdf', cid: CID.whyOp, size: '604 KB', kind: 'PDF' }
  ];
  const PINS = [
    { name: 'Wikipedia (English) mirror', cid: CID.wiki, type: 'Recursive', by: 'You', when: '12 Oct' },
    { name: 'xkcd archive', cid: CID.xkcd, type: 'Recursive', by: 'You', when: '3 Oct' },
    { name: 'Lampas model gateway (PDF)', cid: CID.lampas, type: 'Recursive', by: 'Agent · approved by you', when: 'Today 09:12', dec: 'd-0' },
    { name: 'The operator model (PDF)', cid: CID.opModel, type: 'Recursive', by: 'You', when: '18 Oct' },
    { name: 'The operator service (PDF)', cid: CID.opService, type: 'Direct', by: 'You', when: '18 Oct' },
    { name: 'Why the operator model (PDF)', cid: CID.whyOp, type: 'Direct', by: 'You', when: '18 Oct' }
  ];
  const PEERS = [
    ['12D3KooWQYhTNQdmr3ArTeUHRYzFg94BKyTkoWBDWez9kSCVe2Xo', 'Frankfurt, DE', '/ip4/…/udp/4001/quic-v1', '38 ms', 'v0.39'],
    ['12D3KooWHfGj2CwA9GJh8mKf9oFz4w2kLgWm7JbG1aUTvW8x3Fqk', 'Ashburn, US', '/ip6/…/tcp/4001', '92 ms', 'v0.38'],
    ['12D3KooWJx7bTnLqRrFmZpAZmD4kKfH3wq8dUeW2hN6YcT1sV9xP', 'Singapore, SG', '/ip4/…/udp/4001/webrtc-direct', '171 ms', 'v0.39'],
    ['12D3KooWE5q9wYpM3xN8cR2tF7aKd1uLbV6zH4gJ0sQeT3yU8iOp', 'São Paulo, BR', '/ip4/…/udp/4001/quic-v1', '143 ms', 'v0.37'],
    ['12D3KooWB8nRk2LmQp4sT6vX9yZ1aC3eG5iK7mO0qS2uW4yA6cEg', 'Lagos, NG', '/dns4/…/tcp/443/wss', '205 ms', 'v0.39'],
    ['12D3KooWN3pH6jL9mR2tV5xZ8aD1fG4iK7nQ0sU3wY6bE9hJ2lMo', 'Toronto, CA', '/ip4/…/tcp/4001', '61 ms', 'v0.39']
  ];

  /* ---------- State ---------- */
  const S = {
    theme: localStorage.getItem('px-theme') || 'dark',
    canvas: localStorage.getItem('px-canvas') || 'docked',
    inspector: null,
    composerIntent: 'ask',
    paletteIntent: 'ask',
    unpin: null,
    applyState: 'idle',
    setupStep: 1,
    setupReach: 'idle',
    setupHostTyped: '',
    context: [
      { id: 'page', icon: 'folder', label: 'Page: Files › /Promethity/publish', prov: 'Added because this page is open. Remove to stop sending it.' },
      { id: 'model', icon: 'cpu', label: 'qwen3-32b on your network', ai: true, prov: 'Model placement: llm.lan:8080 (OpenAI-compatible). Node content goes only to this host.' },
      { id: 'tools', icon: 'wrench', label: '12 of 44 tools offered', prov: 'Tool shortlist chosen by the node’s agent loop for this request. Writes need your approval.' },
      { id: 'memory', icon: 'layers', label: 'Memory: none', prov: 'This assistant keeps no memory between threads.' }
    ],
    features: { chat: true, webmcpRead: true, webmcpWrite: false, desktop: true },
    messages: [],
    activities: [
      { id: 'a-1', title: 'Tidy Promethity pins', state: 'Waiting for approval', tone: 'warn', runs: 2, updated: '2 min ago' },
      { id: 'a-2', title: 'Index pinned PDFs for search', state: 'Running', tone: 'info', runs: 1, updated: 'now', progress: '128 of 412 files' },
      { id: 'a-3', title: 'Find who provides the Lampas PDF', state: 'Completed', tone: 'ok', runs: 1, updated: '09:14' },
      { id: 'a-4', title: 'Publish site to IPNS', state: 'Outcome unknown, checking', tone: 'warn', runs: 1, updated: '08:51' }
    ],
    decisions: [
      { id: 'd-1', activity: 'a-1', tool: 'pin_rm', verb: 'Remove pin', title: 'Remove the pin for an old draft', cls: 'Destructive',
        principal: 'Assistant in this Web UI', kind: 'ui', destination: 'This node', scope: [CID.opBiz], egress: null,
        digest: 'sha256:7f3a9c…e2c1', state: 'pending', at: '14:02', surface: 'This Web UI' },
      { id: 'd-2', activity: 'a-1', tool: 'pin_add', verb: 'Pin directory', title: 'Pin the Praxis specification directory', cls: 'Write',
        principal: 'Assistant in this Web UI', kind: 'ui', destination: 'This node', scope: [CID.praxis], egress: null,
        digest: 'sha256:19be04…77ad', state: 'pending', at: '14:02', surface: 'This Web UI' },
      { id: 'd-3', activity: 'a-4', tool: 'ipns_publish', verb: 'Publish name', title: 'Point the site name at the new build', cls: 'Write',
        principal: 'Claude Code (human-mcp)', kind: 'human-mcp', destination: 'IPNS name k51qzi5uqu5dl4…ll3t', scope: [CID.praxis], egress: null,
        digest: 'sha256:c04e11…9b02', state: 'uncertain', at: '08:51', surface: 'Claude Code' },
      { id: 'd-0', activity: 'a-3', tool: 'pin_add', verb: 'Pin file', title: 'Pin the Lampas model gateway PDF', cls: 'Write',
        principal: 'Assistant in this Web UI', kind: 'ui', destination: 'This node', scope: [CID.lampas], egress: null,
        digest: 'sha256:ab12f0…0e3d', state: 'committed', at: '09:12', surface: 'This Web UI', receipt: 'Pinned recursively · 09:12:41' },
      { id: 'd-4', activity: 'a-2', tool: 'mfs_write', verb: 'Write file', title: 'Save extracted text next to each PDF', cls: 'Write',
        principal: 'Claude Code (human-mcp)', kind: 'human-mcp', destination: 'This node · /Promethity/publish', scope: ['/Promethity/publish/*.txt'], egress: null,
        digest: 'sha256:5d0e7a…41f8', state: 'resolved_elsewhere', at: '08:40', surface: 'Promethity Desktop', resolvedAs: 'Denied' },
      { id: 'd-5', activity: 'a-3', tool: 'routing_provide', verb: 'Announce', title: 'Announce the PDFs to the network', cls: 'Write',
        principal: 'A2A peer · ops-agent', kind: 'a2a', destination: 'Public DHT', scope: [CID.lampas, CID.opModel], egress: 'Public network',
        digest: 'sha256:e8c2d4…aa10', state: 'denied', at: 'Yesterday 17:30', surface: 'This Web UI' },
      { id: 'd-6', activity: 'a-2', tool: 'repo_gc', verb: 'Run garbage collection', title: 'Free space by removing unpinned blocks', cls: 'Destructive',
        principal: 'Assistant in this Web UI', kind: 'ui', destination: 'This node', scope: ['Whole repository'], egress: null,
        digest: 'sha256:0b77ce…3f19', state: 'expired', at: 'Yesterday 16:02', surface: 'This Web UI' }
    ]
  };
  const PENDINGISH = ['pending', 'waiting_system_prompt', 'invalidated', 'uncertain', 'needs_resubmit'];
  const pendingCount = () => S.decisions.filter((d) => PENDINGISH.includes(d.state)).length;

  /* ---------- Decision model v2 ---------- */
  const STATE = {
    pending:              { word: 'Awaiting a person', tone: 'warn' },
    waiting_system_prompt:{ word: 'Waiting for system prompt', tone: 'warn' },
    approved:             { word: 'Approved, not yet run', tone: 'info' },
    committed:            { word: 'Done', tone: 'ok' },
    denied:               { word: 'Denied', tone: 'err' },
    expired:              { word: 'Expired', tone: '' },
    invalidated:          { word: 'Changed since you saw it', tone: 'warn' },
    resolved_elsewhere:   { word: 'Answered on another screen', tone: 'info' },
    uncertain:            { word: 'Outcome unknown, checking', tone: 'warn' },
    needs_resubmit:       { word: 'Can’t resume', tone: '' }
  };
  function badge(state) {
    const s = STATE[state];
    return `<span class="badge ${s.tone}"><span class="dot"></span>${s.word}</span>`;
  }
  function decisionCard(d, opts = {}) {
    const cls = opts.inChat ? 'approval in-chat' : 'approval';
    const scope = d.scope.map((c) => c.startsWith('/') || c.includes(' ') ? `<span class="mono">${esc(c)}</span>` : cidChip(c)).join('<br>');
    const egress = d.egress ? `<span class="badge info">Leaves this node: ${esc(d.egress)}</span>` : `<span class="badge">Stays on this node</span>`;
    let actions = '';
    let outcome = '';
    switch (d.state) {
      case 'pending':
        actions = `<button class="btn" data-act="deny" data-id="${d.id}" data-autofocus>Deny</button>
          <button class="btn primary arming" data-act="approve" data-id="${d.id}" disabled aria-describedby="arm-${d.id}">${esc(d.verb)}</button>
          <span class="note" id="arm-${d.id}">Approve unlocks after 1 second</span>`;
        break;
      case 'waiting_system_prompt':
        actions = `<button class="btn" data-act="deny" data-id="${d.id}">Deny</button><span class="note">Answer the macOS prompt to approve. This screen can only deny.</span>`;
        break;
      case 'approved':
        outcome = `<p class="outcome">Approved at ${esc(d.at)}. The node runs it next; this card updates when it has run.</p>`;
        break;
      case 'committed':
        outcome = `<div class="receipt"><strong>Done.</strong><span>${esc(d.receipt || 'Ran once.')} · ${esc(d.tool)} · digest ${esc(d.digest)}</span></div>`;
        break;
      case 'denied':
        outcome = `<p class="outcome">Denied. Nothing was changed.</p>`;
        break;
      case 'expired':
        outcome = `<p class="outcome">Expired. Nothing was changed.</p>`;
        actions = `<button class="btn sm" data-act="resubmit" data-id="${d.id}">${icon('refresh')}Ask again</button>`;
        break;
      case 'invalidated':
        outcome = `<div class="callout warn"><strong>The request changed after it was shown.</strong><span class="small">Scope went from 1 path to 2. The earlier digest no longer matches, so the old approval cannot be used.</span></div>`;
        actions = `<button class="btn sm" data-act="review" data-id="${d.id}">Review again</button>`;
        break;
      case 'resolved_elsewhere':
        outcome = `<p class="outcome">${esc(d.resolvedAs || 'Approved')} on ${esc(d.surface)} at ${esc(d.at)}. Nothing to do here.</p>`;
        break;
      case 'uncertain':
        outcome = `<p class="outcome">The node did not confirm in time. A timeout is not a failure: it may have run.</p>`;
        actions = `<button class="btn sm" data-act="check" data-id="${d.id}">${icon('refresh')}Check now</button>`;
        break;
      case 'needs_resubmit':
        outcome = `<p class="outcome">Can’t resume after the node restarted. Ask again to create a new request.</p>`;
        actions = `<button class="btn sm" data-act="resubmit" data-id="${d.id}">Ask again</button>`;
        break;
    }
    const uar = d.kind === 'uar' ? `<span class="badge">UAR approval, not a node confirmation</span>` : '';
    return `<article class="${cls}" ${d.state === 'pending' ? 'role="alertdialog"' : ''} aria-labelledby="t-${d.id}">
      <div class="ap-head">
        <div class="ap-title">
          <span class="eyebrow">${esc(d.cls)} · ${esc(d.tool)}</span>
          <h4 id="t-${d.id}">${esc(d.title)}</h4>
        </div>
        ${badge(d.state)}
      </div>
      <dl>
        <dt>Asked by</dt><dd>${esc(d.principal)}</dd>
        <dt>Where</dt><dd>${esc(d.destination)}</dd>
        <dt>What</dt><dd>${scope}</dd>
        <dt>Network</dt><dd>${egress} ${uar}</dd>
        <dt>Digest</dt><dd class="digest">${esc(d.digest)}</dd>
      </dl>
      ${outcome}
      ${actions ? `<div class="ap-actions">${actions}</div>` : ''}
    </article>`;
  }
  function cidChip(cid) {
    return `<span class="cid" title="${cid}"><span>${short(cid)}</span><button type="button" data-act="copy" data-copy="${cid}" aria-label="Copy CID ${short(cid)}">${icon('copy')}</button></span>`;
  }
  function armApprovals(root) {
    root.querySelectorAll('button[data-act="approve"][disabled]').forEach((b) => {
      setTimeout(() => {
        b.disabled = false; b.classList.remove('arming');
        const n = document.getElementById('arm-' + b.dataset.id); if (n) n.textContent = 'Deny is the safe default';
      }, 1000);
    });
  }
  function resolve(id, act) {
    const d = S.decisions.find((x) => x.id === id); if (!d) return;
    const now = new Date().toTimeString().slice(0, 5);
    if (act === 'deny') { d.state = 'denied'; d.at = now; toast('Denied. Nothing was changed.'); }
    if (act === 'approve') {
      d.state = 'approved'; d.at = now; toast('Approved. A new run starts on the same activity.');
      setTimeout(() => { d.state = 'committed'; d.receipt = d.tool === 'pin_rm' ? 'Pin removed · ' + now : 'Ran once · ' + now; afterResolve(d); }, 1400);
    }
    if (act === 'check') { d.state = 'committed'; d.receipt = 'Confirmed on check · name points to the new build'; toast('Checked. It had run.'); }
    if (act === 'resubmit') { d.state = 'pending'; d.digest = 'sha256:' + Math.random().toString(16).slice(2, 8) + '…' + Math.random().toString(16).slice(2, 6); d.at = now; toast('New request created. It needs your answer.'); }
    if (act === 'review') { d.state = 'pending'; d.scope = d.scope.concat(['/Promethity/drafts']); d.digest = 'sha256:4aa9e1…0c7b'; }
    afterResolve(d);
  }
  function afterResolve(d) {
    const a = S.activities.find((x) => x.id === d.activity);
    if (a) {
      const open = S.decisions.some((x) => x.activity === a.id && PENDINGISH.includes(x.state));
      if (!open && a.state === 'Waiting for approval') { a.state = 'Running'; a.tone = 'info'; a.runs += 1; a.updated = 'now';
        setTimeout(() => { a.state = 'Completed'; a.tone = 'ok'; window.PX.render(); }, 1800); }
      if (d.activity === 'a-4' && d.state === 'committed') { a.state = 'Completed'; a.tone = 'ok'; }
    }
    window.PX.render();
  }

  /* ---------- Toast ---------- */
  let toastT;
  function toast(msg) {
    const t = document.getElementById('toast');
    t.innerHTML = `${icon('check')}<span>${esc(msg)}</span>`;
    t.classList.add('show'); clearTimeout(toastT); toastT = setTimeout(() => t.classList.remove('show'), 2800);
  }

  /* ---------- Assistant pane ---------- */
  const seed = () => ([
    { who: 'user', text: 'Which of my pins are the Promethity PDFs, and does anyone else provide them?' },
    { who: 'agent', phases: [['Planned', 'done'], ['pin_ls', 'done'], ['routing_findprovs', 'done'], ['Answered', 'done']],
      tools: [
        { name: 'pin_ls', status: 'Done', out: '{ "pins": 6, "matched": 5, "type": "recursive|direct" }' },
        { name: 'routing_findprovs', status: 'Done', out: '{ "cid": "' + short(CID.lampas) + '", "providers": 4 }' }
      ],
      html: `<p>Five Promethity PDFs are pinned. Three are recursive pins and two are direct.</p><p>Besides this node, three peers provide the Lampas gateway PDF. The other four PDFs are only on this node, so if you unpin them they may stop being reachable.</p>`,
      cites: [CID.lampas, CID.opModel] },
    { who: 'user', text: 'Remove the old business-model draft and pin the Praxis spec directory.' },
    { who: 'agent', phases: [['Planned', 'done'], ['pin_rm', 'done'], ['Waiting for your approval', 'wait']],
      html: `<p>Both are changes to this node, so each needs your answer. Removing a pin is destructive.</p>`,
      decisions: ['d-1', 'd-2'],
      notice: 'This run ended at the approvals. Answering starts a new run on the same activity, “Tidy Promethity pins”.' }
  ]);
  S.messages = seed();

  function renderPane() {
    const pane = document.getElementById('pane');
    const app = document.getElementById('app');
    const open = S.canvas !== 'closed';
    app.classList.toggle('pane-open', open);
    app.dataset.canvasState = S.canvas;
    const pt = document.getElementById('pane-toggle');
    pt.setAttribute('aria-pressed', String(open));
    pt.setAttribute('aria-label', open ? 'Close assistant' : 'Open assistant');
    if (!open) { pane.innerHTML = ''; return; }
    const mode = (m, ic, label) => `<button type="button" data-canvas="${m}" aria-pressed="${S.canvas === m}" aria-label="${label}" title="${label}">${icon(ic)}</button>`;
    const turns = S.messages.map(renderTurn).join('');
    pane.innerHTML = `
      <div class="pane-head">
        <h3>Assistant</h3>
        <div class="pane-modes" role="group" aria-label="Assistant size">
          ${mode('docked', 'panel', 'Docked (press [ to narrow)')}${mode('expanded', 'panelWide', 'Expanded (press ] to widen)')}${mode('full', 'full', 'Full width')}
        </div>
        <button class="iconbtn" data-act="inspect" aria-label="Open context inspector">${icon('eye')}</button>
        <button class="iconbtn" data-canvas="closed" aria-label="Close assistant">${icon('x')}</button>
      </div>
      <div class="ctxstrip" aria-label="What the assistant can see">
        ${S.context.map((c) => `<span class="chip ${c.ai ? 'ai' : ''}" title="${esc(c.prov)}">${icon(c.icon)}<span>${esc(c.label)}</span>${c.id === 'model' || c.id === 'tools' ? '' : `<button type="button" data-act="ctx-rm" data-id="${c.id}" aria-label="Remove ${esc(c.label)} from context">${icon('x')}</button>`}</span>`).join('')}
        <button class="linkbtn" data-act="inspect">Inspect</button>
      </div>
      <div class="stream" id="stream" aria-live="polite">${turns}</div>
      <div class="composer">
        <form id="pane-form">
          <label for="pane-input" class="sr-only">Message the assistant</label>
          <textarea id="pane-input" rows="1" placeholder="Ask about this node, a CID or a file"></textarea>
          <button class="btn" type="submit" aria-label="Send">${icon('send')}</button>
        </form>
        <p class="small muted">Writes and deletes always ask you first. Content from the network is shown as data.</p>
      </div>
      ${S.inspector ? renderInspector() : ''}`;
    armApprovals(pane);
    const st = document.getElementById('stream'); if (st) st.scrollTop = st.scrollHeight;
  }
  function renderTurn(m) {
    if (m.who === 'user') return `<div class="msg user"><span class="who">You</span><div class="bubble">${esc(m.text)}</div></div>`;
    const phases = m.phases ? `<div class="lifecycle" aria-label="Run phases">${m.phases.map(([n, s], i) =>
      `${i ? '<span class="arrow">›</span>' : ''}<span class="ph ${s}">${s === 'done' ? icon('check') : s === 'wait' ? icon('clock') : ''}${esc(n)}</span>`).join('')}</div>` : '';
    const tools = (m.tools || []).map((t) => `<div class="tool"><div class="t-head">${icon('wrench')}<span class="t-name">${esc(t.name)}</span><span class="badge ${t.status === 'Done' ? 'ok' : t.status === 'Failed' ? 'err' : 'info'}">${esc(t.status)}</span></div>
      <details><summary>Show result</summary><pre>${esc(t.out)}</pre></details></div>`).join('');
    const decs = (m.decisions || []).map((id) => decisionCard(S.decisions.find((d) => d.id === id), { inChat: true })).join('');
    const cites = m.cites ? `<div class="od-cluster" style="--od-gap:6px">${m.cites.map(cidChip).join('')}</div>` : '';
    const notice = m.notice ? `<div class="notice info">${icon('clock')}<span>${esc(m.notice)}</span></div>` : '';
    return `<div class="msg agent"><span class="who">Assistant</span>${phases}${tools}<div class="bubble ${m.streaming ? 'streaming' : ''}">${m.html || ''}</div>${cites}${decs}${notice}</div>`;
  }
  function renderInspector() {
    const tabs = ['Overview', 'Context', 'Prompt plan', 'Events'];
    const t = S.inspector;
    let body = '';
    if (t === 'Overview') body = `<dl class="kv"><dt>Container</dt><dd>This node (one container per node)</dd><dt>Thread</dt><dd>Tidy Promethity pins</dd><dt>Activity</dt><dd>${badge('pending')} 2 runs</dd><dt>Backend</dt><dd>OpenAI-compatible · llm.lan:8080</dd><dt>Model</dt><dd>qwen3-32b</dd><dt>Limits</dt><dd>8 steps · 32k tokens · 120 s per run</dd></dl>`;
    if (t === 'Context') body = `<ul class="list">${S.context.map((c) => `<li class="row"><span>${icon(c.icon)}</span><div class="od-field od-fill"><strong>${esc(c.label)}</strong><span class="small muted">${esc(c.prov)}</span></div></li>`).join('')}</ul>`;
    if (t === 'Prompt plan') body = `<p class="small muted">The node’s agent loop decides this. The screen only shows it.</p><dl class="kv"><dt>Tools offered</dt><dd>12 of 44: pin_ls, pin_add, pin_rm, pin_verify, mfs_ls, mfs_stat, ipfs_ls, ipfs_cat, dag_stat, routing_findprovs, ipfs_index_search, ipfs_node_status</dd><dt>Not offered</dt><dd>Destructive groups other than pin, and every names/keys tool</dd><dt>Context used</dt><dd>6.1k of 32k tokens</dd><dt>Truncated</dt><dd>Nothing</dd></dl>`;
    if (t === 'Events') body = `<pre class="mono" style="white-space:pre-wrap;margin:0;font-size:12px;line-height:1.7;color:var(--text-muted)">14:01:58 RUN_STARTED run-7c1 · activity a-1
14:01:59 TOOL_CALL pin_ls
14:02:00 TOOL_RESULT pin_ls · 6 pins
14:02:01 TOOL_CALL pin_rm · class destructive
14:02:01 INTERRUPT decision d-1 · digest 7f3a9c…e2c1
14:02:01 TOOL_CALL pin_add · class write
14:02:01 INTERRUPT decision d-2 · digest 19be04…77ad
14:02:02 RUN_FINISHED run-7c1 · reason interrupt</pre><p class="small muted">Replays from stored events, without asking the model again.</p>`;
    return `<section class="inspector" role="dialog" aria-label="Context inspector">
      <div class="pane-head"><h3>Context inspector</h3><button class="iconbtn" data-act="inspect-close" aria-label="Close inspector">${icon('x')}</button></div>
      <div class="tabs" role="tablist">${tabs.map((x) => `<button role="tab" aria-selected="${x === t}" data-tab="${x}">${x}</button>`).join('')}</div>
      <div class="stream">${body}</div></section>`;
  }
  function sendToAssistant(text) {
    if (!text.trim()) return;
    if (S.canvas === 'closed') S.canvas = 'docked';
    S.messages.push({ who: 'user', text });
    const m = { who: 'agent', phases: [['Planned', 'done'], ['ipfs_node_status', 'now']], html: '', streaming: true };
    S.messages.push(m); renderPane();
    setTimeout(() => { m.phases = [['Planned', 'done'], ['ipfs_node_status', 'done'], ['Answering', 'now']]; m.html = '<p>Your node is running with 142 peers.'; renderPane(); }, 700);
    setTimeout(() => { m.phases = [['Planned', 'done'], ['ipfs_node_status', 'done'], ['Answered', 'done']]; m.streaming = false;
      m.tools = [{ name: 'ipfs_node_status', status: 'Done', out: '{ "peers": 142, "repo": "18.4 GB of 50 GB" }' }];
      m.html = '<p>Your node is running with 142 peers. The repository holds 18.4 GB of a 50 GB limit. Nothing needed a change, so nothing asked for approval.</p>'; renderPane(); }, 1600);
  }

  /* ---------- Composer palette (the same composer, opened anywhere) ---------- */
  function renderPalette(q = '') {
    const el = document.getElementById('palette');
    const mode = (m, l) => `<button type="button" data-pintent="${m}" aria-pressed="${S.paletteIntent === m}">${l}</button>`;
    const res = paletteResults(q);
    el.innerHTML = `<div class="palette">
      <div class="pin">${icon(S.paletteIntent === 'go' ? 'arrow' : S.paletteIntent === 'search' ? 'search' : 'msg')}
        <label for="pal-q" class="sr-only">Composer</label>
        <input id="pal-q" autocomplete="off" value="${esc(q)}" placeholder="${S.paletteIntent === 'go' ? 'Paste a CID, /ipfs/ or /ipns/ path' : S.paletteIntent === 'search' ? 'Describe what you are looking for' : 'Ask the assistant'}"></div>
      <div class="modes" role="group" aria-label="Intent">${mode('ask', 'Ask the assistant')}${mode('search', 'Search my content')}${mode('go', 'Go to CID or path')}</div>
      <div class="results" role="listbox">${res}</div>
      <div class="foot"><span><kbd>↵</kbd> choose</span><span><kbd>Tab</kbd> next intent</span><span><kbd>Esc</kbd> close</span></div></div>`;
  }
  function paletteResults(q) {
    if (S.paletteIntent === 'go') {
      const items = [[CID.praxis, 'Praxis specification directory'], [CID.xkcd, 'xkcd archive'], [CID.wiki, 'Wikipedia (English) mirror']]
        .filter(([c, n]) => !q || c.includes(q) || n.toLowerCase().includes(q.toLowerCase()));
      return (q && !items.length ? `<button class="res" data-go="${esc(q)}">${icon('compass')}<span class="od-field"><span>Open ${esc(short(q))} in Explore</span><span class="small muted">Not in your recent list</span></span></button>` : '') +
        items.map(([c, n]) => `<button class="res" data-go="${c}">${icon('compass')}<span class="od-field"><span>${esc(n)}</span><span class="small muted mono">${short(c)}</span></span></button>`).join('');
    }
    if (S.paletteIntent === 'search') {
      return `<button class="res" data-route="#/search">${icon('search')}<span class="od-field"><span>Search for “${esc(q || 'operator service pricing')}”</span><span class="small muted">Searches 128 of 412 indexed files. Indexing is still running.</span></span></button>`;
    }
    const sug = ['Which pins are only on this node?', 'Why is bandwidth high today?', 'Summarise the Lampas PDF'];
    return (q ? `<button class="res" data-ask="${esc(q)}">${icon('msg')}<span class="od-field"><span>Ask: ${esc(q)}</span><span class="small muted">Opens the assistant. Nothing runs until you send.</span></span></button>` : '') +
      sug.map((s) => `<button class="res" data-draft="${esc(s)}">${icon('msg')}<span class="od-field"><span>${esc(s)}</span><span class="small muted">Suggestion. Fills the composer; does not send.</span></span></button>`).join('');
  }
  function openPalette(intent) {
    if (intent) S.paletteIntent = intent;
    renderPalette(); const el = document.getElementById('palette'); el.classList.add('open');
    setTimeout(() => document.getElementById('pal-q').focus(), 10);
  }
  function closeOverlay(id) { document.getElementById(id).classList.remove('open'); document.getElementById(id).innerHTML = ''; }

  window.PX = { S, CID, FILES, PINS, PEERS, icon, esc, short, badge, decisionCard, cidChip, armApprovals, resolve, toast,
    renderPane, sendToAssistant, openPalette, renderPalette, closeOverlay, pendingCount, PENDINGISH, STATE, render: () => {} };
})();
