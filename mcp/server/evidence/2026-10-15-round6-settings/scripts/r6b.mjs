import { T, EV, start, prime, open, shot, call, rev, text, sleep, D, M, XS } from './common.mjs'
const { proc, page } = await start()
const dark = (v) => ({ ...v, theme: 'dark' })
const light = (v) => ({ ...v, theme: 'light' })
const section = async (name, fn) => { if (process.env.ONLY && !name.startsWith(process.env.ONLY)) return; try { await fn(); console.log('done', name) } catch (e) { console.log('FAILED', name, e.message) } }
// scroll so the Backend section's sticky Apply bar is stuck at the bottom of the viewport
const backend = (name, v, down = 0) => shot(page, name, v, { selector: '[data-id="AgentSettingsBackend"]', offset: 90 - down })
const bar = () => page.eval(`(() => { const b = document.querySelector('.pm-apply-bar'); const l = document.querySelector('.ac-launcher'); const r = (e) => e ? e.getBoundingClientRect() : null; const bb = r(b), ll = r(l); return JSON.stringify({ bar: bb && { top: Math.round(bb.top), bottom: Math.round(bb.bottom), left: Math.round(bb.left), right: Math.round(bb.right) }, launcher: ll && { top: Math.round(ll.top), bottom: Math.round(ll.bottom), left: Math.round(ll.left), right: Math.round(ll.right) }, vh: innerHeight, vw: innerWidth, overlap: bb && ll ? !(bb.bottom <= ll.top || bb.top >= ll.bottom || bb.right <= ll.left || bb.left >= ll.right) : null }) })()`)

try {
  await section('B H2 launcher clearance', async () => {
    await prime(page, { chat: true })
    await open(page)
    await page.until(`document.querySelector('[data-id="AgentSettingsOpenAi"]')`)
    await page.typeLabel('Model', 'qwen3:8b'); await page.key('Enter', 'Enter', 13)
    await sleep(500)
    const views = [dark(M), light(M), dark(XS), light(XS), { width: 1024, height: 768, theme: 'dark' }, { width: 1279, height: 800, theme: 'dark' }, { width: 1280, height: 800, theme: 'dark' }, { width: 1366, height: 768, theme: 'dark' }, { width: 1366, height: 768, theme: 'light' }, { width: 1405, height: 800, theme: 'dark' }, { width: 1440, height: 900, theme: 'dark' }, { width: 1440, height: 900, theme: 'light' }]
    for (const v of views) {
      await page.view(v)
      await backend('b-h2', v, v.width < 500 ? 300 : 120)
      console.log(v.width, v.theme, await bar())
    }
    // Is the page-bottom bar clear of the launcher when scrolled to the very end of the section?
    await page.view(dark(XS)); await page.eval(`document.querySelector('[data-id="AgentSettingsBackend"]').scrollIntoView({block:'end'})`); await sleep(300); await shot(page, 'b-h2-section-end', dark(XS)); console.log('section end', await bar())
  })

  await section('D H1/C2 conflict, retry, reload', async () => {
    await prime(page, { chat: true })
    await open(page)
    await page.until(`document.querySelector('[data-id="AgentSettingsOpenAi"]')`)
    await page.typeLabel('Model', 'qwen3:8b'); await page.key('Enter', 'Enter', 13)
    console.log('node moves on:', call('PUT', '/agent/settings', { backend: { model: 'phi4' } }, rev()).split('\r\n')[0])
    await page.clickText('button', 'Apply')
    await page.until(text('reloaded and your edits are kept'), 20000)
    await sleep(500)
    for (const v of [dark(D), light(M)]) await backend('d1-conflict', v, v.width < 500 ? 300 : 120)
    console.log('conflict: Retry present', await page.eval(`[...document.querySelectorAll('button')].some(b => b.textContent.trim() === 'Retry')`), 'Reload settings present', await page.eval(`[...document.querySelectorAll('button')].some(b => b.textContent.trim() === 'Reload settings')`), 'model', await page.eval(`document.querySelector('[role=combobox]').value`))
    await page.clickText('button', 'Retry')
    await page.until(text('Applied'), 20000)
    await sleep(500)
    for (const v of [dark(D), light(M)]) await backend('d2-retry-applied', v, v.width < 500 ? 300 : 120)
    console.log('applied; node model now:', /"model":"([^"]+)"/.exec(call('GET', '/agent/settings'))[1])
    // second conflict, then Reload settings
    await page.typeLabel('Model', 'llama3.1:8b'); await page.key('Enter', 'Enter', 13)
    console.log('node moves on:', call('PUT', '/agent/settings', { backend: { model: 'mistral-small' } }, rev()).split('\r\n')[0])
    await page.clickText('button', 'Apply')
    await page.until(text('reloaded and your edits are kept'), 20000)
    await page.clickText('button', 'Reload settings')
    await sleep(1500)
    console.log('after reload: No changes', await page.eval(text('No changes')), '| conflict text gone', await page.eval(`!document.body.innerText.includes('your edits are kept')`), '| Retry gone', await page.eval(`![...document.querySelectorAll('button')].some(b => b.textContent.trim() === 'Retry')`), '| Apply present+disabled', await page.eval(`(() => { const b = [...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Apply'); return b ? 'present, disabled=' + b.disabled : 'absent' })()`), '| model', await page.eval(`document.querySelector('[role=combobox]').value`))
    for (const v of [dark(D), light(M)]) await backend('d3-after-reload', v, v.width < 500 ? 300 : 120)
  })
} finally { proc.kill() }
