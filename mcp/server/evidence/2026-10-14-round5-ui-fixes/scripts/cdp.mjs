// A small Chrome DevTools Protocol driver for the evidence run. Raw CDP over WebSocket; no Playwright.
import { spawn } from 'node:child_process'
import { writeFileSync } from 'node:fs'

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

export async function launch ({ port = 9333, profile }) {
  const proc = spawn(CHROME, [`--remote-debugging-port=${port}`, `--user-data-dir=${profile}`, '--headless=new', '--no-first-run', '--no-default-browser-check', '--hide-scrollbars', 'about:blank'], { stdio: 'ignore' })
  for (let i = 0; i < 50; i++) {
    try {
      const targets = await (await fetch(`http://127.0.0.1:${port}/json`)).json()
      const page = targets.find((t) => t.type === 'page')
      if (page) return { proc, page: await connect(page.webSocketDebuggerUrl) }
    } catch (_) { /* not up yet */ }
    await sleep(200)
  }
  throw new Error('chrome did not start')
}

async function connect (url) {
  const ws = new WebSocket(url)
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject })
  let id = 0
  const pending = new Map()
  const events = []
  ws.onmessage = (m) => {
    const msg = JSON.parse(m.data)
    if (msg.method) events.push(msg)
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id)
      pending.delete(msg.id)
      msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result)
    }
  }
  const send = (method, params = {}) => new Promise((resolve, reject) => {
    const n = ++id
    pending.set(n, { resolve, reject })
    ws.send(JSON.stringify({ id: n, method, params }))
  })
  await send('Page.enable')
  await send('Runtime.enable')
  await send('Network.enable')

  const api = {
    events,
    send,
    async goto (url) { await send('Page.navigate', { url }); await sleep(1500) },
    async eval (expression) {
      const r = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text)
      return r.result.value
    },
    async until (expression, timeout = 15000) {
      const end = Date.now() + timeout
      while (Date.now() < end) {
        try { if (await api.eval(`!!(${expression})`)) return true } catch (_) { /* page not ready */ }
        await sleep(150)
      }
      throw new Error(`timeout waiting for: ${expression}`)
    },
    async view ({ width, height, theme }) {
      await send('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: width < 500 })
      await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: theme }] })
      await sleep(300)
    },
    async click (selector) { await api.eval(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) throw new Error('no ${selector.replace(/'/g, '')}'); el.scrollIntoView({block:'center'}); el.click(); return true })()`); await sleep(250) },
    async clickText (role, text) {
      await api.eval(`(() => { const els=[...document.querySelectorAll(${JSON.stringify(role)})]; const el = els.find(e => e.textContent.trim() === ${JSON.stringify(text)} || e.getAttribute('aria-label') === ${JSON.stringify(text)}); if (!el) throw new Error('no ${text}'); el.scrollIntoView({block:'center'}); el.click(); return true })()`)
      await sleep(250)
    },
    async type (selector, text, { clear = true } = {}) {
      await api.eval(`(() => { const el = document.querySelector(${JSON.stringify(selector)}); if (!el) throw new Error('no ${selector.replace(/'/g, '')}'); el.scrollIntoView({block:'center'}); el.focus(); ${clear ? 'el.select();' : ''} return true })()`)
      if (clear) await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Backspace', code: 'Backspace', windowsVirtualKeyCode: 8 }), await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Backspace', code: 'Backspace', windowsVirtualKeyCode: 8 })
      await send('Input.insertText', { text })
      await sleep(200)
    },
    async typeLabel (label, text) {
      await api.eval(`(() => { const l=[...document.querySelectorAll('label')].find(x=>x.textContent.trim().startsWith(${JSON.stringify(label)})); if(!l) throw new Error('no label ${label}'); const el=document.getElementById(l.htmlFor); el.scrollIntoView({block:'center'}); el.focus(); el.select && el.select(); return true })()`)
      await send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Backspace', code: 'Backspace', windowsVirtualKeyCode: 8 }); await send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Backspace', code: 'Backspace', windowsVirtualKeyCode: 8 })
      await send('Input.insertText', { text })
      await sleep(200)
    },
    async reload () { await send('Page.reload'); await sleep(2500) },
    async key (key, code, vk) { await send('Input.dispatchKeyEvent', { type: 'keyDown', key, code, windowsVirtualKeyCode: vk }); await send('Input.dispatchKeyEvent', { type: 'keyUp', key, code, windowsVirtualKeyCode: vk }); await sleep(200) },
    async shot (path, { full = false } = {}) {
      const params = { format: 'png' }
      if (full) {
        const m = await send('Page.getLayoutMetrics')
        params.clip = { x: 0, y: 0, width: m.cssContentSize.width, height: Math.min(m.cssContentSize.height, 4000), scale: 1 }
        params.captureBeyondViewport = true
      }
      const { data } = await send('Page.captureScreenshot', params)
      writeFileSync(path, Buffer.from(data, 'base64'))
    },
    close () { ws.close() }
  }
  return api
}
