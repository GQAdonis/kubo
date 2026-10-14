package server

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyContent(t *testing.T) {
	t.Parallel()
	docx := zipOf(t, "[Content_Types].xml", "word/document.xml")
	xlsx := zipOf(t, "[Content_Types].xml", "xl/workbook.xml")
	pptx := zipOf(t, "ppt/presentation.xml")
	plainZip := zipOf(t, "a.txt")
	tests := []struct {
		name     string
		file     string
		data     []byte
		wantMIME string
		wantKind string
	}{
		{"pdf by signature", "", pdfBytes, "application/pdf", kindDocument},
		{"docx by container", "", docx, docxMIME, kindDocument},
		{"xlsx by container", "", xlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", kindDocument},
		{"pptx by container", "", pptx, "application/vnd.openxmlformats-officedocument.presentationml.presentation", kindDocument},
		{"plain zip has no kind", "", plainZip, "application/zip", ""},
		{"iwork zip named by extension", "notes.pages", plainZip, "application/vnd.apple.pages", kindDocument},
		{"rtf", "", []byte(`{\rtf1\ansi hello}`), "application/rtf", kindDocument},
		{"html", "", []byte("<!DOCTYPE html><html><body>hi</body></html>"), "text/html", kindDocument},
		{"python by shebang", "", []byte("#!/usr/bin/env python3\nprint(1)\n"), "text/x-python", kindCode},
		{"shell by shebang", "", []byte("#!/bin/sh\necho hi\n"), "application/x-sh", kindCode},
		{"go by name", "main.go", []byte("package main\n"), "text/x-go", kindCode},
		{"rust by name", "lib.rs", []byte("fn main() {}\n"), "text/x-rust", kindCode},
		{"json by content", "", []byte(`{"a": [1, 2]}`), "application/json", kindData},
		{"json by name", "data.json", []byte("{not json"), "application/json", kindData},
		{"csv by content", "", []byte("a,b\n1,2\n3,4\n"), "text/csv", kindData},
		{"tsv by content", "", []byte("a\tb\n1\t2\n"), "text/tab-separated-values", kindData},
		{"xml by content", "", []byte("<?xml version=\"1.0\"?><r/>"), "application/xml", kindData},
		{"yaml by name", "c.yml", []byte("a: 1\n"), "application/yaml", kindData},
		{"png", "", pngBytes, "image/png", kindImage},
		{"jpeg", "", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01"), "image/jpeg", kindImage},
		{"gif", "", []byte("GIF89a\x01\x00\x01\x00"), "image/gif", kindImage},
		{"mp4", "", mp4Bytes, "video/mp4", kindVideo},
		{"webm", "", append([]byte{0x1a, 0x45, 0xdf, 0xa3}, []byte("\x01\x00\x00\x00webm")...), "video/webm", kindVideo},
		{"plain text", "", []byte("just some prose, with a comma\nand another line\n"), "text/plain", kindText},
		{"markdown by name", "README.md", []byte("# hi\n"), "text/markdown", kindText},
		{"wide csv rows are still csv", "", wideDelimited(',', 40, 320), "text/csv", kindData},
		{"wide tsv rows are still tsv", "", wideDelimited('\t', 40, 320), "text/tab-separated-values", kindData},
		{"one record wider than the window is not classified", "", append(wideDelimited(',', 1, 6000), []byte("\nx,y\n")...), "text/plain", kindText},
		{"m4a audio is not a video", "", append([]byte("\x00\x00\x00\x18ftypM4A \x00\x00\x00\x00M4A mp42"), make([]byte, 32)...), "audio/mp4", ""},
		{"prose with commas is not csv", "", []byte("One, two and three.\nSeven.\n"), "text/plain", kindText},
		{"binary of no known type", "", []byte{0x00, 0x01, 0xfe, 0xff, 0x00}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mime, kind := classifyContent(tt.file, tt.data)
			assert.Equal(t, tt.wantMIME, mime)
			assert.Equal(t, tt.wantKind, kind)
		})
	}
}

// TestKindsComeFromTheContract pins that every type this file names is in the
// contract's map, so the two cannot drift.
// wideDelimited is a header and rows data rows of about width bytes each, so the
// text is longer than the window the classifier reads.
func wideDelimited(comma byte, rows, width int) []byte {
	var b []byte
	field := bytes.Repeat([]byte("v"), width/3)
	for i := 0; i <= rows; i++ {
		b = append(b, field...)
		b = append(b, comma)
		b = append(b, field...)
		b = append(b, comma)
		b = append(b, field...)
		b = append(b, '\n')
	}
	return b
}

func TestKindsComeFromTheContract(t *testing.T) {
	t.Parallel()
	for _, e := range extMIME {
		assert.NotEmpty(t, kindOfMIME(e.mime), "%s (%s) is not in derivations/content-kinds.json", e.ext, e.mime)
	}
	assert.Equal(t, kindText, kindOfMIME("text/plain; charset=utf-8"))
	assert.Equal(t, kindDocument, kindOfMIME("Application/PDF"))
	assert.Empty(t, kindOfMIME("application/x-nothing"))
	assert.Equal(t, ".pdf", extensionFor("application/pdf"))
	assert.Empty(t, extensionFor("application/x-nothing"))
}

func TestExtractorConfigResolvesToSettings(t *testing.T) {
	t.Parallel()
	var cfg Config
	require.NoError(t, json.Unmarshal([]byte(`{"embeddings":{"mode":"embedded",
		"embedded":{"surreal_binary":"/opt/surreal","data_dir":"/var/surreal"},
		"embedder":{"base_url":"http://127.0.0.1:11434/v1","model":"bge-m3","dimensions":1024},
		"extractor":{"mode":"embedded","embedded":{"xberg_binary":"/opt/xberg","cache_dir":"/var/xberg","allow_model_downloads":false},
			"timeout_seconds":30,"max_bytes":1000,"max_text_bytes":500,"ocr":true}}}`), &cfg))
	set, err := cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.NoError(t, set.Validate())
	x := set.Extractor
	require.NotNil(t, x)
	assert.Equal(t, embeddings.ModeEmbedded, x.Mode)
	assert.Equal(t, "/opt/xberg", x.Embedded.Binary)
	assert.Equal(t, "/var/xberg", x.Embedded.CacheDir)
	assert.False(t, x.Embedded.AllowModelDownloads)
	assert.Equal(t, 30, int(x.Timeout.Seconds()))
	assert.Equal(t, 1000, x.MaxBytes)
	assert.Equal(t, 500, x.MaxTextBytes)
	assert.True(t, x.OCR)
	assert.False(t, x.Transcribe)
	assert.Equal(t, ReservedPorts, x.Embedded.AvoidPorts)

	// A relative binary path is refused by the settings, not by a PATH lookup.
	cfg.Embeddings.Extractor.Embedded.XbergBinary = "xberg"
	set, err = cfg.Embeddings.Settings()
	require.NoError(t, err)
	require.ErrorContains(t, set.Validate(), "absolute path")

	// No extractor block: no extractor settings at all.
	cfg.Embeddings.Extractor = nil
	set, err = cfg.Embeddings.Settings()
	require.NoError(t, err)
	assert.Nil(t, set.Extractor)
}
