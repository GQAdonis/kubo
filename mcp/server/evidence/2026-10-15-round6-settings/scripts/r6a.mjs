import { T, EV, start, prime, open, shot, call, rev, text, sleep, D, M, XS } from './common.mjs'
const { proc, page } = await start()
const dark = (v) => ({ ...v, theme: 'dark' })
const light = (v) => ({ ...v, theme: 'light' })
const both = (v) => [dark(v), light(v)]
const section = async (name, fn) => { try { await fn(); console.log('done', name) } catch (e) { console.log('FAILED', name, e.message) } }
const bottom = () => page.eval('window.scrollTo(0, document.documentElement.scrollHeight)')

try {
  await page.view(dark(D))

  await section('A default-on', async () => {
    await prime(page, { chat: 'default' })
    await open(page)
    const stored = await page.eval(`JSON.stringify({chat: localStorage.getItem('ipfs-webui:agent-chat'), mcp: localStorage.getItem('ipfs-webui:webmcp')})`)
    console.log('stored after boot', stored)
    await page.until(`document.querySelector('.ac-launcher-button')`, 10000)
    for (const v of [dark(D), light(D), dark(M)]) await shot(page, 'a1-default-on-features', v, { selector: '[data-id="AgentChatSettings"]', offset: 120 })
  })

  await section('B H2 launcher clearance', async () => {
    await prime(page, { chat: true })
    await open(page)
    await page.until(`document.querySelector('input[type=radio][value=openai]')`)
    await page.click('input[type=radio][value=openai]')
    await page.typeLabel('Base URL', 'http://127.0.0.1:5286/v1')
    await sleep(500)
    for (const v of [...both(M), ...both(XS)]) { await page.view(v); await bottom(); await sleep(300); await shot(page, 'b-h2-bottom', v) }
    for (const v of [{ width: 1024, height: 768, theme: 'dark' }, { width: 1279, height: 800, theme: 'dark' }, { width: 1280, height: 800, theme: 'dark' }, { width: 1440, height: 900, theme: 'light' }, { width: 1440, height: 900, theme: 'dark' }]) {
      await page.view(v); await bottom(); await sleep(300); await shot(page, 'b-h2-bottom', v)
      console.log(v.width, await page.eval(`JSON.stringify({clearance: getComputedStyle(document.documentElement).getPropertyValue('--ac-launcher-clearance')})`))
    }
    // scroll position partway so the sticky bar overlays the form
    for (const v of [dark(XS)]) { await page.view(v); await page.eval('window.scrollTo(0, 600)'); await sleep(300); await shot(page, 'b-h2-mid', v) }
  })

  await section('C M2 no token', async () => {
    await prime(page, { chat: true, adminToken: false })
    await open(page)
    for (const v of [dark(D), light(M)]) await shot(page, 'c-m2-no-token', v)
  })

  await section('D H1/C2 conflict, retry, reload', async () => {
    await prime(page, { chat: true })
    await open(page)
    await page.until(`document.querySelector('input[type=radio][value=openai]')`)
    await page.click('input[type=radio][value=openai]')
    await page.typeLabel('Base URL', 'http://127.0.0.1:5286/v1')
    await page.clickText('button', 'Test connection')
    await page.until(text('Reachable'), 20000)
    await page.typeLabel('Model', 'qwen3:8b'); await page.key('Enter', 'Enter', 13)
    await page.typeLabel('Type the model host', '127.0.0.1:5286')
    // the node moves on under the form: another client sets a different model
    console.log(call('PUT', '/agent/settings', { backend: { kind: 'openai', base_url: 'http://127.0.0.1:5286/v1', model: 'phi4' }, egress_confirmation: { host: '127.0.0.1:5286' } }, rev()).split('\r\n')[0])
    await page.clickText('button', 'Apply')
    await page.until(text('reloaded and your edits are kept'), 20000)
    for (const v of [dark(D), light(M)]) await shot(page, 'd1-conflict', v, { selector: '[data-id="AgentSettingsBackend"] .as-fields', offset: 60 })
    await page.view(dark(D)); await bottom(); await sleep(300); await shot(page, 'd1-conflict-bar', dark(D))
    await page.clickText('button', 'Retry')
    await page.until(text('Applied'), 20000)
    for (const v of [dark(D), light(M)]) await shot(page, 'd2-retry-applied', v)
    await page.view(dark(D)); await bottom(); await sleep(300); await shot(page, 'd2-retry-applied-bar', dark(D))
    console.log('revision now', rev())
    // second conflict, then Reload settings
    await page.typeLabel('Model', 'llama3.1:8b'); await page.key('Enter', 'Enter', 13)
    console.log(call('PUT', '/agent/settings', { backend: { model: 'mistral-small' } }, rev()).split('\r\n')[0])
    await page.clickText('button', 'Apply')
    await page.until(text('reloaded and your edits are kept'), 20000)
    await page.clickText('button', 'Reload settings')
    await sleep(1500)
    console.log('after reload: No changes?', await page.eval(text('No changes')), 'Retry gone?', await page.eval(`![...document.querySelectorAll('button')].some(b => b.textContent.trim() === 'Retry')`), 'Apply present?', await page.eval(`[...document.querySelectorAll('button')].some(b => b.textContent.trim() === 'Apply')`))
    for (const v of [dark(D), light(M)]) { await page.view(v); await bottom(); await sleep(300); await shot(page, 'd3-after-reload', v) }
  })
} finally { proc.kill() }
