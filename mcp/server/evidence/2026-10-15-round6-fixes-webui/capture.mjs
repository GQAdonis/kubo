// Round 6 correction captures for the Web UI. Run from the ipfs-webui checkout:
//   node <this file> <out-dir>
// Needs the dev server on 127.0.0.1:3199 and test/e2e/stub-ipfs-mcp.js --port 3198 --rpc-shim.
// Uses the playwright-core already in node_modules and the system Chrome. No Kubo node, no
// ports 4001, 5001, 8080 or 5050: the shell's RPC address is pointed at the stub through
// localStorage before the app starts.
import { createRequire } from 'node:module'
import fs from 'node:fs'
import path from 'node:path'

const require = createRequire(path.join(process.cwd(), 'package.json'))
const { chromium } = require('playwright-core')

const out = process.argv[2]
fs.mkdirSync(out, { recursive: true })
const APP = 'http://127.0.0.1:3199'
const STUB = 'http://127.0.0.1:3198'
const TOKEN = 'stub-token-0000'
const report = []

const scenario = (s) => fetch(`${STUB}/__stub/scenario`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify(s) })

const browser = await chromium.launch({ executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', headless: true })

async function open (theme, width, height, { chat = true, search } = {}) {
  const context = await browser.newContext({ viewport: { width, height }, reducedMotion: 'reduce', colorScheme: theme })
  await context.addInitScript(({ theme, chat, search, endpoint, token }) => {
    const set = (k, v) => window.localStorage.setItem(k, typeof v === 'string' ? v : JSON.stringify(v))
    set('ipfsApi', '/ip4/127.0.0.1/tcp/3198')
    set('ipfs-webui:theme:v1', theme)
    set('ipfs-webui:agent-chat', { enabled: chat, endpoint })
    window.localStorage.setItem('ipfs-webui:agent-chat:token', token)
    if (search !== undefined) set('ipfs-webui:search', { enabled: search })
  }, { theme, chat, search, endpoint: `${STUB}/agui`, token: TOKEN })
  const page = await context.newPage()
  return { page, context }
}

async function metrics (page) {
  return page.evaluate(() => {
    const launcher = document.querySelector('.ac-launcher')
    const l = launcher?.querySelector('button')?.getBoundingClientRect()
    return {
      overflowX: document.documentElement.scrollWidth > document.documentElement.clientWidth,
      launcherTop: l ? Math.round(l.top) : null,
      clearance: getComputedStyle(document.documentElement).getPropertyValue('--ac-launcher-clearance').trim(),
      scrollPaddingBottom: getComputedStyle(document.documentElement).scrollPaddingBottom
    }
  })
}

const shot = async (page, name, extra = {}) => {
  await page.screenshot({ path: path.join(out, `${name}.png`), ...extra })
}

const ONLY = process.env.ONLY
if (!ONLY || ONLY === 'search') {
// 1. Search at three widths, both themes, keyboard focus on the last chip.
for (const theme of ['light', 'dark']) {
  for (const [width, height] of [[320, 640], [375, 700], [1440, 900]]) {
    await scenario({ status: 'idle', search: 'ok' })
    const { page, context } = await open(theme, width, height)
    await page.goto(`${APP}/#/search`)
    await page.getByRole('searchbox', { name: 'Search your files' }).waitFor({ timeout: 30000 })
    await page.waitForSelector('.ac-launcher button', { timeout: 30000 })
    // Tab from the search box through to the last filter chip, as a keyboard user would.
    await page.getByRole('searchbox').focus()
    let last = ''
    for (let i = 0; i < 40; i++) {
      await page.keyboard.press('Tab')
      const info = await page.evaluate(() => {
        const a = document.activeElement
        return { pressed: a?.getAttribute('aria-pressed') !== null, label: a?.textContent?.trim() ?? '', inSources: !!a?.closest('fieldset') }
      })
      if (info.pressed) last = info.label
      const next = await page.evaluate(() => {
        const chips = [...document.querySelectorAll('button[aria-pressed]')]
        return document.activeElement === chips[chips.length - 1]
      })
      if (next) break
    }
    await page.waitForTimeout(250)
    const focus = await page.evaluate(() => {
      const a = document.activeElement
      const r = a.getBoundingClientRect()
      const lb = document.querySelector('.ac-launcher button')?.getBoundingClientRect()
      return { text: a.textContent.trim(), bottom: Math.round(r.bottom), top: Math.round(r.top), launcherTop: lb ? Math.round(lb.top) : null, clearOfLauncher: lb ? (r.bottom <= lb.top || r.right <= lb.left) : true }
    })
    report.push({ shot: `search-focus-last-chip-${theme}-${width}`, ...focus, ...(await metrics(page)) })
    await shot(page, `search-focus-last-chip-${theme}-${width}`)
    await context.close()
  }
}

// 2. Status probe 403 and the unreachable gate.
for (const theme of ['light', 'dark']) {
  await scenario({ status: 'forbidden', search: 'ok' })
  let s = await open(theme, 375, 800)
  await s.page.goto(`${APP}/#/search`)
  await s.page.getByText(/does not have search access/).first().waitFor({ timeout: 30000 })
  await shot(s.page, `status-403-${theme}-375`, { fullPage: true })
  await s.context.close()
  s = await open(theme, 1440, 900)
  await s.page.goto(`${APP}/#/search`)
  await s.page.getByText(/does not have search access/).first().waitFor({ timeout: 30000 })
  await shot(s.page, `status-403-${theme}-1440`)
  report.push({ shot: `status-403-${theme}-1440`, panelText: await s.page.locator('.sr-status').innerText(), ...(await metrics(s.page)) })
  await s.context.close()
}

// 3. The Search switch in Features on Agent settings.
for (const theme of ['light', 'dark']) {
  for (const [width, height] of [[1440, 900], [375, 800]]) {
    await scenario({ status: 'idle', search: 'ok' })
    const { page, context } = await open(theme, width, height)
    await page.goto(`${APP}/#/settings/agent`)
    const toggle = page.getByRole('switch', { name: 'Search your files' })
    await toggle.waitFor({ timeout: 30000 })
    await page.locator('[data-id="AgentSettingsFeatures"]').scrollIntoViewIfNeeded()
    await page.waitForTimeout(250)
    await page.locator('[data-id="AgentSettingsFeatures"]').screenshot({ path: path.join(out, `settings-search-switch-${theme}-${width}.png`) })
    report.push({ shot: `settings-search-switch-${theme}-${width}`, checked: await toggle.getAttribute('aria-checked'), navEntry: await page.getByRole('menuitem', { name: 'Search' }).count() })
    if (width === 1440 && theme === 'light') {
      await toggle.click()
      await page.waitForTimeout(250)
      report.push({ shot: 'after switch off', checked: await toggle.getAttribute('aria-checked'), navEntry: await page.getByRole('menuitem', { name: 'Search' }).count() })
      await page.locator('[data-id="AgentSettingsFeatures"]').screenshot({ path: path.join(out, `settings-search-switch-off-${theme}-${width}.png`) })
    }
    await context.close()
  }
}

}
if (!ONLY || ONLY === 'logs') {
// 4. Diagnostics logs, launcher clearance.
for (const theme of ['light', 'dark']) {
  for (const [width, height] of [[375, 700], [1440, 900]]) {
    const { page, context } = await open(theme, width, height)
    // The stub reports no Kubo version and serves no logs: answer the four calls the logs
    // screen needs so it renders (a version that supports log levels, two subsystems, a short
    // tail that then ends, which is the "Streaming stopped" state).
    const cors = { 'access-control-allow-origin': APP, 'access-control-allow-credentials': 'true', 'access-control-allow-headers': '*', 'access-control-allow-methods': 'POST, GET, OPTIONS', vary: 'Origin' }
    const answer = (body, type = 'application/json') => async (route) => (route.request().method() === 'OPTIONS'
      ? route.fulfill({ status: 204, headers: cors })
      : route.fulfill({ status: 200, headers: { ...cors, 'content-type': type }, body }))
    await page.route('**/api/v0/id*', answer(JSON.stringify({ ID: '12D3KooWStubStubStubStubStubStubStubStubStubStubStub', AgentVersion: 'kubo/0.40.0/stub', ProtocolVersion: 'ipfs/0.1.0', Addresses: [], Protocols: [], PublicKey: '' })))
    await page.route('**/api/v0/version*', answer(JSON.stringify({ Version: '0.40.0', Commit: 'stub', Repo: '18', System: 'stub', Golang: 'stub' })))
    await page.route('**/api/v0/log/level*', answer(JSON.stringify({ levels: { '(default)': 'error', bitswap: 'warn', dht: 'info' } })))
    await page.route('**/api/v0/log/ls*', answer(JSON.stringify({ Strings: ['bitswap', 'dht'] })))
    const line = (i, lvl, sub, msg) => JSON.stringify({ ts: `2026-10-15T10:0${i}:00.000Z`, level: lvl, logger: sub, caller: 'x.go:1', msg })
    await page.route('**/api/v0/log/tail*', answer([line(1, 'info', 'dht', 'bootstrapping routing table'), line(2, 'warn', 'bitswap', 'peer sent a block we did not ask for'), line(3, 'error', 'dht', 'lookup failed: context deadline exceeded')].join('\n') + '\n', 'application/x-ndjson'))
    await page.goto(`${APP}/#/diagnostics/logs`)
    await page.waitForSelector('.logs-screen-container', { timeout: 30000 })
    await page.waitForSelector('.ac-launcher button', { timeout: 30000 })
    await page.waitForTimeout(800)
    const geo = await page.evaluate(() => {
      const lb = document.querySelector('.ac-launcher button').getBoundingClientRect()
      const footer = document.querySelector('.statsBar')?.getBoundingClientRect()
      const bar = document.querySelector('.log-toolbar')?.getBoundingClientRect()
      return { launcherTop: Math.round(lb.top), launcherLeft: Math.round(lb.left), footerBottom: footer && Math.round(footer.bottom), toolbarBottom: bar && Math.round(bar.bottom), toolbarTop: bar && Math.round(bar.top) }
    })
    report.push({ shot: `diagnostics-logs-${theme}-${width}`, ...geo, ...(await metrics(page)) })
    await shot(page, `diagnostics-logs-${theme}-${width}`)
    // Scrolled to the end: the toolbar and the footer must be reachable above the launcher.
    await page.evaluate(() => window.scrollTo(0, document.scrollingElement.scrollHeight))
    await page.waitForTimeout(300)
    const end = await page.evaluate(() => {
      const lb = document.querySelector('.ac-launcher button').getBoundingClientRect()
      const box = (sel) => { const r = document.querySelector(sel)?.getBoundingClientRect(); return r && { top: Math.round(r.top), bottom: Math.round(r.bottom) } }
      return { launcherTop: Math.round(lb.top), viewport: window.innerHeight, toolbar: box('.log-toolbar'), footer: box('.statsBar') }
    })
    end.footerClearOfLauncher = end.footer.bottom <= end.launcherTop
    end.toolbarVisible = end.toolbar.top >= 0 && end.toolbar.bottom <= end.viewport
    report.push({ shot: `diagnostics-logs-end-${theme}-${width}`, ...end })
    await shot(page, `diagnostics-logs-end-${theme}-${width}`)
    await context.close()
  }
}

}
await browser.close()
fs.writeFileSync(path.join(out, 'metrics.json'), JSON.stringify(report, null, 2))
console.log(JSON.stringify(report, null, 2))
