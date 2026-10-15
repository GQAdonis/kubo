import { launchChrome, newPage, sleep } from './cdp.mjs'
import { writeFileSync, appendFileSync } from 'node:fs'
const OUT = '/Users/gqadonis/Projects/references/kubo/mcp/server/evidence/2026-10-14-round5-route-polish'
const BASE = 'http://localhost:3199/#'
const only = process.argv[2] ? process.argv[2].split(',') : null
const log = (m) => { console.log(m); appendFileSync(`${OUT}/capture.log`, m + '\n') }
const browser = await launchChrome(9199)
log(`chrome pid ${browser.pid}`)
writeFileSync(`${OUT}/.chrome.pid`, String(browser.pid))

const init = (theme, api = '/ip4/127.0.0.1/tcp/5199', stub = false) => `
  localStorage.setItem('ipfsApi', '${api}');
  localStorage.setItem('ipfs-webui:theme:v1', '${theme}');
  localStorage.setItem('i18nextLng', 'en');
  localStorage.setItem('showTours', 'false');
  localStorage.setItem('toursEnabled', 'false');
  ${stub ? `localStorage.setItem('ipfs-webui:webmcp', JSON.stringify({enabled:true,dnslink:false}));
  window.__tools = {};
  Object.defineProperty(document, 'modelContext', { configurable: true, value: { registerTool(tool){ window.__tools[tool.name] = tool }, unregisterTool(){}, provideContext(){} } });` : ''}
`
const SIZES = [[1440, 900], [375, 812], [320, 640]]
const THEMES = ['dark', 'light']
const want = (k) => !only || only.includes(k)
async function withPage (theme, w, h, opts, fn) {
  const page = await newPage(browser, { width: w, height: h, initScript: init(theme, opts.api, opts.stub) })
  try { await fn(page) } catch (e) { log(`ERR ${theme} ${w} ${e.message}`) } finally { await page.close() }
}
const shotFull = async (page, f) => page.shot(`${OUT}/${f}`, { full: true })

// 1. Plain routes
const routes = { files: '/files', pins: '/pins', peers: '/peers', diagnostics: '/diagnostics/logs', welcome: '/welcome', explore: '/explore', 'explore-cid': '/explore/QmdEdjFrEkrdZXabTNZD9mev786YqFQSkq6faDKuwZP2Jv' }
for (const [name, route] of Object.entries(routes)) {
  if (!want(name)) continue
  for (const theme of THEMES) for (const [w, h] of SIZES) {
    await withPage(theme, w, h, {}, async (page) => {
      await page.goto(BASE + route, name === 'explore-cid' ? 20000 : name.startsWith('explore') ? 8000 : 4500)
      await shotFull(page, `${name}-${theme}-${w}.png`)
      log(`ok ${name} ${theme} ${w}`)
    })
  }
}

// 2. Files: new-folder modal, context menu, selected row
if (want('modal')) for (const theme of THEMES) for (const [w, h] of SIZES) {
  await withPage(theme, w, h, {}, async (page) => {
    await page.goto(BASE + '/files', 4500)
    await page.click('#import-button')
    await sleep(500)
    const clicked = await page.eval(`(() => { const el=[...document.querySelectorAll('button, [role=menuitem], div')].find(e => e.children.length<4 && /new folder/i.test(e.textContent||'') && e.textContent.length<30); if(!el) return false; el.click(); return el.textContent })()`)
    await sleep(700)
    await page.shot(`${OUT}/modal-new-folder-${theme}-${w}.png`)
    log(`modal ${theme} ${w} ${clicked}`)
    const m = await page.eval(`(() => { const m=document.querySelector('[data-testid=ipfs-modal]'); if(!m) return null; const r=m.getBoundingClientRect(); return {left:r.left,right:r.right,vw:innerWidth, btns:[...m.querySelectorAll('button')].map(b=>{const q=b.getBoundingClientRect(); return [Math.round(q.left),Math.round(q.right)]}), sw: document.documentElement.scrollWidth} })()`)
    log(`modal-metrics ${theme} ${w} ${JSON.stringify(m)}`)
  })
}
if (want('ctx')) for (const theme of THEMES) {
  await withPage(theme, 1440, 900, {}, async (page) => {
    await page.goto(BASE + '/files', 4500)
    // select a row, then open the context menu of the first file
    await page.eval(`(() => { const c=document.querySelectorAll('[data-testid=file-row] input[type=checkbox]'); if(c[2]) c[2].click() })()`)
    await sleep(500)
    await page.shot(`${OUT}/files-selected-${theme}-1440.png`)
    await page.eval(`(() => { const b=document.querySelector('.file-context-menu'); if(b) b.click() })()`)
    await sleep(600)
    await page.shot(`${OUT}/files-context-menu-${theme}-1440.png`)
    log(`ctx ${theme}`)
  })
}

// 3. WebMCP dialog
if (want('webmcp')) for (const theme of THEMES) for (const [w, h] of SIZES) {
  await withPage(theme, w, h, { stub: true }, async (page) => {
    await page.goto(BASE + '/settings/agent', 4500)
    await page.eval(`(() => { const b=[...document.querySelectorAll('button[role=switch]')].find(e => /change your node/i.test(e.parentElement.textContent)); if(!b) throw new Error('no writes switch'); b.click() })()`)
    await sleep(800)
    const names = await page.eval(`Object.keys(window.__tools||{}).join(',')`)
    log(`tools ${names.slice(0, 80)}`)
    await page.eval(`(() => { window.__r = window.__tools.ipfs_add.execute({content:'Hello from an agent, added through the WebMCP confirmation dialog.', filename:'greeting.txt', pin:true, provide:false}); return 1 })()`)
    await sleep(500)
    await page.shot(`${OUT}/webmcp-dialog-unarmed-${theme}-${w}.png`)
    await sleep(1500)
    await page.shot(`${OUT}/webmcp-dialog-${theme}-${w}.png`)
    const m = await page.eval(`(() => { const d=document.querySelector('[data-testid=webmcp-confirm]'); if(!d) return null; const dr=d.getBoundingClientRect(); const f=d.querySelector('.webmcp-footer').getBoundingClientRect(); const n=[...d.querySelectorAll('.webmcp-note')].map(e=>{const r=e.getBoundingClientRect(); return {t:e.textContent.slice(0,28), top:Math.round(r.top), bottom:Math.round(r.bottom)}}); return {scrollTop:d.scrollTop, scrollH:d.scrollHeight, clientH:d.clientHeight, dialogBottom:Math.round(dr.bottom), footerTop:Math.round(f.top), footerH:Math.round(f.height), vh:innerHeight, notes:n, docScrollW:document.documentElement.scrollWidth} })()`)
    log(`webmcp-metrics ${theme} ${w} ${JSON.stringify(m)}`)
    await page.key('Escape', 'Escape', 27)
  })
}

// 4. Peers: empty state after disconnect
if (want('peers-empty')) {
  const peers = await (await fetch('http://127.0.0.1:5199/api/v0/swarm/peers', { method: 'POST' })).json()
  for (const p of peers.Peers || []) {
    await fetch(`http://127.0.0.1:5199/api/v0/swarm/disconnect?arg=${encodeURIComponent(p.Addr + '/p2p/' + p.Peer)}`, { method: 'POST' })
  }
  log('disconnected peers: ' + (peers.Peers || []).length)
  await sleep(2000)
}
if (want('peers-empty')) for (const theme of THEMES) for (const [w, h] of [[1440, 900], [375, 812]]) {
  await withPage(theme, w, h, {}, async (page) => {
    await page.goto(BASE + '/peers', 4500)
    await shotFull(page, `peers-empty-${theme}-${w}.png`)
    log(`peers-empty ${theme} ${w}`)
  })
}

// 5. Welcome not connected
if (want('welcome-down')) for (const theme of THEMES) for (const [w, h] of [[1440, 900], [375, 812]]) {
  await withPage(theme, w, h, { api: '/ip4/127.0.0.1/tcp/5977' }, async (page) => {
    await page.goto(BASE + '/welcome', 6000)
    await shotFull(page, `welcome-notconnected-${theme}-${w}.png`)
    log(`welcome-down ${theme} ${w}`)
  })
}
await browser.close()
log('done')
