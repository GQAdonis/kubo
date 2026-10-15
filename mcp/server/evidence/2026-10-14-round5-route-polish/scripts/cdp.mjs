// Minimal Chrome DevTools Protocol driver (Node global WebSocket, no dependencies).
import { spawn } from 'node:child_process'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

export const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))

export async function launchChrome (port = 9399) {
  const dir = mkdtempSync(join(tmpdir(), 'webui-evidence-'))
  const child = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
    '--headless=new', `--remote-debugging-port=${port}`, `--user-data-dir=${dir}`,
    '--no-first-run', '--no-default-browser-check', '--force-device-scale-factor=1',
    '--disable-extensions', '--disable-sync', 'about:blank'
  ], { stdio: 'ignore' })
  let version
  for (let i = 0; i < 60; i++) {
    try {
      version = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json()
      break
    } catch (_) { await sleep(250) }
  }
  if (!version) throw new Error('chrome did not start')
  const ws = new WebSocket(version.webSocketDebuggerUrl)
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject })
  let id = 0
  const pending = new Map()
  const listeners = []
  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data)
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id)
      pending.delete(msg.id)
      if (msg.error) reject(new Error(`${msg.error.message}`))
      else resolve(msg.result)
    } else if (msg.method) {
      listeners.forEach((fn) => fn(msg))
    }
  }
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const mid = ++id
    pending.set(mid, { resolve, reject })
    ws.send(JSON.stringify({ id: mid, method, params, sessionId }))
  })
  return {
    pid: child.pid,
    send,
    listeners,
    async close () {
      try { await send('Browser.close') } catch (_) {}
      ws.close()
    }
  }
}

/** A fresh incognito-like context with one page, bound to a session. */
export async function newPage (browser, { width, height, initScript }) {
  const { browserContextId } = await browser.send('Target.createBrowserContext')
  const { targetId } = await browser.send('Target.createTarget', { url: 'about:blank', browserContextId })
  const { sessionId } = await browser.send('Target.attachToTarget', { targetId, flatten: true })
  const s = (method, params) => browser.send(method, params, sessionId)
  await s('Page.enable')
  await s('Runtime.enable')
  await s('Emulation.setDeviceMetricsOverride', { width, height, deviceScaleFactor: 1, mobile: false })
  if (initScript) await s('Page.addScriptToEvaluateOnNewDocument', { source: initScript })
  const page = {
    s,
    async goto (url, settle = 3500) {
      await s('Page.navigate', { url })
      await sleep(settle)
    },
    async eval (expression) {
      const r = await s('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      if (r.exceptionDetails) throw new Error(r.exceptionDetails.exception?.description || r.exceptionDetails.text)
      return r.result.value
    },
    async resize (w, h) {
      await s('Emulation.setDeviceMetricsOverride', { width: w, height: h, deviceScaleFactor: 1, mobile: false })
      await sleep(400)
    },
    async shot (file, { full = false, clip } = {}) {
      const params = { format: 'png', captureBeyondViewport: full }
      if (clip) params.clip = { ...clip, scale: 1 }
      if (full) {
        const m = await s('Page.getLayoutMetrics')
        const size = m.cssContentSize || m.contentSize
        if (!clip) params.clip = { x: 0, y: 0, width: size.width, height: Math.min(size.height, 4000), scale: 1 }
      }
      const { data } = await s('Page.captureScreenshot', params)
      writeFileSync(file, Buffer.from(data, 'base64'))
    },
    async key (key, code, vk, modifiers = 0) {
      await s('Input.dispatchKeyEvent', { type: 'rawKeyDown', key, code, windowsVirtualKeyCode: vk, modifiers })
      await s('Input.dispatchKeyEvent', { type: 'keyUp', key, code, windowsVirtualKeyCode: vk, modifiers })
    },
    async tab () { await page.key('Tab', 'Tab', 9); await sleep(120) },
    async click (selector) {
      const box = await page.eval(`(() => { const e = document.querySelector(${JSON.stringify(selector)}); if (!e) return null; e.scrollIntoView({block:'center'}); const r = e.getBoundingClientRect(); return {x:r.x+r.width/2, y:r.y+r.height/2} })()`)
      if (!box) throw new Error(`no element ${selector}`)
      await s('Input.dispatchMouseEvent', { type: 'mouseMoved', x: box.x, y: box.y })
      await s('Input.dispatchMouseEvent', { type: 'mousePressed', x: box.x, y: box.y, button: 'left', clickCount: 1 })
      await s('Input.dispatchMouseEvent', { type: 'mouseReleased', x: box.x, y: box.y, button: 'left', clickCount: 1 })
      await sleep(400)
    },
    async close () { await browser.send('Target.closeTarget', { targetId }); await browser.send('Target.disposeBrowserContext', { browserContextId }) }
  }
  return page
}
