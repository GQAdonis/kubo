import { launch } from './cdp.mjs'
import { execFileSync } from 'node:child_process'
export const T = process.env.T
export const EV = process.env.EV
export const ADMIN = 'adm-evidence-fake-0001'
export const UI = 'ui-evidence-fake-0001'
export const ORIGIN = 'http://localhost:3297'
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
export const SESSION_KEY = 'ipfs-webui:agent-settings:admin-token:v2'
export async function start () {
  const { proc, page } = await launch({ profile: `${T}/chrome` })
  return { proc, page }
}
// Prime storage on the app origin, then reload so the app boots from it (no blank page exists).
export async function prime (page, { chat, enabled, endpoint = `${ORIGIN}/agui`, token = true, adminToken = ADMIN, api = '/ip4/127.0.0.1/tcp/5299', webmcp, extra = '' } = {}) {
  await page.goto(`${ORIGIN}/#/`)
  const chatStored = chat === 'absent' ? 'null' : chat === 'default' ? JSON.stringify({ endpoint }) : JSON.stringify({ enabled: chat === true, endpoint })
  const webmcpStored = webmcp === undefined ? 'null' : JSON.stringify({ enabled: webmcp })
  await page.eval(`(() => {
    localStorage.clear(); sessionStorage.clear();
    localStorage.setItem('ipfsApi', '${api}');
    localStorage.setItem('ipfs-webui:theme:v1', 'system');
    ${chatStored === 'null' ? '' : `localStorage.setItem('ipfs-webui:agent-chat', JSON.stringify(${chatStored}));`}
    ${webmcpStored === 'null' ? '' : `localStorage.setItem('ipfs-webui:webmcp', JSON.stringify(${webmcpStored}));`}
    ${(chat === true || chat === 'default') && token ? `localStorage.setItem('ipfs-webui:agent-chat:token', '${UI}');` : ''}
    ${adminToken ? `sessionStorage.setItem('${SESSION_KEY}', JSON.stringify({ token: '${adminToken}', root: '${ORIGIN}' }));` : ''}
    ${extra}
  })()`)
}
export async function open (page, hash = '#/settings/agent', ready = `document.querySelector('[data-id="AgentSettingsPage"], [data-id="SettingsPage"]')`) {
  await page.goto(`${ORIGIN}/${hash}`)
  await page.send('Page.reload')
  await sleep(2500)
  await page.until(ready, 30000)
  await page.until(`document.body.innerText.includes('Connected')`, 20000).catch(() => console.log('note: shell not Connected'))
  await sleep(500)
}
export const text = (t) => `document.body.innerText.includes(${JSON.stringify(t)})`
export const call = (method, path, body, ifMatch) => {
  const args = ['-s', '-i', '-X', method, '-H', `Authorization: Bearer ${ADMIN}`, `http://127.0.0.1:5287${path}`]
  if (body) args.push('-H', 'Content-Type: application/json', '-d', JSON.stringify(body))
  if (ifMatch) args.push('-H', `If-Match: "${ifMatch}"`)
  return execFileSync('curl', args).toString()
}
export const rev = () => /"revision":"([^"]+)"/.exec(call('GET', '/agent/settings'))[1]
export async function shot (page, name, v, { selector, offset = 70, full = false } = {}) {
  await page.view(v)
  if (selector) await page.eval(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (el) { const y = el.getBoundingClientRect().top + window.scrollY - ${offset}; window.scrollTo(0, Math.max(0, y)) } })()`)
  await sleep(400)
  await page.shot(`${EV}/${name}-${v.width}-${v.theme}.png`, { full })
  console.log('shot', `${name}-${v.width}-${v.theme}`)
}
export const D = { width: 1440, height: 900 }
export const M = { width: 375, height: 812 }
export const XS = { width: 320, height: 568 }
