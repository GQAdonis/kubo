package server

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ipfs/kubo/mcp/contract"
)

// This file is built with and without the noagent tag, like index.go.

// Content kinds, the keys of kinds in derivations/content-kinds.json.
const (
	kindText     = "text"
	kindDocument = "document"
	kindCode     = "code"
	kindData     = "data"
	kindImage    = "image"
	kindVideo    = "video"
)

// kindOfMIME maps a media type to its content kind through the contract's
// derivations/content-kinds.json, the single source of the map. It returns ""
// for a type the contract does not list.
func kindOfMIME(mime string) string { return contentKinds()[baseMIME(mime)] }

var contentKinds = sync.OnceValue(func() map[string]string {
	data, err := contract.FS().Open("derivations/content-kinds.json")
	if err != nil {
		panic("mcp contract: derivations/content-kinds.json is missing: " + err.Error())
	}
	defer data.Close()
	var doc struct {
		MimeTypes map[string]string `json:"mimeTypes"`
	}
	if err := json.NewDecoder(data).Decode(&doc); err != nil {
		panic("mcp contract: derivations/content-kinds.json is not valid: " + err.Error())
	}
	return doc.MimeTypes
})

func baseMIME(m string) string {
	m, _, _ = strings.Cut(m, ";")
	return strings.ToLower(strings.TrimSpace(m))
}

// extMIME lists file name extensions and the media type each stands for. The
// first extension listed for a type is the one sent to the extractor, which
// detects the format from it. Only types of the contract's map appear.
var extMIME = []struct{ ext, mime string }{
	{".txt", "text/plain"}, {".md", "text/markdown"}, {".markdown", "text/markdown"},
	{".pdf", "application/pdf"}, {".doc", "application/msword"},
	{".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	{".xls", "application/vnd.ms-excel"},
	{".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	{".ppt", "application/vnd.ms-powerpoint"},
	{".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	{".odt", "application/vnd.oasis.opendocument.text"},
	{".ods", "application/vnd.oasis.opendocument.spreadsheet"},
	{".odp", "application/vnd.oasis.opendocument.presentation"},
	{".pages", "application/vnd.apple.pages"}, {".numbers", "application/vnd.apple.numbers"},
	{".key", "application/vnd.apple.keynote"},
	{".rtf", "application/rtf"}, {".epub", "application/epub+zip"},
	{".html", "text/html"}, {".htm", "text/html"}, {".xhtml", "application/xhtml+xml"},
	{".js", "application/javascript"}, {".mjs", "application/javascript"}, {".ts", "application/x-typescript"},
	{".sh", "application/x-sh"}, {".sql", "application/sql"},
	{".py", "text/x-python"}, {".go", "text/x-go"}, {".java", "text/x-java-source"},
	{".c", "text/x-c"}, {".h", "text/x-c"}, {".cc", "text/x-c++"}, {".cpp", "text/x-c++"}, {".hpp", "text/x-c++"},
	{".rs", "text/x-rust"}, {".rb", "text/x-ruby"}, {".php", "text/x-php"}, {".pl", "text/x-perl"},
	{".swift", "text/x-swift"}, {".kt", "text/x-kotlin"}, {".scala", "text/x-scala"},
	{".hs", "text/x-haskell"}, {".lua", "text/x-lua"}, {".css", "text/css"},
	{".json", "application/json"}, {".jsonld", "application/ld+json"}, {".ndjson", "application/x-ndjson"},
	{".yaml", "application/yaml"}, {".yml", "application/yaml"}, {".toml", "application/toml"},
	{".csv", "text/csv"}, {".tsv", "text/tab-separated-values"}, {".xml", "application/xml"},
	{".png", "image/png"}, {".jpg", "image/jpeg"}, {".jpeg", "image/jpeg"}, {".gif", "image/gif"},
	{".webp", "image/webp"}, {".tif", "image/tiff"}, {".tiff", "image/tiff"}, {".bmp", "image/bmp"},
	{".avif", "image/avif"}, {".heic", "image/heic"},
	{".mp4", "video/mp4"}, {".webm", "video/webm"}, {".mov", "video/quicktime"}, {".mkv", "video/x-matroska"},
	{".mpg", "video/mpeg"}, {".mpeg", "video/mpeg"}, {".avi", "video/x-msvideo"},
}

func mimeByName(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ext == "" {
		return ""
	}
	for _, e := range extMIME {
		if e.ext == ext {
			return e.mime
		}
	}
	return ""
}

// extensionFor is the file name extension sent to the extractor for a type.
func extensionFor(mime string) string {
	mime = baseMIME(mime)
	for _, e := range extMIME {
		if e.mime == mime {
			return e.ext
		}
	}
	return ""
}

// classifyContent names the media type and content kind of a file: its binary
// signature first, then its name, then what the text looks like. The kind is
// "" for a type the contract does not list. Text that is only plain text keeps
// the type DetectMIME gives it, so the native path behaves as before.
func classifyContent(name string, data []byte) (mime, kind string) {
	mime = sniffBinary(data)
	if mime == "" || mime == "application/zip" {
		// A name settles what a signature cannot (an iWork package is a zip).
		if byName := mimeByName(name); byName != "" {
			mime = byName
		}
	}
	if mime == "" && looksLikeText(data) {
		mime = sniffText(data)
	}
	return mime, kindOfMIME(mime)
}

// looksLikeText reports whether the bytes are UTF-8 text without a NUL byte,
// the only content the native path indexes.
func looksLikeText(data []byte) bool {
	return utf8.Valid(data) && !slices.Contains(data, 0)
}

// sniffBinary recognises binary formats by signature. It returns "" for text
// and for anything it does not know.
func sniffBinary(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "application/pdf"
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return sniffZip(data)
	case bytes.HasPrefix(data, []byte(`{\rtf`)):
		return "application/rtf"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return sniffISOBMFF(string(data[8:12]))
	case bytes.HasPrefix(data, []byte{0x1a, 0x45, 0xdf, 0xa3}):
		head := data[:min(len(data), 64)]
		if bytes.Contains(head, []byte("webm")) {
			return "video/webm"
		}
		return "video/x-matroska"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "AVI ":
		return "video/x-msvideo"
	case bytes.HasPrefix(data, []byte{0x00, 0x00, 0x01, 0xba}), bytes.HasPrefix(data, []byte{0x00, 0x00, 0x01, 0xb3}):
		return "video/mpeg"
	case bytes.HasPrefix(data, []byte("II*\x00")), bytes.HasPrefix(data, []byte("MM\x00*")):
		return "image/tiff"
	}
	// The standard library knows the common image signatures.
	got := baseMIME(http.DetectContentType(data))
	if strings.HasPrefix(got, "image/") && kindOfMIME(got) != "" {
		return got
	}
	return ""
}

// sniffISOBMFF maps the major brand of an ISO base media file to a type.
func sniffISOBMFF(brand string) string {
	switch brand {
	case "avif", "avis":
		return "image/avif"
	case "heic", "heix", "hevc", "mif1", "msf1":
		return "image/heic"
	case "qt  ":
		return "video/quicktime"
	case "M4A ", "M4B ", "M4P ":
		// Audio only: the contract lists no audio type, so it is not a video to
		// transcribe.
		return "audio/mp4"
	}
	return "video/mp4"
}

// sniffZip tells Office, OpenDocument and EPUB containers from a plain zip by
// the entry names of its central directory. The data is already bounded by the
// caller; the archive is read in memory and never extracted.
func sniffZip(data []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "application/zip"
	}
	names := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		names[f.Name] = f
	}
	switch {
	case names["word/document.xml"] != nil:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case names["xl/workbook.xml"] != nil:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case names["ppt/presentation.xml"] != nil:
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	}
	if f := names["mimetype"]; f != nil && f.UncompressedSize64 <= 128 {
		if rc, err := f.Open(); err == nil {
			b, _ := io.ReadAll(io.LimitReader(rc, 129))
			_ = rc.Close()
			if m := baseMIME(string(b)); strings.HasPrefix(m, "application/vnd.oasis.opendocument.") || m == "application/epub+zip" {
				return m
			}
		}
	}
	return "application/zip"
}

// sniffText refines UTF-8 text into JSON, XML, HTML, CSV, TSV, a shell or
// scripting-language script by shebang, or plain text. Markdown, YAML and most
// source code cannot be told from plain text by content, only by name.
func sniffText(data []byte) string {
	trim := bytes.TrimSpace(data[:min(len(data), 4096)])
	switch {
	case len(trim) == 0:
		return "text/plain"
	case bytes.HasPrefix(trim, []byte("#!")):
		return sniffShebang(trim)
	case bytes.HasPrefix(trim, []byte("<?php")):
		return "text/x-php"
	case (trim[0] == '{' || trim[0] == '[') && json.Valid(bytes.TrimSpace(data)):
		return "application/json"
	case bytes.HasPrefix(trim, []byte("<?xml")) && !bytes.Contains(bytes.ToLower(trim), []byte("<html")):
		return "application/xml"
	}
	if got := baseMIME(http.DetectContentType(data)); got == "text/html" {
		return got
	}
	window := delimitedWindow(data, trim)
	switch {
	case delimited(window, ','):
		return "text/csv"
	case delimited(window, '\t'):
		return "text/tab-separated-values"
	}
	return "text/plain"
}

// delimitedWindow is the part of the text the CSV and TSV check reads: the
// trimmed window itself when it holds the whole file, else only the complete
// lines of it. A window cut in the middle of a record fails the field count, so
// wide rows (a few hundred bytes each) would never be recognised.
func delimitedWindow(data, trim []byte) []byte {
	if len(bytes.TrimSpace(data)) == len(trim) {
		return trim
	}
	if i := bytes.LastIndexByte(trim, '\n'); i >= 0 {
		return trim[:i]
	}
	return nil
}

// delimited reports whether the text parses as at least two records with the
// same number of at least two fields.
func delimited(text []byte, comma rune) bool {
	r := csv.NewReader(bytes.NewReader(text))
	r.Comma = comma
	r.LazyQuotes = true
	recs := 0
	for recs < 20 {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) < 2 {
			return false
		}
		recs++
	}
	return recs >= 2
}

// sniffShebang names the language of a script by its interpreter.
func sniffShebang(text []byte) string {
	line, _, _ := bytes.Cut(text, []byte("\n"))
	l := strings.ToLower(string(line))
	switch {
	case strings.Contains(l, "python"):
		return "text/x-python"
	case strings.Contains(l, "node"):
		return "application/javascript"
	case strings.Contains(l, "ruby"):
		return "text/x-ruby"
	case strings.Contains(l, "perl"):
		return "text/x-perl"
	case strings.Contains(l, "php"):
		return "text/x-php"
	case strings.Contains(l, "sh"):
		return "application/x-sh"
	}
	return "text/plain"
}
