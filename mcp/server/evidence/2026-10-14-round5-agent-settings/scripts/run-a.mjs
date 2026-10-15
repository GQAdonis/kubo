import { launch } from './cdp.mjs'
import { execFileSync } from 'node:child_process'
const EV = process.env.EV
const ADMIN = 'adm-evidence-fake-0001'
const UI = 'ui-evidence-fake-0001'
const ORIGIN = 'http://localhost:3297'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const { proc, page } = await launch({ profile: process.env.PROFILE })
const VIEWS = [
  { width: 1440, height: 900, theme: 'dark' }, { width: 1440, height: 900, theme: 'light' },
  { width: 375, height: 812, theme: 'dark' }, { width: 375, height: 812, theme: 'light' }
]
async function prime ({ chat = false, adminToken = ADMIN } = {}) {
  await page.goto(`${ORIGIN}/blank-prime.html`)
  await page.eval(`(() => {
    localStorage.clear(); sessionStorage.clear();
    localStorage.setItem('ipfsApi', '/ip4/127.0.0.1/tcp/5299');
    localStorage.setItem('ipfs-webui:theme:v1', 'system');
    localStorage.setItem('ipfs-webui:agent-chat', JSON.stringify({ enabled: ${chat}, endpoint: '${ORIGIN}/agui' }));
    ${chat ? `localStorage.setItem('ipfs-webui:agent-chat:token', '${UI}');` : ''}
    ${adminToken ? `sessionStorage.setItem('ipfs-webui:agent-settings:admin-token:v1', '${adminToken}');` : ''}
  })()`)
}
async function open (hash = '#/settings/agent') {
  await page.goto(`${ORIGIN}/${hash}`)
  await page.until(`document.querySelector('[data-id="AgentSettingsPage"], [data-id="SettingsPage"]')`, 30000)
  await page.until(`document.body.innerText.includes('Connected')`, 20000).catch(() => console.log('note: shell not Connected'))
  await sleep(500)
}
async function capture (name, selector) {
  for (const v of VIEWS) {
    await page.view(v)
    if (selector) await page.eval(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (el) { const y = el.getBoundingClientRect().top + window.scrollY - 70; window.scrollTo(0, Math.max(0, y)) } })()`)
    else await page.eval('window.scrollTo(0,0)')
    await sleep(350)
    await page.shot(`${EV}/${name}-${v.width}-${v.theme}.png`)
  }
  console.log('captured', name)
}
const text = (t) => `document.body.innerText.includes(${JSON.stringify(t)})`
const get = (path) => execFileSync('curl', ['-s', '-i', '-H', `Authorization: Bearer ${ADMIN}`, `http://127.0.0.1:5287${path}`]).toString()
const rev = () => /"revision":"([^"]+)"/.exec(get('/agent/settings'))[1]

try {
  await page.view(VIEWS[0])
  // 1. first run, backend not chosen
  await prime(); await open()
  await page.until(`document.querySelector('input[type=radio]')`)
  await capture('01-first-run', '.as-header')

  // 2. chat island on a node with no backend: 503 backend_not_configured
  await prime({ chat: true }); await open('#/settings/agent')
  await page.until(`document.querySelector('.ac-launcher-button')`)
  await page.click('.ac-launcher-button')
  await sleep(1500)
  await page.send('Input.insertText', { text: 'Which pins do I have?' })
  await page.key('Enter', 'Enter', 13)
  await page.until(text('Pick a model to start'), 20000)
  await capture('02-chat-no-backend')
  // the launcher closed state for reference
  await page.click('.ac-launcher-button')

  // 3. OpenAI-compatible form, tested connection and typed host confirmation
  await prime(); await open()
  await page.until(`document.querySelector('input[type=radio][value=openai]')`)
  await page.click('input[type=radio][value=openai]')
  await page.typeLabel('Base URL', 'http://127.0.0.1:5286/v1')
  await page.clickText('button', 'Test connection')
  await page.until(text('Reachable'), 20000)
  await page.typeLabel('Model', 'qwen3')
  await sleep(400)
  await capture('03-openai-tested', '[data-id="AgentSettingsBackend"]')
  await page.key('ArrowDown', 'ArrowDown', 40); await page.key('Enter', 'Enter', 13)
  await page.typeLabel('Confirm the model host', '127.0.0.1:5286')
  await capture('04-openai-confirm', '[data-id="AgentSettingsConfirm"]')

  // 4. key saved, then stored state
  await page.typeLabel('API key', 'sk-evidence-fake-0001')
  await page.clickText('button', 'Save key')
  await page.until(text('Stored'), 15000)
  await capture('05-key-stored', '[data-id="AgentSettingsKey"]')

  // 5. apply success
  await page.clickText('button', 'Apply')
  await page.until(text('Applied'), 15000)
  await sleep(500)
  await capture('06-apply-success', '[data-id="AgentSettingsBackend"] .pm-apply-bar')
  await page.eval('window.scrollTo(0,0)')
  await capture('07-configured-header')

  // 6. apply error: another client changes the node while this form has edits
  await page.typeLabel('Model', 'phi4'); await page.key('Enter', 'Enter', 13)
  const r = rev()
  execFileSync('curl', ['-s', '-X', 'PUT', '-H', `Authorization: Bearer ${ADMIN}`, '-H', `If-Match: "${r}"`, '-H', 'Content-Type: application/json', '-d', '{"backend":{"model":"mistral-small"}}', 'http://127.0.0.1:5287/agent/settings'])
  await sleep(800)
  await page.clickText('button', 'Apply')
  await page.until(`document.querySelector('[data-testid=apply-failure]')`, 15000)
  await capture('08-apply-error', '[data-id="AgentSettingsBackend"] .pm-apply-bar')

  // 7. UAR unavailable
  await page.click('input[type=radio][value=uar]')
  await capture('09-uar-unavailable', '[data-id="AgentSettingsBackend"]')

  // 8. 401: a refused token
  await prime({ adminToken: 'wrong-token-fake' }); await open()
  await page.until(text('Sign in again'), 20000)
  await capture('10-sign-in-again')

  // 9. summary row on /settings
  await prime(); await open('#/settings')
  await page.until(`document.querySelector('[data-id="AgentSettingsSummary"]')`, 30000)
  await capture('11-settings-summary', '[data-id="AgentSettingsSummary"]')
} catch (e) {
  console.error('FAILED', e.message, await page.eval(`location.href + ' :: ' + document.body.innerText.slice(0,160).replace(/\\n/g,' | ')`).catch(() => ''))
  await page.shot(`${EV}/FAILED-run-a.png`)
  process.exitCode = 1
} finally { proc.kill() }
