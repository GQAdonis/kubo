/* Promethity node + agent — views, router and events. Depends on app-core.js (window.PX). */
(function () {
  'use strict';
  const { S, CID, FILES, PINS, PEERS, icon, esc, short, badge, decisionCard, cidChip, armApprovals, resolve, toast,
    renderPane, sendToAssistant, openPalette, renderPalette, closeOverlay, pendingCount, PENDINGISH, STATE } = window.PX;

  /* ---------- Navigator ---------- */
  const NAV = [
    ['home', 'Home', 'home'], ['files', 'Files', 'folder'], ['pins', 'Pins', 'pin'], ['explore', 'Explore', 'compass'],
    ['peers', 'Peers', 'network'], ['activity', 'Activity', 'list']
  ];
  const NAV_FOOT = [['diagnostics', 'Diagnostics', 'pulse'], ['settings', 'Settings', 'settings'], ['surfaces', 'Design board', 'layers']];
  function renderNav(route) {
    const item = ([id, label, ic], extra = '') => {
      const count = id === 'activity' && pendingCount() ? `<span class="count" aria-label="${pendingCount()} decisions waiting">${pendingCount()}</span>` : '';
      return `<li class="${extra}"><a href="#/${id}" ${route === id ? 'aria-current="page"' : ''}>${icon(ic)}<span class="label">${label}</span>${count}</a></li>`;
    };
    const moreRoutes = ['explore', 'peers', 'diagnostics', 'settings', 'surfaces', 'search', 'setup', 'thread'];
    document.getElementById('nav').innerHTML = NAV.map((n) => item(n, n[0] === 'explore' || n[0] === 'peers' ? 'hide-sm' : '')).join('') +
      `<li class="show-sm"><button type="button" class="navbtn" data-act="more" aria-haspopup="dialog" ${moreRoutes.includes(route) ? 'aria-current="true"' : ''}>${icon('dots')}<span class="label">More</span></button></li>`;
    document.getElementById('nav-foot').innerHTML = NAV_FOOT.map((n) => item(n)).join('');
  }

  /* ---------- Shared bits ---------- */
  const head = (eyebrow, title, sub, actions = '') => `<header class="page-head"><span class="eyebrow">${eyebrow}</span>
    <div class="od-row" style="--od-gap:16px;flex-wrap:wrap"><h1 class="od-fill">${title}</h1>${actions}</div>${sub ? `<p>${sub}</p>` : ''}</header>`;
  const spark = () => `<svg class="spark" viewBox="0 0 300 56" preserveAspectRatio="none" role="img" aria-label="Bandwidth over the last hour, sample data">
    <polyline fill="none" stroke="var(--text-muted)" stroke-width="1.5" points="0,40 20,38 40,42 60,30 80,34 100,22 120,28 140,18 160,24 180,14 200,20 220,26 240,16 260,22 280,12 300,18"/>
    <polyline fill="none" stroke="var(--info)" stroke-width="1.5" points="0,48 20,46 40,47 60,44 80,45 100,40 120,43 140,38 160,41 180,36 200,39 220,42 240,37 260,40 280,35 300,38"/></svg>`;

  /* ---------- Home (HOME-01/02/07/08) ---------- */
  function vHome() {
    const pend = S.decisions.filter((d) => PENDINGISH.includes(d.state));
    const intent = (m, l) => `<button type="button" data-intent="${m}" aria-pressed="${S.composerIntent === m}">${l}</button>`;
    return `<div class="page">
      ${head('Home · this node', 'Your node is running.', pend.length ? `${pend.length} ${pend.length === 1 ? 'decision is' : 'decisions are'} waiting for you. Nothing that changes this node runs until you answer.` : 'No decisions are waiting.')}
      <section class="composer-home" aria-label="Composer">
        <div class="seg" role="group" aria-label="What should the composer do">${intent('ask', 'Ask the assistant')}${intent('search', 'Search my content')}${intent('go', 'Go to CID or path')}</div>
        <form id="home-form"><label class="sr-only" for="home-input">Composer</label>
          <textarea id="home-input" rows="1" placeholder="${S.composerIntent === 'go' ? 'Paste a CID, /ipfs/ or /ipns/ path' : S.composerIntent === 'search' ? 'Describe the file you are looking for' : 'Ask about pins, peers, files or a CID'}"></textarea>
          <button class="btn" type="submit" aria-label="Send">${icon(S.composerIntent === 'go' ? 'arrow' : 'send')}</button></form>
        <div class="suggest" aria-label="Suggestions (fill the composer, never send)">
          <button type="button" data-draft="Which pins are only on this node?">Pins only on this node</button>
          <button type="button" data-draft="Why is bandwidth higher than usual today?">Why is bandwidth high</button>
          <button type="button" data-draft="Summarise the Lampas model gateway PDF">Summarise the Lampas PDF</button>
        </div>
      </section>
      <div class="od-grid" style="--od-cols:3;--od-gap:14px" id="home-cards">
        <section class="card" style="grid-column: span 2" aria-labelledby="h-dec">
          <div class="card-head"><h4 id="h-dec">Needs your decision</h4>${pend.length ? `<span class="badge warn"><span class="dot"></span>${pend.length} waiting</span>` : '<span class="badge ok"><span class="dot"></span>All answered</span>'}</div>
          ${pend.length ? `<ul class="list">${pend.slice(0, 3).map((d) => `<li class="row link" data-route="#/activity">${icon(d.cls === 'Destructive' ? 'trash' : 'pin')}<div class="od-field od-fill"><strong class="od-truncate">${esc(d.title)}</strong><span class="small muted">${esc(d.principal)} · ${esc(d.cls)}</span></div>${badge(d.state)}</li>`).join('')}</ul>
            <div><a class="btn primary" href="#/activity">Review ${pend.length} ${pend.length === 1 ? 'decision' : 'decisions'}</a></div>` : '<p class="muted">When an agent asks to change this node, the request appears here and in the tray.</p>'}
        </section>
        <section class="card" aria-labelledby="h-node">
          <div class="card-head"><h4 id="h-node">Node</h4><span class="badge ok"><span class="dot"></span>Running</span></div>
          <div class="od-row" style="--od-gap:24px"><div class="od-stat"><span class="stat-num">142</span><span class="stat-cap">peers</span></div><div class="od-stat"><span class="stat-num">1,284</span><span class="stat-cap">pins</span></div></div>
          <div class="od-stack" style="--od-gap:6px"><div class="od-row"><span class="small od-fill">Repository</span><span class="small muted tnum od-nowrap">18.4 of 50 GB</span></div><div class="meter"><i style="width:37%"></i></div></div>
        </section>
        <section class="card" aria-labelledby="h-cont">
          <div class="card-head"><h4 id="h-cont">Continue working</h4></div>
          <ul class="list">${S.activities.filter((a) => a.state !== 'Completed').map((a) => `<li class="row link" data-act="open-thread">${icon('msg')}<div class="od-field od-fill"><strong class="od-truncate">${esc(a.title)}</strong><span class="small muted">${a.runs} ${a.runs === 1 ? 'run' : 'runs'} · ${esc(a.updated)}${a.progress ? ' · ' + esc(a.progress) : ''}</span></div><span class="badge ${a.tone}"><span class="dot"></span>${esc(a.state)}</span></li>`).join('')}</ul>
        </section>
        <section class="card" aria-labelledby="h-bw">
          <div class="card-head"><h4 id="h-bw">Bandwidth</h4><span class="small muted">sample</span></div>
          ${spark()}
          <div class="od-row small"><span class="od-fill"><span style="color:var(--text-muted)">In</span> <span class="tnum od-nowrap">2.1 MB/s</span></span><span><span style="color:var(--info)">Out</span> <span class="tnum od-nowrap">640 KB/s</span></span></div>
        </section>
        <section class="card" aria-labelledby="h-agent">
          <div class="card-head"><h4 id="h-agent">Assistant</h4><span class="badge ok"><span class="dot"></span>Connected</span></div>
          <dl class="kv"><dt>Backend</dt><dd>OpenAI-compatible · llm.lan:8080</dd><dt>Model</dt><dd>qwen3-32b</dd><dt>Search</dt><dd>Indexing, 128 of 412</dd><dt>Desktop</dt><dd>Paired</dd></dl>
          <a href="#/settings/agent" class="small">Assistant settings</a>
        </section>
        <section class="card" style="grid-column: span 3" aria-labelledby="h-recent">
          <div class="card-head"><h4 id="h-recent">Recently added</h4><a class="small" href="#/files">Open Files</a></div>
          <div class="tablewrap"><table class="table"><thead><tr><th>Name</th><th>CID</th><th class="num">Size</th></tr></thead><tbody>
          ${FILES.slice(0, 3).map((f) => `<tr><td><span class="od-row">${icon('file')}<span class="od-truncate">${esc(f.name)}</span></span></td><td>${cidChip(f.cid)}</td><td class="num">${f.size}</td></tr>`).join('')}</tbody></table></div>
        </section>
      </div>
    </div>`;
  }

  /* ---------- Thread (conversation-forward: thread · canvas) ---------- */
  function vThread() {
    return `<div class="page" style="max-width:1400px">
      ${head('Thread · Tidy Promethity pins', 'Tidy Promethity pins', 'Conversation-forward mode: the thread on the left, the canvas with the live surface on the right. The IPFS apps stay one click away in the navigator.')}
      <div class="thread-layout">
        <div class="thread-col" id="thread-col"></div>
        <div class="canvas-col">
          <div><div class="surface-strip">${icon('layers')}<span class="od-fill">Surface · PinList · catalog ipfs-mcp 0.9 · revision 3</span><button class="iconbtn" style="width:32px;height:32px" aria-label="Close surface" data-route="#/home">${icon('x')}</button></div>
          <div class="surface-body">
            <div class="tablewrap"><table class="table"><thead><tr><th>Pin</th><th>Type</th><th>State</th></tr></thead><tbody>
            ${PINS.slice(2).map((p) => `<tr><td><div class="od-field"><strong class="od-truncate">${esc(p.name)}</strong>${cidChip(p.cid)}</div></td><td>${p.type}</td><td>${p.cid === CID.opBiz ? badge('pending') : '<span class="badge ok"><span class="dot"></span>Pinned</span>'}</td></tr>`).join('')}
            <tr><td><div class="od-field"><strong>Operator service business model (PDF)</strong>${cidChip(CID.opBiz)}</div></td><td>Direct</td><td>${badge(S.decisions[0].state)}</td></tr>
            </tbody></table></div>
            <div class="inert" role="note"><strong>Unknown component: PeerMap</strong><span class="muted">This surface asked for a component the catalog does not have. It is shown as text and does nothing.</span></div>
          </div></div>
        </div>
      </div></div>`;
  }

  /* ---------- Files ---------- */
  function vFiles() {
    return `<div class="page">
      ${head('Files · MFS', 'Files', null, `<button class="btn" data-act="toast" data-msg="Choose files to add. Added files are pinned to this node.">${icon('upload')}Add files</button>`)}
      <nav aria-label="Path" class="od-cluster small"><a href="#/files">Files</a><span class="muted">/</span><a href="#/files">Promethity</a><span class="muted">/</span><span>publish</span></nav>
      <div class="tablewrap"><table class="table"><thead><tr><th>Name</th><th>CID</th><th class="num">Size</th><th><span class="sr-only">Actions</span></th></tr></thead><tbody>
      ${FILES.map((f) => `<tr><td><span class="od-row">${icon('file')}<span class="od-truncate" title="${esc(f.name)}">${esc(f.name)}</span></span></td><td>${cidChip(f.cid)}</td><td class="num">${f.size}</td>
        <td><div class="od-row" style="justify-content:flex-end"><button class="btn sm ghost" data-act="ask-file" data-name="${esc(f.name)}">${icon('msg')}Ask about this</button><button class="btn sm ghost" data-act="copy" data-copy="https://ipfs.io/ipfs/${f.cid}" aria-label="Copy share link for ${esc(f.name)}">${icon('link')}</button></div></td></tr>`).join('')}
      </tbody></table></div>
      <p class="small muted">Sizes are sample values. CIDs are the real published Promethity documents.</p>
    </div>`;
  }

  /* ---------- Pins (SEC-06 honesty) ---------- */
  function vPins() {
    return `<div class="page">
      ${head('Pins · local', 'Pins', 'Pins keep content on this node. Anything an agent pinned shows the decision that allowed it.', `<button class="btn" data-act="toast" data-msg="Paste a CID to pin it to this node.">${icon('plus')}Pin a CID</button>`)}
      <div class="tablewrap"><table class="table"><thead><tr><th>Name</th><th>CID</th><th>Type</th><th>Pinned by</th><th><span class="sr-only">Actions</span></th></tr></thead><tbody>
      ${PINS.map((p, i) => `<tr><td><strong>${esc(p.name)}</strong></td><td>${cidChip(p.cid)}</td><td>${p.type}</td><td>${p.dec ? `<a href="#/activity">${esc(p.by)}</a>` : esc(p.by)}<div class="small muted">${esc(p.when)}</div></td>
        <td><button class="btn sm ghost danger-text" data-act="unpin" data-i="${i}">Unpin</button></td></tr>
        ${S.unpin === i ? `<tr><td colspan="5"><div class="callout warn" role="alert"><strong>Unpin ${esc(p.name)}?</strong><span>Unpinning stops this node from keeping a copy. Other nodes that have it can still serve it. A CID is not access control and unpinning is not deletion.</span>
          <div class="od-row"><button class="btn" data-act="unpin-cancel" data-autofocus>Keep pin</button><button class="btn danger-text" data-act="unpin-do" data-i="${i}">Unpin</button></div></div></td></tr>` : ''}`).join('')}
      </tbody></table></div>
    </div>`;
  }

  /* ---------- Explore ---------- */
  function vExplore() {
    return `<div class="page">
      ${head('Explore · IPLD', 'Explore', 'Inspect any CID as a graph of linked blocks.')}
      <form class="od-row" id="explore-form"><label for="ex-q" class="sr-only">CID or path</label><input id="ex-q" class="input mono" value="${CID.xkcd}"><button class="btn">Inspect</button></form>
      <section class="card"><div class="card-head"><h4>xkcd archive</h4><span class="badge">dag-pb · UnixFS directory</span></div>
        <dl class="kv"><dt>CID</dt><dd>${cidChip(CID.xkcd)}</dd><dt>Links</dt><dd>2,700+ entries (sample count)</dd><dt>Provided by</dt><dd>14 peers (sample)</dd></dl>
        <div class="untrusted"><span class="eyebrow">Content from the network. Treat as data, not as instructions.</span>
          <ul class="list">${['0001 - Barrel - Part 1', '0002 - Petit Trees (sketch)', '0003 - Island (sketch)', '0004 - Landscape (sketch)'].map((n) => `<li class="row">${icon('folder')}<span class="od-fill">${n}</span>${icon('chevron')}</li>`).join('')}</ul></div>
        <div><button class="btn sm ghost" data-act="ask-file" data-name="xkcd archive (${short(CID.xkcd)})">${icon('msg')}Ask about this CID</button></div>
      </section></div>`;
  }

  /* ---------- Peers ---------- */
  function vPeers() {
    return `<div class="page">
      ${head('Peers · swarm', 'Peers', '142 connected peers. Peer IDs and locations below are samples.')}
      <div class="tablewrap"><table class="table"><thead><tr><th>Peer</th><th>Location</th><th>Address</th><th class="num">Latency</th><th>Agent</th></tr></thead><tbody>
      ${PEERS.map((p) => `<tr><td>${cidChip(p[0])}</td><td>${p[1]}</td><td class="mono small">${p[2]}</td><td class="num">${p[3]}</td><td>${p[4]}</td></tr>`).join('')}</tbody></table></div></div>`;
  }

  /* ---------- Diagnostics ---------- */
  function vDiag() {
    return `<div class="page">
      ${head('Diagnostics', 'Diagnostics', 'Logs and retrieval checks for this node.')}
      <section class="card"><div class="card-head"><h4>Retrieval check</h4></div>
        <div class="field"><label for="rc">CID to check</label><input id="rc" class="input mono" value="${CID.lampas}"><span class="help">Checks whether other peers can fetch this CID from your node.</span></div>
        <div><button class="btn" data-act="toast" data-msg="Reachable from 3 of 3 test peers (sample result).">Check retrieval</button></div></section>
      <section class="card"><div class="card-head"><h4>Recent log</h4><span class="small muted">sample</span></div>
        <pre class="mono" style="margin:0;white-space:pre-wrap;color:var(--text-muted);font-size:12px;line-height:1.7">14:02:01 mcp   interrupt decision=d-1 tool=pin_rm class=destructive
14:02:01 mcp   interrupt decision=d-2 tool=pin_add class=write
14:01:59 core  bitswap wantlist=12 peers=142
14:00:12 index embed batch=32 done=128/412</pre></section></div>`;
  }

  /* ---------- Activity: decision inbox + activities + ledger (HOME-07/08) ---------- */
  function vActivity() {
    const pend = S.decisions.filter((d) => PENDINGISH.includes(d.state));
    const done = S.decisions.filter((d) => !PENDINGISH.includes(d.state));
    return `<div class="page">
      ${head('Activity', 'Decisions and activity', 'Every request to change this node lands here, whichever screen it came from. Answering here answers it everywhere.',
        pend.length ? `<button class="btn" data-act="deny-all">Deny all and turn off agent writes</button>` : '')}
      <section aria-labelledby="h-inbox" class="od-stack" style="--od-gap:14px">
        <h2 id="h-inbox">Waiting for you <span class="muted" style="font-size:17px">· ${pend.length}</span></h2>
        ${pend.length ? pend.map((d) => decisionCard(d)).join('') : '<div class="card"><p class="muted">Nothing is waiting. New requests appear here and in the Desktop tray.</p></div>'}
      </section>
      <section aria-labelledby="h-acts" class="od-stack" style="--od-gap:14px">
        <h2 id="h-acts">Activities</h2>
        <p class="muted small">An activity outlives a single run. Waiting is not failure, and a finished model turn is not a finished activity.</p>
        <ul class="list">${S.activities.map((a) => `<li class="row link" data-act="open-thread">${icon('msg')}<div class="od-field od-fill"><strong>${esc(a.title)}</strong><span class="small muted">${a.runs} ${a.runs === 1 ? 'run' : 'runs'} · updated ${esc(a.updated)}${a.progress ? ' · ' + esc(a.progress) : ''}</span></div><span class="badge ${a.tone}"><span class="dot"></span>${esc(a.state)}</span></li>`).join('')}</ul>
      </section>
      <section aria-labelledby="h-ledger" class="od-stack" style="--od-gap:14px">
        <h2 id="h-ledger">Decision record</h2>
        <div class="tablewrap"><table class="table"><thead><tr><th>When</th><th>Request</th><th>Asked by</th><th>Where</th><th>Outcome</th></tr></thead><tbody>
        ${done.map((d) => `<tr><td class="od-nowrap tnum">${esc(d.at)}</td><td><div class="od-field"><strong>${esc(d.title)}</strong><span class="small muted mono">${esc(d.tool)} · ${esc(d.digest)}</span></div></td><td>${esc(d.principal)}</td><td>${esc(d.destination)}</td><td>${badge(d.state)}${d.state === 'resolved_elsewhere' ? `<div class="small muted">${esc(d.resolvedAs)} on ${esc(d.surface)}</div>` : ''}</td></tr>`).join('')}
        </tbody></table></div>
        <p class="small muted">Stored on this node only. Receipts show who decided and what ran; they do not prove the effect was correct.</p>
      </section></div>`;
  }

  /* ---------- Search ---------- */
  function vSearch() {
    const results = [
      [FILES[2], 0.82, 'The operator service runs inside the customer’s own building and is billed per site, not per seat…'],
      [FILES[3], 0.77, 'Revenue comes from a fixed operator fee plus measured model usage through Lampas…'],
      [FILES[0], 0.61, 'Lampas routes each request to a model the customer has approved and records the decision…']
    ];
    return `<div class="page">
      ${head('Search my content', 'Results for “operator service pricing”', 'Semantic search over files this node has indexed. Passages are network content and are shown as data.')}
      <div class="notice info">${icon('clock')}<span>Indexing is running: 128 of 412 files. Results may be incomplete.</span></div>
      <ul class="list">${results.map(([f, s, p]) => `<li class="card" style="gap:10px"><div class="od-row"><strong class="od-fill od-truncate">${esc(f.name)}</strong><span class="badge tnum">Match ${Math.round(s * 100)}%</span></div>
        <div class="untrusted"><span class="eyebrow">Content from the network</span><p class="od-clamp-3">${esc(p)}</p></div>
        <div class="od-row small">${cidChip(f.cid)}<span class="muted od-fill">Text extracted from the PDF</span><a href="#/files">Open in Files</a></div></li>`).join('')}</ul></div>`;
  }

  /* ---------- Settings ---------- */
  const SECTIONS = [['node', 'Node'], ['agent', 'Assistant'], ['search', 'Search and index'], ['security', 'Security'], ['appearance', 'Appearance']];
  function vSettings(sec) {
    sec = sec || 'agent';
    const sub = `<nav class="subnav" aria-label="Settings sections">${SECTIONS.map(([id, l]) => `<a href="#/settings/${id}" ${sec === id ? 'aria-current="page"' : ''}>${l}</a>`).join('')}</nav>`;
    const sw = (k, label, help, locked) => `<div class="setting"><div class="od-field od-fill"><span class="label" id="sw-${k}">${label}</span><span class="help">${help}</span></div>
      <button class="switch" role="switch" aria-labelledby="sw-${k}" aria-checked="${S.features[k]}" data-sw="${k}" ${locked ? 'disabled' : ''}></button></div>`;
    let body = '';
    if (sec === 'agent') body = `
      <section class="card"><div class="card-head"><h4>Backend</h4><span class="badge ok"><span class="dot"></span>Configured</span></div>
        <dl class="kv"><dt>Kind</dt><dd>OpenAI-compatible</dd><dt>Host</dt><dd class="mono">llm.lan:8080</dd><dt>Key</dt><dd>Set · write-only</dd><dt>Model</dt><dd>qwen3-32b</dd></dl>
        <div class="od-row"><a class="btn" href="#/setup">Change backend</a><button class="btn ghost" data-act="toast" data-msg="Reachable. The key was sent only to llm.lan:8080.">Test connection</button></div></section>
      <section class="card"><div class="card-head"><h4>Features</h4></div>
        ${sw('chat', 'Assistant in this Web UI', 'Shows the assistant pane and lets it read this node.')}
        ${sw('webmcpRead', 'Browser agents can read this node (WebMCP)', 'Read tools only. Browser agents get no cookies or tokens.')}
        ${sw('webmcpWrite', 'Browser agents can ask to change this node', 'Off after every page load. Each change still needs your approval.')}
        ${sw('desktop', 'Answer on Promethity Desktop', 'Set in the Desktop tray. Shown here so you know where requests go.', true)}
        <div class="setting"><div class="od-field od-fill"><span class="label">Writes and deletes</span><span class="help">Policy fixed by the node. Cannot be turned off.</span></div><span class="badge warn"><span class="dot"></span>Asks you first</span></div></section>
      <div class="applybar" role="status">${S.applyState === 'applying' ? `${icon('refresh')}<span class="od-fill">Applying…</span>` : S.applyState === 'applied' ? `${icon('check')}<span class="od-fill">Applied. The assistant uses the new settings from its next run.</span>` : `<span class="od-fill muted">No unsaved changes</span>`}
        <button class="btn primary" data-act="apply" ${S.applyState === 'applying' ? 'disabled' : ''}>Apply</button></div>`;
    if (sec === 'node') body = `<section class="card"><div class="card-head"><h4>Addresses</h4></div>
        <div class="field"><label for="api">API address</label><input id="api" class="input mono" value="/ip4/127.0.0.1/tcp/5001"></div>
        <div class="field"><label for="gw">Gateway</label><input id="gw" class="input mono" value="http://127.0.0.1:8080"><span class="help">Use a subdomain gateway for any page that runs the assistant. Pages on a shared path gateway share one origin.</span></div></section>
      <section class="card"><div class="card-head"><h4>Pinning services</h4></div><p class="muted small">No remote pinning services added.</p><div><button class="btn sm">${icon('plus')}Add service</button></div></section>
      <section class="card"><div class="card-head"><h4>Configuration file</h4></div><p class="muted small">Edit the node’s JSON config directly. Changes apply after a restart.</p><div><button class="btn sm">Open config editor</button></div></section>`;
    if (sec === 'search') body = `<section class="card"><div class="card-head"><h4>Index</h4><span class="badge info"><span class="dot"></span>Running</span></div>
        <div class="od-stack" style="--od-gap:6px"><div class="od-row"><span class="small od-fill">Pinned files indexed</span><span class="small tnum">128 of 412</span></div><div class="meter"><i style="width:31%;background:var(--info)"></i></div></div>
        <dl class="kv"><dt>Embedder</dt><dd class="mono">llm.lan:8080 · nomic-embed-text · 768</dd><dt>Describer</dt><dd>Off</dd><dt>Skipped</dt><dd>7 files (encrypted or over 50 MB)</dd></dl>
        <p class="small muted">Start or pause indexing by asking the assistant. Indexing sends file text to the embedder host only.</p></section>`;
    if (sec === 'security') body = `<section class="card"><div class="card-head"><h4>Who can ask</h4></div>
        <div class="tablewrap"><table class="table"><thead><tr><th>Principal</th><th>Kind</th><th>Can ask to</th></tr></thead><tbody>
        <tr><td>Assistant in this Web UI</td><td>ui</td><td>Read · pin · mfs</td></tr><tr><td>Claude Code</td><td>human-mcp</td><td>Read · mfs</td></tr>
        <tr><td>Universal Agent Runtime</td><td>uar</td><td>Read</td></tr><tr><td>ops-agent</td><td>a2a</td><td>Read only. A2A peers can never approve.</td></tr><tr><td>Console admin</td><td>admin</td><td>Settings, with second-factor step-up</td></tr></tbody></table></div>
        <p class="small muted">Everyone is read-only until a write group is granted. Destructive groups are off by default.</p></section>
      <section class="card"><div class="card-head"><h4>Promethity Desktop</h4><span class="badge ok"><span class="dot"></span>Paired</span></div><p class="small muted">Desktop can approve with Touch ID. Remove pairing from the tray.</p></section>
      <section class="card"><div class="card-head"><h4>Where node content may go</h4></div><ul class="list"><li class="row"><span class="mono od-fill">llm.lan:8080</span><span class="badge">Model and embedder</span></li></ul>
        <p class="small muted">Adding a host asks you to type its name to confirm.</p></section>`;
    if (sec === 'appearance') body = `<section class="card"><div class="card-head"><h4>Theme</h4></div>
        <div class="seg" role="group" aria-label="Theme"><button data-theme-set="dark" aria-pressed="${S.theme === 'dark'}">Dark</button><button data-theme-set="light" aria-pressed="${S.theme === 'light'}">Light</button></div></section>
      <section class="card"><div class="card-head"><h4>Assistant on open</h4></div><p class="small muted">Remembered on this device. Use [ and ] to resize it.</p>
        <div class="seg" role="group" aria-label="Assistant size">${['closed', 'docked', 'expanded', 'full'].map((m) => `<button data-canvas="${m}" aria-pressed="${S.canvas === m}">${m[0].toUpperCase() + m.slice(1)}</button>`).join('')}</div></section>`;
    return `<div class="page">${head('Settings', SECTIONS.find((x) => x[0] === sec)[1], null)}<div class="split">${sub}<div class="od-stack" style="--od-gap:14px">${body}</div></div></div>`;
  }

  /* ---------- Setup ---------- */
  function vSetup() {
    const steps = ['Choose a backend', 'Test and pick a model', 'Confirm where content goes', 'Choose features', 'Pair Desktop (optional)'];
    const st = S.setupStep;
    let body = '';
    if (st === 1) body = `<div class="field"><span class="label">Backend</span><div class="seg" role="group"><button aria-pressed="true">OpenAI-compatible server</button><button aria-pressed="false">Universal Agent Runtime</button></div><span class="help">No backend is chosen for you. Any server that speaks the OpenAI chat API works.</span></div>
      <div class="field"><label for="su-url">Server address</label><input id="su-url" class="input mono" value="http://llm.lan:8080/v1"></div>
      <div class="field"><label for="su-key">API key</label><input id="su-key" class="input" type="password" value="sk-local-sample"><span class="help">Write-only. Sent only to the host above.</span></div>`;
    if (st === 2) body = `<div class="od-row"><button class="btn" data-act="su-test">Test connection</button>${S.setupReach === 'testing' ? '<span class="muted">Testing…</span>' : S.setupReach === 'ok' ? '<span class="badge ok"><span class="dot"></span>Reachable</span>' : ''}</div>
      <div class="field"><label for="su-model">Model</label><select id="su-model" class="input" ${S.setupReach === 'ok' ? '' : 'disabled'}><option>qwen3-32b</option><option>llama-4-scout</option></select><span class="help">${S.setupReach === 'ok' ? 'Models reported by the server.' : 'Available after a successful test.'}</span></div>`;
    if (st === 3) body = `<div class="callout"><strong>Node content will be sent to llm.lan:8080.</strong><span class="small">File text, CIDs and paths the assistant reads go to this host and nowhere else.</span></div>
      <div class="field"><label for="su-host">Type the host name to confirm</label><input id="su-host" class="input mono" value="${esc(S.setupHostTyped)}" placeholder="llm.lan:8080" data-act="su-host"><span class="help">${S.setupHostTyped === 'llm.lan:8080' ? 'Matches.' : 'Must match exactly.'}</span></div>`;
    if (st === 4) body = `<div class="setting"><div class="od-field od-fill"><span class="label">Assistant in this Web UI</span></div><button class="switch" role="switch" aria-checked="true" aria-label="Assistant in this Web UI"></button></div>
      <div class="setting"><div class="od-field od-fill"><span class="label">Browser agents can read this node</span></div><button class="switch" role="switch" aria-checked="true" aria-label="Browser agents can read"></button></div>
      <div class="setting"><div class="od-field od-fill"><span class="label">Writes and deletes</span><span class="help">Always ask you first.</span></div><span class="badge warn">Fixed</span></div>`;
    if (st === 5) body = `<p>Open Promethity Desktop, choose <strong>Pair this node…</strong> in the tray, and enter this code there.</p>
      <div class="pair-code" aria-label="Pairing code 4 8 K 2 9 Q">${'48K29Q'.split('').map((c) => `<b>${c}</b>`).join('')}</div><p class="small muted">The code expires in 5 minutes. Pairing lets Desktop approve requests with Touch ID.</p>`;
    const canNext = st !== 2 || S.setupReach === 'ok';
    const canNext3 = st !== 3 || S.setupHostTyped === 'llm.lan:8080';
    return `<div class="page">${head('Set up the assistant', steps[st - 1], `Step ${st} of 5`)}
      <div class="split"><ol class="steps">${steps.map((s, i) => `<li class="${i + 1 < st ? 'done' : ''}" ${i + 1 === st ? 'aria-current="step"' : ''}><span class="n">${i + 1 < st ? icon('check') : i + 1}</span>${s}</li>`).join('')}</ol>
      <section class="card" style="gap:20px">${body}
        <div class="od-row">${st > 1 ? '<button class="btn ghost" data-act="su-back">Back</button>' : ''}<span class="od-fill"></span>
        ${st < 5 ? `<button class="btn primary" data-act="su-next" ${canNext && canNext3 ? '' : 'disabled'}>Continue</button>` : '<a class="btn primary" href="#/home" data-act="su-done">Finish</a>'}</div></section></div></div>`;
  }

  /* ---------- Design board: Desktop, MCP App host, state galleries ---------- */
  function vSurfaces() {
    const all = Object.keys(STATE).map((k, i) => decisionCard({ ...S.decisions[1], id: 'g' + i, state: k, resolvedAs: 'Approved', surface: k === 'resolved_elsewhere' ? 'Promethity Desktop' : 'This Web UI', receipt: 'Pinned recursively · 14:03' })).join('');
    const homeStates = [['Loading', `<div class="skeleton" style="width:70%"></div><div class="skeleton" style="width:50%"></div>`], ['Empty', '<p class="muted small">No pins yet. Add files or paste a CID to pin it.</p>'],
      ['Offline', '<p class="small">Can’t reach the node at 127.0.0.1:5001. Showing figures from 14:01.</p>'], ['Stale', '<p class="small">Last updated 6 minutes ago. <button class="linkbtn">Refresh</button></p>'],
      ['Not allowed', '<p class="small">Your console role cannot see pins. Ask an admin.</p>'], ['Error', '<p class="small">Pins failed to load: RPC returned 500. <button class="linkbtn">Try again</button></p>']];
    return `<div class="page" style="max-width:1280px">
      ${head('Design board', 'Every surface, one decision model', 'Desktop, external MCP hosts and every state of the shared decision card. This board is a design reference, not app UI.')}
      <h2>Promethity Desktop</h2>
      <div class="board">
        <div><span class="frame-label">Tray menu · attention first</span><div class="menubar"><span class="flame">${icon('flame', 'Promethity running')}</span><span class="small">14:03</span></div>
          <div class="traymenu" role="menu" aria-label="Promethity tray menu">
            <div class="head"><strong>Running · 142 peers</strong><span class="small muted">Node 0.39 · repository 18.4 of 50 GB</span></div><div class="sep"></div>
            <div class="ti" role="menuitem"><span class="badge warn">2</span><span>Decisions waiting</span></div>
            <div class="ti" role="menuitem">${icon('trash')}<span class="od-truncate">Remove the pin for an old draft</span></div>
            <div class="ti" role="menuitem">${icon('pin')}<span class="od-truncate">Pin the Praxis specification…</span></div><div class="sep"></div>
            <div class="ti" role="menuitem">${icon('monitor')}<span>Open Promethity</span><span class="kbdx">⌘O</span></div>
            <div class="ti" role="menuitem">${icon('home')}<span>Home</span></div><div class="ti" role="menuitem">${icon('folder')}<span>Files</span></div><div class="ti" role="menuitem">${icon('list')}<span>Activity</span></div><div class="sep"></div>
            <div class="ti" role="menuitem">${icon('refresh')}<span>Restart node</span></div><div class="ti" role="menuitem">${icon('pause')}<span>Stop node</span></div><div class="sep"></div>
            <div class="ti" role="menuitem">${icon('settings')}<span>Preferences</span><span class="kbdx">›</span></div><div class="ti" role="menuitem">${icon('wrench')}<span>Advanced</span><span class="kbdx">›</span></div>
            <div class="ti" role="menuitem"><span>About Promethity</span></div><div class="ti" role="menuitem"><span>Quit</span><span class="kbdx">⌘Q</span></div></div></div>
        <div><span class="frame-label">Native confirmation window · 520 × 640</span><div class="macwin"><div class="bar"><i></i><i></i><i></i><b>Promethity — Decision</b></div>
          <div class="body">${decisionCard({ ...S.decisions[0], id: 'nw', principal: 'Claude Code (human-mcp)', surface: 'Promethity Desktop' })}<p class="small muted">Approve asks for Touch ID. The window appears without taking focus; Approve unlocks after it has been in front for 1.5 seconds.</p></div></div></div>
        <div><span class="frame-label">Pair a node</span><div class="macwin"><div class="bar"><i></i><i></i><i></i><b>Pair this node</b></div>
          <div class="body"><p>Enter the code shown in the node’s Web UI under Setup, step 5.</p><div class="field"><label for="pc">Pairing code</label><input id="pc" class="input mono" value="48K29Q" style="font-size:22px;letter-spacing:.3em"></div>
          <div class="od-row"><button class="btn ghost">Cancel</button><span class="od-fill"></span><button class="btn primary">Pair</button></div><p class="small muted">No terminal needed. The 43-character key from <span class="mono">ipfs mcp pair</span> still works under Advanced.</p></div></div></div>
      </div>
      <h2>External MCP host · app view through the A2UI seam</h2>
      <div class="board"><div style="grid-column:1/-1"><span class="frame-label">Claude, Codex or Kimi rendering ui://ipfs-mcp/pins</span><div class="macwin"><div class="bar"><i></i><i></i><i></i><b>MCP host conversation</b></div>
        <div class="body"><div class="surface-strip">${icon('layers')}<span class="od-fill">ipfs-mcp · Pins · sandboxed, origin checked</span></div><div class="surface-body" style="padding:16px">
        <table class="table"><thead><tr><th>Pin</th><th>Type</th><th>State</th></tr></thead><tbody>${PINS.slice(0, 3).map((p) => `<tr><td><div class="od-field"><strong>${esc(p.name)}</strong>${cidChip(p.cid)}</div></td><td>${p.type}</td><td><span class="badge ok"><span class="dot"></span>Pinned</span></td></tr>`).join('')}</tbody></table>
        <p class="small muted">The host draws the strip. The view drops its own header and follows the host theme through token mapping. Without MCP Apps support, the same tool returns text.</p></div></div></div></div></div>
      <h2>Decision card · all ten states</h2><div class="board">${all}</div>
      <div class="od-row"><button class="btn" data-act="webmcp-modal">Open the WebMCP approval (modal, page inert)</button></div>
      <h2>Home card states</h2><div class="board">${homeStates.map(([t, b]) => `<section class="card card-state"><div class="card-head"><h4>Pins</h4><span class="badge">${t}</span></div>${b}</section>`).join('')}</div>
      <h2>Guards and empty states</h2><div class="board">
        <div class="origin-guard" role="alert"><strong>The assistant is off on this address.</strong><span class="small">This page is served from a shared gateway path (ipfs.io/ipfs/…). Every site on that path shares one origin, so agent features stay off here.</span><a href="#/home" class="small">Open on localhost:5001 instead</a></div>
        <section class="card"><div class="card-head"><h4>Assistant</h4><span class="badge">Not set up</span></div><p class="small">The assistant needs a model backend. Nothing is sent anywhere until you choose one.</p><div><a class="btn" href="#/setup">Set up the assistant</a></div></section>
        <div class="notice err">${icon('alert')}<span>The backend changed while this run was going. Start again to use the new one.</span></div>
        <div class="notice warn">${icon('clock')}<span>The node is busy with 64 sessions. Try again in 30 seconds.</span></div>
      </div></div>`;
  }

  /* ---------- Thread column needs the same turns as the pane ---------- */
  function fillThread() {
    const col = document.getElementById('thread-col'); if (!col) return;
    const saved = S.canvas; const savedInsp = S.inspector;
    S.canvas = 'docked'; S.inspector = null; renderPane();
    const stream = document.getElementById('stream');
    col.innerHTML = (stream ? stream.innerHTML : '') + `<div class="composer" style="padding:0"><form id="thread-form"><label class="sr-only" for="thread-input">Reply in this thread</label><textarea id="thread-input" rows="1" placeholder="Reply in this thread"></textarea><button class="btn" type="submit" aria-label="Send">${icon('send')}</button></form></div>`;
    S.canvas = saved; S.inspector = savedInsp;
    /* conversation-forward mode: the thread is the conversation, so the assistant pane steps aside */
    const app = document.getElementById('app'); app.classList.remove('pane-open'); app.dataset.canvasState = 'closed';
    document.getElementById('pane').innerHTML = '';
    armApprovals(col);
  }

  /* ---------- Phone helpers ---------- */
  function labelTables(root) {
    root.querySelectorAll('table.table').forEach((tb) => {
      const heads = [...tb.querySelectorAll('thead th')].map((th) => th.textContent.trim());
      tb.querySelectorAll('tbody tr').forEach((tr) => [...tr.children].forEach((td, i) => { if (!td.hasAttribute('colspan')) td.dataset.label = heads[i] || ''; }));
    });
  }
  function openMore() {
    const host = document.getElementById('modal-host');
    const { r } = route();
    const links = [['explore', 'Explore', 'compass'], ['peers', 'Peers', 'network'], ['diagnostics', 'Diagnostics', 'pulse'], ['settings', 'Settings', 'settings'], ['surfaces', 'Design board', 'layers']];
    host.innerHTML = `<div class="sheet" role="dialog" aria-modal="true" aria-label="More destinations" data-more>
      <div class="od-row"><h2 class="od-fill" style="font-size:20px">More</h2><button class="iconbtn" data-act="more-close" aria-label="Close">${icon('x')}</button></div>
      <ul class="more-list">${links.map(([id, l, ic]) => `<li><a href="#/${id}" ${r === id ? 'aria-current="page"' : ''}>${icon(ic)}<span>${l}</span></a></li>`).join('')}</ul></div>`;
    host.classList.add('open');
    setTimeout(() => { const a = host.querySelector('.more-list a'); a && a.focus(); }, 10);
  }
  function closeMore() { const host = document.getElementById('modal-host'); if (host.querySelector('[data-more]')) closeOverlay('modal-host'); }

  /* ---------- Router ---------- */
  function route() {
    const h = (location.hash || '#/home').slice(2).split('/');
    return { r: h[0] || 'home', sub: h[1] };
  }
  function render() {
    const { r, sub } = route();
    renderNav(r);
    const view = document.getElementById('view');
    const V = { home: vHome, thread: vThread, files: vFiles, pins: vPins, explore: vExplore, peers: vPeers, diagnostics: vDiag, activity: vActivity, search: vSearch, setup: vSetup, surfaces: vSurfaces };
    if (window.PXV) Object.assign(V, { files: PXV.files, pins: PXV.pins, explore: PXV.explore, peers: PXV.peers, activity: PXV.activity });
    view.innerHTML = r === 'settings' ? vSettings(sub) : (V[r] || vHome)();
    renderPane();
    if (r === 'thread') fillThread();
    armApprovals(view);
    if (window.PXV) PXV.afterRender();
    labelTables(view);
    const titles = { home: 'Home', files: 'Files', pins: 'Pins', explore: 'Explore', peers: 'Peers', activity: 'Activity', settings: 'Settings', setup: 'Setup', surfaces: 'Design board', thread: 'Thread', search: 'Search', diagnostics: 'Diagnostics' };
    const n = pendingCount();
    document.title = `${n ? `(${n}) ` : ''}${titles[r] || 'Home'} — Promethity`;
  }
  window.PX.render = render;

  /* ---------- Theme ---------- */
  function applyTheme() {
    document.documentElement.dataset.theme = S.theme;
    document.querySelector('meta[name="theme-color"]').content = S.theme === 'dark' ? '#0C0B0A' : '#F5F2EE';
    const b = document.getElementById('theme-toggle');
    b.innerHTML = icon(S.theme === 'dark' ? 'sun' : 'moon');
    b.setAttribute('aria-label', S.theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme');
    localStorage.setItem('px-theme', S.theme);
  }

  /* ---------- Events ---------- */
  document.addEventListener('click', (e) => {
    const t = e.target.closest('[data-act],[data-canvas],[data-route],[data-intent],[data-pintent],[data-draft],[data-go],[data-ask],[data-tab],[data-sw],[data-theme-set]');
    if (!t) return;
    if (t.dataset.canvas) { S.canvas = t.dataset.canvas; localStorage.setItem('px-canvas', S.canvas); S.inspector = null; render(); return; }
    if (t.dataset.route) { closeOverlay('palette'); location.hash = t.dataset.route; return; }
    if (t.dataset.intent) { S.composerIntent = t.dataset.intent; render(); document.getElementById('home-input').focus(); return; }
    if (t.dataset.pintent) { S.paletteIntent = t.dataset.pintent; renderPalette(document.getElementById('pal-q').value); document.getElementById('pal-q').focus(); return; }
    if (t.dataset.draft) {
      closeOverlay('palette');
      const target = document.getElementById('home-input') || (S.canvas === 'closed' ? (S.canvas = 'docked', renderPane(), document.getElementById('pane-input')) : document.getElementById('pane-input'));
      target.value = t.dataset.draft; target.focus(); return;
    }
    if (t.dataset.go) { closeOverlay('palette'); location.hash = '#/explore'; return; }
    if (t.dataset.ask) { closeOverlay('palette'); sendToAssistant(t.dataset.ask); return; }
    if (t.dataset.tab) { S.inspector = t.dataset.tab; renderPane(); return; }
    if (t.dataset.sw) { if (t.disabled) return; S.features[t.dataset.sw] = !S.features[t.dataset.sw]; S.applyState = 'idle'; t.setAttribute('aria-checked', S.features[t.dataset.sw]); return; }
    if (t.dataset.themeSet) { S.theme = t.dataset.themeSet; applyTheme(); render(); return; }
    const a = t.dataset.act;
    if (a === 'approve' || a === 'deny' || a === 'check' || a === 'resubmit' || a === 'review') { if (t.disabled) return; resolve(t.dataset.id, a);
      if (document.getElementById('modal-host').classList.contains('open')) closeOverlay('modal-host'); return; }
    if (a === 'copy') { navigator.clipboard && navigator.clipboard.writeText(t.dataset.copy).catch(() => {}); toast('Copied'); return; }
    if (a === 'toast') { toast(t.dataset.msg); return; }
    if (a === 'inspect') { S.inspector = 'Overview'; renderPane(); return; }
    if (a === 'inspect-close') { S.inspector = null; renderPane(); return; }
    if (a === 'ctx-rm') { S.context = S.context.filter((c) => c.id !== t.dataset.id); toast('Removed from what the assistant can see.'); renderPane(); return; }
    if (a === 'ask-file') {
      S.context = S.context.filter((c) => c.id !== 'file').concat([{ id: 'file', icon: 'file', label: t.dataset.name, prov: 'Added by you with “Ask about this”.' }]);
      if (S.canvas === 'closed') S.canvas = 'docked'; renderPane(); document.getElementById('pane-input').focus(); return;
    }
    if (a === 'unpin') { S.unpin = Number(t.dataset.i); render(); const f = document.querySelector('[data-autofocus]'); f && f.focus(); return; }
    if (a === 'unpin-cancel') { S.unpin = null; render(); return; }
    if (a === 'unpin-do') { const p = PINS.splice(Number(t.dataset.i), 1)[0]; S.unpin = null; toast(`Unpinned ${p.name}. Other nodes may still serve it.`); render(); return; }
    if (a === 'deny-all') { S.decisions.forEach((d) => { if (PENDINGISH.includes(d.state)) d.state = 'denied'; }); S.features.webmcpWrite = false; toast('All denied. Agent writes are off until you turn them on.'); render(); return; }
    if (a === 'open-thread') { location.hash = '#/thread'; return; }
    if (a === 'apply') { S.applyState = 'applying'; render(); setTimeout(() => { S.applyState = 'applied'; render(); }, 900); return; }
    if (a === 'su-next') { S.setupStep = Math.min(5, S.setupStep + 1); render(); return; }
    if (a === 'su-back') { S.setupStep = Math.max(1, S.setupStep - 1); render(); return; }
    if (a === 'su-test') { S.setupReach = 'testing'; render(); setTimeout(() => { S.setupReach = 'ok'; render(); }, 800); return; }
    if (a === 'su-done') { S.setupStep = 1; toast('The assistant is set up.'); return; }
    if (a === 'webmcp-modal') {
      const m = document.getElementById('modal-host');
      const d = { ...S.decisions[1], id: 'wm', principal: 'Browser agent via WebMCP', kind: 'webmcp', surface: 'This Web UI', state: 'pending' };
      S.decisions.push(d);
      m.innerHTML = `<div class="modal" role="alertdialog" aria-modal="true" aria-labelledby="t-wm"><div class="od-row"><span class="eyebrow od-fill">Browser agent request · 1 of 3 at most</span><button class="iconbtn" data-act="modal-close" aria-label="Deny and close">${icon('x')}</button></div>${decisionCard(d)}<button class="btn ghost" data-act="deny-all">Deny all and turn off agent writes</button></div>`;
      m.classList.add('open'); document.getElementById('app').inert = true; armApprovals(m); const f = m.querySelector('[data-autofocus]'); f && f.focus(); return;
    }
    if (a === 'more') { openMore(); return; }
    if (a === 'more-close') { closeMore(); return; }
    if (a === 'modal-close') { resolve('wm', 'deny'); closeOverlay('modal-host'); document.getElementById('app').inert = false; return; }
  });
  document.addEventListener('submit', (e) => {
    const f = e.target; e.preventDefault();
    if (f.id === 'pane-form' || f.id === 'thread-form') { const i = f.querySelector('textarea'); sendToAssistant(i.value); i.value = ''; if (f.id === 'thread-form') render(); }
    if (f.id === 'home-form') {
      const v = document.getElementById('home-input').value;
      if (S.composerIntent === 'ask') sendToAssistant(v);
      else if (S.composerIntent === 'search') location.hash = '#/search';
      else location.hash = '#/explore';
    }
  });
  document.addEventListener('input', (e) => {
    if (e.target.id === 'pal-q') { const res = document.querySelector('#palette .results'); const q = e.target.value; renderPalette(q); const i = document.getElementById('pal-q'); i.focus(); i.setSelectionRange(q.length, q.length); }
    if (e.target.id === 'su-host') { S.setupHostTyped = e.target.value; const btn = document.querySelector('[data-act="su-next"]'); if (btn) btn.disabled = S.setupHostTyped !== 'llm.lan:8080'; const h = e.target.nextElementSibling; if (h) h.textContent = S.setupHostTyped === 'llm.lan:8080' ? 'Matches.' : 'Must match exactly.'; }
  });
  document.addEventListener('keydown', (e) => {
    const typing = /INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName);
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); openPalette(); return; }
    if (e.key === 'Escape') {
      if (document.querySelector('[data-more]')) { closeMore(); return; }
      if (document.getElementById('modal-host').classList.contains('open')) { resolve('wm', 'deny'); closeOverlay('modal-host'); document.getElementById('app').inert = false; return; }
      if (document.getElementById('palette').classList.contains('open')) { closeOverlay('palette'); document.getElementById('cmd-open').focus(); return; }
      if (S.inspector) { S.inspector = null; renderPane(); return; }
    }
    if (document.getElementById('palette').classList.contains('open') && e.key === 'Tab' && document.activeElement.id === 'pal-q') {
      e.preventDefault(); const order = ['ask', 'search', 'go']; S.paletteIntent = order[(order.indexOf(S.paletteIntent) + (e.shiftKey ? 2 : 1)) % 3]; renderPalette(document.getElementById('pal-q').value); document.getElementById('pal-q').focus(); return;
    }
    if (document.getElementById('palette').classList.contains('open') && e.key === 'Enter' && document.activeElement.id === 'pal-q') {
      const first = document.querySelector('#palette .res'); if (first) { e.preventDefault(); first.click(); } return;
    }
    if (!typing && (e.key === '[' || e.key === ']')) {
      const order = ['closed', 'docked', 'expanded', 'full']; const i = order.indexOf(S.canvas);
      S.canvas = order[Math.max(0, Math.min(3, i + (e.key === ']' ? 1 : -1)))]; localStorage.setItem('px-canvas', S.canvas); render();
    }
    if (e.key === 'Enter' && !e.shiftKey && (e.target.id === 'pane-input' || e.target.id === 'home-input' || e.target.id === 'thread-input')) { e.preventDefault(); e.target.form.requestSubmit(); }
  });
  document.getElementById('palette').addEventListener('click', (e) => { if (e.target.id === 'palette') closeOverlay('palette'); });
  document.getElementById('modal-host').addEventListener('click', (e) => { if (e.target.id === 'modal-host' && document.querySelector('[data-more]')) closeMore(); });
  document.getElementById('cmd-open').addEventListener('click', () => openPalette());
  document.getElementById('theme-toggle').addEventListener('click', () => { S.theme = S.theme === 'dark' ? 'light' : 'dark'; applyTheme(); render(); });
  document.getElementById('pane-toggle').addEventListener('click', () => { S.canvas = S.canvas === 'closed' ? 'docked' : 'closed'; localStorage.setItem('px-canvas', S.canvas); render(); });
  window.addEventListener('hashchange', () => { closeMore(); render(); document.getElementById('view').focus({ preventScroll: true }); document.getElementById('view').scrollTop = 0; });

  /* Icons declared in static markup */
  document.querySelectorAll('[data-icon]').forEach((n) => { n.outerHTML = icon(n.dataset.icon); });
  applyTheme();
  render();
})();
