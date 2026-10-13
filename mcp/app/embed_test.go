package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestEmbeddedViewsMatchManifest(t *testing.T) {
	m, err := LoadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if m.MimeType != MIMEType {
		t.Fatalf("manifest mime type = %q, want %q", m.MimeType, MIMEType)
	}
	if len(m.Views) != 5 {
		t.Fatalf("views = %d, want 5", len(m.Views))
	}
	for _, v := range m.Views {
		if want := "ui://ipfs-mcp/" + v.ID; v.URI != want {
			t.Errorf("%s uri = %q, want %q", v.ID, v.URI, want)
		}
		html, err := HTML(v)
		if err != nil {
			t.Fatal(err)
		}
		if len(html) != v.Bytes {
			t.Errorf("%s: %d bytes, manifest says %d", v.ID, len(html), v.Bytes)
		}
		sum := sha256.Sum256(html)
		if hex.EncodeToString(sum[:]) != v.SHA256 {
			t.Errorf("%s: digest differs from manifest", v.ID)
		}
		doc := string(html)
		if !strings.Contains(doc, "default-src 'none'") || !strings.Contains(doc, "connect-src 'none'") {
			t.Errorf("%s: missing the no-origin CSP", v.ID)
		}
		if strings.Contains(doc, "<script src") || strings.Contains(doc, "<link ") {
			t.Errorf("%s: references an external resource", v.ID)
		}
		if len(v.Tools) == 0 {
			t.Errorf("%s: no linked tools", v.ID)
		}
	}
}
