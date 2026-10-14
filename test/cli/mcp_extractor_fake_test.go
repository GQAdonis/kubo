//go:build !windows && !noagent

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeXbergScript is an executable that stands in for xberg 1.3.10: it answers
// --version, and `serve` runs an HTTP server with /health and POST /extract
// (chunked bodies included, as ipfs mcp streams them). It records the multipart
// field names of every /extract in fake-fields.txt in its working directory,
// the extractor's cache_dir. It needs python3 only.
const fakeXbergScript = `#!/usr/bin/env python3
import http.server, json, re, sys

if "--version" in sys.argv:
    print("xberg 1.3.10")
    sys.exit(0)

host = sys.argv[sys.argv.index("--host") + 1]
port = int(sys.argv[sys.argv.index("--port") + 1])

def read_body(h):
    if h.headers.get("Transfer-Encoding", "").lower() == "chunked":
        out = b""
        while True:
            size = int(h.rfile.readline().strip().split(b";")[0], 16)
            if size == 0:
                h.rfile.readline()
                return out
            out += h.rfile.read(size)
            h.rfile.readline()
    return h.rfile.read(int(h.headers.get("Content-Length", "0")))

class H(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def reply(self, status, obj):
        data = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        if self.path == "/health":
            self.reply(200, {"status": "healthy", "version": "1.3.10"})
        else:
            self.reply(404, {"message": "not found"})

    def do_POST(self):
        body = read_body(self)
        if self.path != "/extract":
            self.reply(404, {"message": "not found"})
            return
        names = re.findall(rb'[; ]name="([^"]+)"', body)
        with open("fake-fields.txt", "ab") as f:
            f.write(b",".join(names) + b"\n")
        self.reply(200, {"results": [{"content": "cats extracted by the fake script", "mime_type": "application/pdf", "metadata": {}}]})

http.server.ThreadingHTTPServer((host, port), H).serve_forever()
`

// TestMCPExtractorFakeXbergIsReapedAfterSIGKILL runs ipfs mcp with an embedded
// extractor whose xberg is a script, so it needs neither the real xberg nor
// IPFS_MCP_TEST_XBERG. It checks that a document is extracted through the
// authenticated proxy (only the allowed form fields reach xberg), and that
// SIGKILL of ipfs mcp, which runs no cleanup of its own, leaves no xberg
// process behind: the watchdog stops it.
func TestMCPExtractorFakeXbergIsReapedAfterSIGKILL(t *testing.T) {
	surreal := surrealBinary(t)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is needed to run the fake xberg script")
	}
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	pdf := addFixture(node, extractorTestPDF("cats are in this pdf"))

	work := t.TempDir()
	script := filepath.Join(work, "xberg")
	require.NoError(t, os.WriteFile(script, []byte(fakeXbergScript), 0o755))
	cache := filepath.Join(work, "xberg-cache")
	embed := newCountingEmbed(t)
	extractor, _ := json.Marshal(map[string]any{"mode": "embedded", "embedded": map[string]any{"xberg_binary": script, "cache_dir": cache}})
	p := startIndexMCP(t, node, extractorConfig(surreal, filepath.Join(work, "idx"), embed.URL+"/v1", "fake", 8,
		`{"mime_allowlist": ["text/plain", "application/pdf"]}`, string(extractor)), true)
	cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

	// The script is a child of ipfs mcp, bound to loopback on a non-default port.
	m := xbergLine.FindStringSubmatch(p.stderr.String())
	require.NotNil(t, m, "the startup line names the xberg address; stderr: %s", p.stderr.String())
	port, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	assert.NotContains(t, []int{4001, 5001, 8080, 5050, 8000}, port)
	var child psEntry
	for _, e := range grepServe(processesMentioning(t, cache)) {
		child = e
	}
	require.NotZero(t, child.pid, "no xberg process found for cache %s", cache)
	assert.Equal(t, p.cmd.Process.Pid, child.ppid, "xberg is a child of ipfs mcp")

	// A PDF is extracted and indexed through the proxy.
	out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pdf}))
	require.EqualValues(t, 1, out["chunks_indexed"], "%+v", out)
	hits := searchHits(t, cs, "cats", map[string]any{"top_k": 1})
	require.NotEmpty(t, hits)
	assert.Equal(t, pdf, hits[0]["cid"])
	assert.Equal(t, "cats extracted by the fake script", hits[0]["text"])
	assert.Equal(t, "extracted", hits[0]["source"])

	fields, err := os.ReadFile(filepath.Join(cache, "fake-fields.txt"))
	require.NoError(t, err)
	got := strings.Fields(string(fields))
	require.Len(t, got, 1, "one extraction reached xberg")
	assert.ElementsMatch(t, []string{"output_format", "files"}, strings.Split(got[0], ","), "only the allowed fields travel")

	// SIGKILL runs no deferred cleanup: the watchdog must stop xberg.
	mcpPID := p.cmd.Process.Pid
	require.NoError(t, p.cmd.Process.Kill())
	// startIndexMCP's goroutine reaps it; cmd.Wait here could block on a
	// descendant that still holds the stderr pipe.
	eventually(t, 10*time.Second, func() bool { return !pidAlive(mcpPID) }, "ipfs mcp pid %d did not die", mcpPID)
	eventually(t, 30*time.Second, func() bool { return !pidAlive(child.pid) },
		"fake xberg pid %d outlived a SIGKILLed ipfs mcp", child.pid)
	assert.Empty(t, grepServe(processesMentioning(t, cache)), "no xberg process is left for the test cache")
}
