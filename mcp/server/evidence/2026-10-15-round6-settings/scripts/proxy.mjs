// One origin for the evidence run: /agent/* and /agui go to ipfs mcp, everything else to the dev server.
import http from 'node:http'
const MCP = { host: '127.0.0.1', port: 5287 }
const DEV = { host: '127.0.0.1', port: 3299 }
http.createServer((req, res) => {
  const target = req.url.startsWith('/agent/') || req.url.startsWith('/agui') ? MCP : DEV
  const up = http.request({ ...target, method: req.method, path: req.url, headers: req.headers }, (r) => {
    res.writeHead(r.statusCode, r.headers)
    r.pipe(res)
  })
  up.on('error', () => { res.writeHead(502); res.end('bad gateway') })
  req.pipe(up)
}).listen(3297, '127.0.0.1')
