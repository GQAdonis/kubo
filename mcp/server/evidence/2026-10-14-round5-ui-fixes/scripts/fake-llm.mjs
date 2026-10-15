// A fake OpenAI-compatible model server for the evidence run: lists models, takes any bearer.
import http from 'node:http'
http.createServer((req, res) => {
  if (req.url.startsWith('/v1/models')) {
    res.writeHead(200, { 'content-type': 'application/json' })
    res.end(JSON.stringify({ object: 'list', data: [{ id: 'qwen3:8b' }, { id: 'llama3.1:8b' }, { id: 'mistral-small' }, { id: 'phi4' }] }))
  } else { res.writeHead(404); res.end('no') }
}).listen(5286, '127.0.0.1')
