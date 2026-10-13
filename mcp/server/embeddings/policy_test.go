package embeddings

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyDefaults(t *testing.T) {
	t.Parallel()
	p := Policy{}.Effective()
	assert.False(t, p.Auto, "automatic indexing is off by default")
	assert.Equal(t, []string{"text/plain", "text/markdown"}, p.MIMEAllowlist)
	assert.Equal(t, 1<<20, p.MaxBytes)
	assert.Equal(t, 2000, p.ChunkChars)

	// An empty allowlist is the off switch and is not replaced by the default.
	p = Policy{MIMEAllowlist: []string{}}.Effective()
	assert.Empty(t, p.MIMEAllowlist)
	assert.False(t, p.AllowsMIME("text/plain"))
}

func TestPolicyValidate(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		p    Policy
		want string
	}{
		"defaults":         {Policy{}, ""},
		"explicit":         {Policy{Auto: true, MIMEAllowlist: []string{"text/x-go"}, MaxBytes: 5, ChunkChars: 200}, ""},
		"max_bytes high":   {Policy{MaxBytes: MaxMaxBytes + 1}, "max_bytes"},
		"max_bytes neg":    {Policy{MaxBytes: -1}, "max_bytes"},
		"chunk_chars low":  {Policy{ChunkChars: 10}, "chunk_chars"},
		"chunk_chars high": {Policy{ChunkChars: MaxChunkChars + 1}, "chunk_chars"},
		"bad mime":         {Policy{MIMEAllowlist: []string{"Text"}}, "mime_allowlist"},
	} {
		err := Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/b", DataDir: "/d"}, Embedder: validEmbedder(), Policy: tc.p}.Validate()
		if tc.want == "" {
			assert.NoError(t, err, name)
			continue
		}
		assert.ErrorContains(t, err, tc.want, name)
	}
}

func TestCheckContent(t *testing.T) {
	t.Parallel()
	p := Policy{MaxBytes: 40}.Effective()
	for name, tc := range map[string]struct {
		file, data, mime, skip string
	}{
		"plain by sniffing":     {"", "cats purr", "text/plain", ""},
		"txt extension":         {"a.txt", "cats purr", "text/plain", ""},
		"markdown extension":    {"README.MD", "# cats", "text/markdown", ""},
		"markup is not allowed": {"p.html", "<html><body>cats</body></html>", "text/html", SkipMIME},
		"too large":             {"a.txt", strings.Repeat("x", 41), "", SkipTooLarge},
		"not utf-8":             {"a.txt", "\xff\xfe", "", SkipMIME},
		"nul byte":              {"a.txt", "ca\x00ts", "", SkipMIME},
		"binary signature":      {"", "%PDF-1.4 \x01\x02", "", SkipMIME},
	} {
		mime, skip := p.CheckContent(tc.file, []byte(tc.data))
		assert.Equal(t, tc.skip, skip, name)
		if tc.skip != SkipMIME || tc.mime != "" {
			assert.Equal(t, tc.mime, mime, name)
		}
	}
	assert.True(t, Policy{MIMEAllowlist: []string{"text/html; charset=utf-8"}}.Effective().AllowsMIME("TEXT/HTML"), "parameters and case are ignored")
}

func TestChunk(t *testing.T) {
	t.Parallel()
	assert.Empty(t, Chunk("", 200))
	assert.Empty(t, Chunk(" \n\t ", 200))
	assert.Equal(t, []string{"short text"}, Chunk("  short text\n", 200))

	para := strings.Repeat("word ", 30) // 150 characters
	text := para + "\n\n" + para + "\n" + para
	chunks := Chunk(text, 200)
	require.Greater(t, len(chunks), 1)
	for _, c := range chunks {
		assert.LessOrEqual(t, utf8.RuneCountInString(c), 200)
		assert.Equal(t, strings.TrimSpace(c), c, "pieces are trimmed")
		assert.NotEmpty(t, c)
	}
	assert.Equal(t, strings.TrimSpace(para), chunks[0], "the cut is at the paragraph break")
	assert.Equal(t, strings.Fields(text), strings.Fields(strings.Join(chunks, " ")), "no word is lost or split")

	// A run with no break is cut hard at the size; multibyte characters count once.
	hard := strings.Repeat("é", 450)
	pieces := Chunk(hard, 200)
	assert.Equal(t, []int{200, 200, 50}, []int{utf8.RuneCountInString(pieces[0]), utf8.RuneCountInString(pieces[1]), utf8.RuneCountInString(pieces[2])})
	assert.Equal(t, hard, strings.Join(pieces, ""))

	// The split is a pure function of the text and the size: a search re-cuts
	// the content and finds the chunk by its index.
	assert.Equal(t, Chunk(text, 200), Chunk(text, 200))
	assert.NotEqual(t, len(Chunk(text, 200)), len(Chunk(text, 1000)))
}
