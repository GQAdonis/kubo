import { T, EV, start, prime, open, shot, text, sleep, D, M } from './common.mjs'
const { proc, page } = await start()
const dark = (v) => ({ ...v, theme: 'dark' })
const light = (v) => ({ ...v, theme: 'light' })
const section = async (name, fn) => { try { await fn(); console.log('done', name) } catch (e) { console.log('FAILED', name, e.message) } }
try {
  await section('Peers local network (M6)', async () => {
    await prime(page, { chat: true })
    await open(page, '#/peers', `document.body.innerText.includes('Local network')`)
    await sleep(800)
    for (const v of [dark(D), light(D), dark(M), light(M)]) await shot(page, 'f1-peers-local', v)
  })
  await section('Diagnostics DHT provide (M5)', async () => {
    await prime(page, { chat: true })
    await open(page, '#/diagnostics/provider', `document.querySelector('a[href="#/diagnostics/provider"]')`)
    await sleep(2500)
    for (const v of [dark(D), light(D), dark(M), light(M)]) await shot(page, 'f2-diagnostics-dht-provide', v)
  })
  await section('Diagnostics logs first view (L6)', async () => {
    await prime(page, { chat: true })
    await open(page, '#/diagnostics/logs', `document.querySelector('.log-toolbar')`)
    await sleep(1500)
    console.log('nudge button:', await page.eval(`(() => { const b = document.querySelector('.prom-log-btn-nudge'); if (!b) return 'none'; const cs = getComputedStyle(b); const accent = getComputedStyle(document.documentElement).getPropertyValue('--accent-text').trim(); const text = getComputedStyle(document.documentElement).getPropertyValue('--text').trim(); return JSON.stringify({ color: cs.color, accentText: accent, text, label: b.getAttribute('aria-label') }) })()`))
    for (const v of [dark(D), light(D), dark(M), light(M)]) await shot(page, 'f3-diagnostics-logs-first-view', v)
  })
  await section('Welcome not connected (L5)', async () => {
    await prime(page, { chat: true, api: '/ip4/127.0.0.1/tcp/5286' })
    await page.goto('http://localhost:3297/#/welcome')
    await page.send('Page.reload'); await sleep(6000)
    for (const v of [dark(D), light(D), dark(M), light(M)]) await shot(page, 'f4-welcome-not-connected', v)
    console.log('welcome text:', (await page.eval('document.body.innerText')).slice(0, 400).replace(/\n+/g, ' | '))
  })
} finally { proc.kill() }
