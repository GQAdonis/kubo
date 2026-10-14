//go:build !windows

package cli

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of mcp-content-extractor against a real daemon and the real
// ipfs mcp binary. The xberg run itself is mcp_extractor_xberg_test.go; the
// tests here need no xberg binary.

// extractorTestPDF is a one-page PDF whose text is phrase.
func extractorTestPDF(phrase string) []byte {
	stream := fmt.Sprintf("BT /F1 14 Tf 72 720 Td (%s) Tj ET", phrase)
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objs))
	for i, o := range objs {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return out.Bytes()
}

// extractorTestDOCX is a minimal Word document with one paragraph, phrase.
func extractorTestDOCX(t *testing.T, phrase string) []byte {
	t.Helper()
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
			`<w:p><w:r><w:t>` + phrase + `</w:t></w:r></w:p></w:body></w:document>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.Create(p.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(p.body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// sortedLines orders the lines of a listing whose order the node does not fix.
func sortedLines(s string) []string {
	lines := strings.Fields(s)
	slices.Sort(lines)
	return lines
}

// addFixture adds bytes to the node (CIDv1, raw leaves, default chunker for a
// small file) and returns the CID. The node pins it, as `ipfs add` does.
func addFixture(node *harness.Node, data []byte) string {
	return node.IPFSAdd(bytes.NewReader(data), "--cid-version=1", "--raw-leaves", "--chunker=size-262144")
}

// extractorConfig is indexConfig with an embeddings.extractor block.
func extractorConfig(surrealBin, dataDir, embedBase, model string, dims int, policy, extractor string) string {
	return fmt.Sprintf(`{
  "principals": [{"name": "indexer", "kind": "human-mcp", "credential": {"env": "CLI_IDX_TOKEN"},
    "write_groups": ["content", "pin", "index"], "destructive_groups": ["pin", "diag"], "egress_ack": ["local"]}],
  "embeddings": {"mode": "embedded", "embedded": {"surreal_binary": %q, "data_dir": %q},
    "embedder": {"base_url": %q, "model": %q, "dimensions": %d}, "policy": %s, "extractor": %s}
}`, surrealBin, dataDir, embedBase, model, dims, policy, extractor)
}

// countingXberg is a stand-in xberg that counts the requests it gets.
func countingXberg(t *testing.T) (url string, requests *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		_, _ = w.Write([]byte(`{"results":[{"content":"cats extracted","mime_type":"application/pdf","metadata":{}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &n
}

// TestMCPExtractorDisabled: without an extractor block a PDF is skipped
// mime_not_allowed as in G12 and no xberg process or request exists; with an
// extractor but the PDF type absent from policy.mime_allowlist it is skipped
// kind_not_enabled and the extractor receives nothing.
func TestMCPExtractorDisabled(t *testing.T) {
	bin := surrealBinary(t)
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()
	pdf := addFixture(node, extractorTestPDF("cats are in this pdf"))
	refs := sortedLines(localRefs(node))

	t.Run("no extractor block", func(t *testing.T) {
		embed := newCountingEmbed(t)
		dataDir := filepath.Join(t.TempDir(), "idx")
		// The PDF type is allowlisted: only the missing extractor can skip it.
		p := startIndexMCP(t, node, indexConfig(bin, dataDir, embed.URL+"/v1", "fake", 8,
			`{"mime_allowlist": ["text/plain", "application/pdf"]}`), true)
		cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

		out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pdf}))
		assert.Equal(t, "mime_not_allowed", out["skipped_reason"])
		assert.EqualValues(t, 0, out["chunks_indexed"])
		assert.Zero(t, embed.requests.Load(), "nothing was embedded")
		for _, e := range psTable(t) {
			if e.ppid == p.cmd.Process.Pid {
				assert.NotContains(t, e.args, "xberg", "no xberg process exists: %s", e.args)
			}
		}
		assert.Equal(t, refs, sortedLines(localRefs(node)))
	})

	t.Run("extractor but type not allowlisted", func(t *testing.T) {
		embed := newCountingEmbed(t)
		xberg, xbergRequests := countingXberg(t)
		dataDir := filepath.Join(t.TempDir(), "idx")
		p := startIndexMCP(t, node, extractorConfig(bin, dataDir, embed.URL+"/v1", "fake", 8, `{}`,
			fmt.Sprintf(`{"mode": "remote", "remote": {"base_url": %q}}`, xberg)), true)
		cs := p.mustClient(indexTestToken, (&elicitor{action: "accept"}).options())

		out := okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": pdf}))
		assert.Equal(t, "kind_not_enabled", out["skipped_reason"])
		assert.Zero(t, xbergRequests.Load(), "the extractor received nothing")
		assert.Zero(t, embed.requests.Load())
		assert.Contains(t, p.stderr.String(), "extractor xberg at", "the startup line names the extractor")

		// Text keeps working natively beside the extractor.
		note := mcpAdd(t, cs, "cats nap in the sun", false)["cid"].(string)
		out = okResult(t, callMCP(t, cs, "ipfs_index_cid", map[string]any{"cid": note}))
		assert.EqualValues(t, 1, out["chunks_indexed"])
		assert.Zero(t, xbergRequests.Load())
	})
}
