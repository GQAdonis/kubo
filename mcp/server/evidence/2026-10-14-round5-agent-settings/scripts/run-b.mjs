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
  await prime(); await open()
  await page.until(`document.querySelector('input[type=radio][value=openai]')`)
  await page.click('input[type=radio][value=openai]')
  await page.typeLabel('Base URL', 'http://127.0.0.1:5286/v1')
  await page.typeLabel('API key', 'sk-evidence-fake-0001')
  await page.clickText('button', 'Save key')
  await page.until(text('Set on the server'), 15000)
  await capture('12-secrets-not-accepted', '[data-id="AgentSettingsBackend"]')
} catch (e) {
  console.error('FAILED', e.message)
  await page.shot(`${EV}/FAILED-run-b.png`)
  process.exitCode = 1
} finally { proc.kill() }
