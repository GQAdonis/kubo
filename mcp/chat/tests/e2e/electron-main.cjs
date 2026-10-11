// A minimal Electron main process that loads the harness page the way IPFS Desktop loads the Web UI:
// from the privileged custom scheme webui://localhost, with webSecurity on, contextIsolation on, the
// sandbox on, and no preload. It serves the scheme by forwarding to the harness HTTP server, so the page
// and the island run on an origin that is not http (ADR-0005: "credentialed fetch from the webui://
// scheme to a loopback ipfs mcp origin" is one of the things this proves in Electron 44).
//
// Environment: HARNESS_ORIGIN (http://127.0.0.1:<port>), HARNESS_PATH (path and query to load),
// WINDOW_WIDTH, WINDOW_HEIGHT.
const { app, BrowserWindow, protocol, net } = require('electron');

protocol.registerSchemesAsPrivileged([
  { scheme: 'webui', privileges: { standard: true, secure: true, supportFetchAPI: true, corsEnabled: true, stream: true } },
]);

app.whenReady().then(async () => {
  const origin = process.env.HARNESS_ORIGIN;
  const path = process.env.HARNESS_PATH || '/host.html';
  if (!origin) throw new Error('HARNESS_ORIGIN is required');

  protocol.handle('webui', (request) => {
    const url = new URL(request.url);
    // Forward only what the harness server needs. The page's Origin and Referer name webui://localhost, and
    // net.fetch fails a module-script request that carries them (net::ERR_FAILED on /host.js).
    const headers = new Headers();
    const accept = request.headers.get('accept');
    if (accept) headers.set('accept', accept);
    return net.fetch(origin + url.pathname + url.search, { method: request.method, headers });
  });

  const width = Number(process.env.WINDOW_WIDTH || 1024);
  const height = Number(process.env.WINDOW_HEIGHT || 768);
  const win = new BrowserWindow({
    width,
    height,
    useContentSize: true,
    show: process.env.SHOW_WINDOW === '1',
    webPreferences: { contextIsolation: true, sandbox: true, nodeIntegration: false, webSecurity: true },
  });
  await win.loadURL('webui://localhost' + path);
});

app.on('window-all-closed', () => app.quit());
