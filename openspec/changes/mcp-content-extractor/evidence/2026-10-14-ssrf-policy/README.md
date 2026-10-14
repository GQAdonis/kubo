# xberg 1.3.10 SSRF policy check (2026-10-14)

Purpose: security finding C1 item 3. Does the raw xberg REST port refuse server-side fetches of private, link-local and loopback targets?

Method: the real xberg 1.3.10 binary (aarch64-apple-darwin) started by hand on a free ephemeral loopback port (53845) with a fresh `XBERG_CACHE_DIR`, `HF_HOME`, `HF_HUB_OFFLINE=1`, `DO_NOT_TRACK=1`. Requests were sent with curl and short timeouts (`-m 15`). No listener was started on any LAN address and nothing was scanned: every target below was refused before a connection was attempted.

```bash
xberg serve --host 127.0.0.1 --port 53845        # PID 64505, stopped with kill 64505
curl -s -m 15 -X POST http://127.0.0.1:53845/extract -F 'urls=["<URL>"]'
```

`urls` is a multipart field holding a JSON array of strings. A bare string gives HTTP 400 "Invalid urls field JSON".

## Results

Every request returned HTTP 200 with an `errors[0]` entry, `code` 1003, `error_type` "validation", and the message `crawlberg URL extraction failed: ssrf_policy_violation: <url> - denied by SSRF policy: <class>`. Response time was 0.001 s (refused by policy before any connection; a connection attempt to an unrouted address would have run into the client timeout).

| URL | Policy class | Time |
|-----|--------------|------|
| `http://10.0.0.1/` | `private_network` | 0.001 s |
| `http://192.168.1.1/` | `private_network` | 0.001 s |
| `http://169.254.169.254/latest/meta-data/` | `link_local` | 0.001 s |
| `http://[fe80::1]/` | `link_local` | 0.001 s |
| `http://127.0.0.1:53845/health` | `loopback` | 0.001 s |
| `http://172.16.0.1/` (extra) | `private_network` | 0.001 s |
| `http://[fd00::1]/` (extra) | `unique_local` | 0.002 s |
| `http://localhost/` (extra) | `loopback` | 0.023 s |
| `http://0.0.0.0/` (extra) | `unspecified` | 0.005 s |

Conclusion: for literal-IP and `localhost` targets, xberg 1.3.10 refuses by policy. Not tested: a public hostname that resolves to a private address (DNS rebinding of the fetch target), and fetches of public URLs, which xberg does perform. A caller of the raw port can therefore still make xberg fetch public URLs.

## Other behavior observed on the same run

- `Host` is not checked: `curl -H 'Host: rebind.example.test' http://127.0.0.1:53845/health` returned HTTP 200 with the health body.
- `DELETE /cache/clear` with `Origin: http://evil.example` returned HTTP 200 and cleared the (empty, scratch) cache directory: `{"directory":".../ssrf-cache","removed_files":0,"freed_mb":0.0}`.
- Startup log: "CORS configured to allow all origins (default). This permits CSRF attacks."
- Cleanup: `kill 64505` by the PID started here; `kill -0` confirmed it gone and `/health` no longer answered.
