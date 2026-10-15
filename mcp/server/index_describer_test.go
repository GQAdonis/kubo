package server

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mediaEnv is an index environment whose service can have a remote extractor
// and a describer, over a store that keeps kind, source, text and described_by.
type mediaEnv struct {
	*indexEnv
	store *kindStore
	x     *extractorFake
	chat  *chatFake
	svc   *embeddings.Service
}

type mediaOpts struct {
	policy embeddings.Policy
	// extractor, when set, adds a remote extractor after mutating its settings.
	extractor func(*embeddings.ExtractorSettings)
	// describer, when set, adds a describer after mutating its settings.
	describer func(*embeddings.DescriberSettings)
	bulk      BulkConfig
	// store, when set, is shared with another environment: a second ipfs mcp
	// over the same index.
	store *kindStore
	// options are the server options (resolvers, tool selection).
	options Options
}

func newMediaEnv(t *testing.T, o mediaOpts) *mediaEnv {
	t.Helper()
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true, options: o.options})
	m := &mediaEnv{indexEnv: e, store: o.store, chat: newChatFake(t)}
	if m.store == nil {
		m.store = &kindStore{}
	}
	set := embeddings.Settings{
		Mode:     embeddings.ModeEmbedded,
		Embedder: embeddings.EmbedderSettings{BaseURL: e.embed.URL + "/v1", Model: "fake-embed", Dimensions: indexTestDims},
		Policy:   o.policy,
	}
	emb, err := embeddings.NewEmbedder(set.Embedder, nil)
	require.NoError(t, err)
	m.svc = embeddings.NewService(set, m.store, emb)
	if o.extractor != nil {
		m.x = newExtractorFake(t)
		xs := embeddings.ExtractorSettings{Mode: embeddings.ModeRemote, Remote: &embeddings.ExtractorRemote{BaseURL: m.x.URL}}
		o.extractor(&xs)
		ext, err := embeddings.NewExtractor(xs, nil)
		require.NoError(t, err)
		m.svc.WithExtractor(ext)
	}
	if o.describer != nil {
		ds := embeddings.DescriberSettings{BaseURL: m.chat.URL + "/v1", Model: "vision-test"}
		o.describer(&ds)
		d, err := embeddings.NewDescriber(ds, nil)
		require.NoError(t, err)
		m.svc.WithDescriber(d)
	}
	e.s.bulk.cfg = o.bulk
	e.s.attachEmbeddings(m.svc)
	return m
}

// principal acknowledges the embedder and every stage the environment has.
func (m *mediaEnv) principal() Identity {
	acks := []string{m.embed.host()}
	if m.x != nil {
		acks = append(acks, m.x.host())
	}
	if m.svc.Describer() != nil {
		acks = append(acks, m.chat.host())
	}
	return indexIdentity(acks...)
}

// call runs ipfs_index_cid as id and returns the raw result.
func (m *mediaEnv) call(id Identity, cid string, force bool) *mcp.CallToolResult {
	m.t.Helper()
	return toolCall(m.t, m.connect(&id), "ipfs_index_cid", map[string]any{"cid": cid, "force": force})
}

func (m *mediaEnv) index(id Identity, cid string, force bool) map[string]any {
	m.t.Helper()
	res := m.call(id, cid, force)
	require.False(m.t, res.IsError, "%+v", res.Content)
	out := structured(m.t, res)
	validateOutput(m.t, "ipfs_index_cid", out)
	return out
}

func (m *mediaEnv) rows() []embeddings.Row { return m.store.snapshot() }

// --- 1.1 and 1.3: images are described and stored --------------------------------

func TestDescriberIndexesAnImageAsADescription(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(s *embeddings.DescriberSettings) { s.APIKey = "sekrit" },
	})
	m.chat.setReply(func(describeReq) (int, string) { return 200, "cats asleep on a red sofa" })
	m.files["/ipfs/bafyPNG"] = pngBytes

	out := m.index(m.principal(), "bafyPNG", false)
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.NotContains(t, out, "skipped_reason")

	reqs := m.chat.requests()
	require.Len(t, reqs, 1)
	assert.Equal(t, "vision-test", reqs[0].Model)
	assert.Equal(t, "Bearer sekrit", reqs[0].Auth)
	assert.Equal(t, []string{"text", "image_url"}, reqs[0].Parts)
	require.Len(t, reqs[0].URLs, 1)
	const prefix = "data:image/png;base64,"
	require.True(t, strings.HasPrefix(reqs[0].URLs[0], prefix))
	got, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(reqs[0].URLs[0], prefix))
	require.NoError(t, err)
	assert.Equal(t, pngBytes, got)
	assert.NotContains(t, reqs[0].Raw, "bafyPNG", "the request names neither the CID nor a node or gateway URL")
	assert.NotContains(t, reqs[0].Raw, "/ipfs/")
	assert.NotContains(t, reqs[0].Raw, "http")

	rows := m.rows()
	require.Len(t, rows, 1)
	r := rows[0]
	assert.Equal(t, "image", r.Kind)
	assert.Equal(t, "description", r.Source)
	assert.Equal(t, "vision-test", r.DescribedBy)
	assert.Equal(t, "image/png", r.MIME)
	assert.Equal(t, "cats asleep on a red sofa", r.Text, "the description is stored with the vector")
	assert.Equal(t, "/ipfs/bafyPNG", r.Path)
}

func TestDescriberForceReplacesTheDescriptionEntries(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.files["/ipfs/bafyPNG"] = pngBytes
	m.chat.setReply(func(describeReq) (int, string) { return 200, "cats first description" })
	m.index(m.principal(), "bafyPNG", false)

	m.chat.setReply(func(describeReq) (int, string) { return 200, "dogs second description" })
	again := m.index(m.principal(), "bafyPNG", false)
	assert.Equal(t, "already_indexed", again["skipped_reason"])
	assert.Len(t, m.chat.requests(), 1, "an indexed file is not described again without force")

	forced := m.index(m.principal(), "bafyPNG", true)
	assert.EqualValues(t, 1, forced["chunks_indexed"])
	rows := m.rows()
	require.Len(t, rows, 1, "force replaces the entries of the file, it does not add to them")
	assert.Equal(t, "dogs second description", rows[0].Text)
	assert.Equal(t, "description", rows[0].Source)
}

func TestDescriptionHitsCarryDescribedByAndTheUntrustedMarking(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.files["/ipfs/bafyPNG"] = pngBytes
	m.index(m.principal(), "bafyPNG", false)

	id := m.principal()
	res := toolCall(t, m.connect(&id), "ipfs_index_search", map[string]any{"query": "cats", "top_k": 3})
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	validateOutput(t, "ipfs_index_search", out)
	hits, _ := out["results"].([]any)
	require.Len(t, hits, 1)
	hit := hits[0].(map[string]any)
	assert.Equal(t, "description", hit["source"])
	assert.Equal(t, "vision-test", hit["described_by"], "described_by is always present on a description hit")
	assert.Equal(t, "image", hit["kind"])
	assert.Equal(t, "image/png", hit["mime_type"])
	assert.Equal(t, "cats asleep on a sofa", hit["text"])
	assert.Equal(t, true, res.Meta[untrustedMetaKey], "search results are marked untrusted, descriptions included")
}

func TestOnlyDescriptionHitsNameAModel(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{policy: embeddings.Policy{MIMEAllowlist: []string{"image/png", "text/plain"}}, describer: func(*embeddings.DescriberSettings) {}})
	m.files["/ipfs/bafyText"] = []byte("cats purr")
	m.index(m.principal(), "bafyText", false)
	id := m.principal()
	out := structured(t, toolCall(t, m.connect(&id), "ipfs_index_search", map[string]any{"query": "cats"}))
	hit := out["results"].([]any)[0].(map[string]any)
	assert.NotContains(t, hit, "described_by")
}

// --- stages are independent -------------------------------------------------------

func TestOCRAndDescriptionOfOneImageAreBothStored(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		extractor: func(s *embeddings.ExtractorSettings) { s.OCR = true },
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.x.setReply(func(extractedFile) (int, string) {
		return 200, `{"results":[{"content":"cats text found in the picture","mime_type":"image/png","metadata":{"ocr_used":true}}]}`
	})
	m.files["/ipfs/bafyPNG"] = pngBytes

	out := m.index(m.principal(), "bafyPNG", false)
	assert.EqualValues(t, 2, out["chunks_indexed"])
	rows := m.rows()
	require.Len(t, rows, 2)
	bySource := map[string]embeddings.Row{}
	for _, r := range rows {
		bySource[r.Source] = r
	}
	require.Contains(t, bySource, "ocr")
	require.Contains(t, bySource, "description")
	assert.NotEqual(t, bySource["ocr"].ChunkIndex, bySource["description"].ChunkIndex, "chunk indexes continue across stages")
	assert.Empty(t, bySource["ocr"].DescribedBy)
	assert.Equal(t, "vision-test", bySource["description"].DescribedBy)
}

// --- 1.4: egress ----------------------------------------------------------------------

func TestDescriberHostNotAcknowledgedSendsNothingAndOCRStillRuns(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		extractor: func(s *embeddings.ExtractorSettings) { s.OCR = true },
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.files["/ipfs/bafyPNG"] = pngBytes
	id := indexIdentity(m.embed.host(), m.x.host()) // not the describer

	res := m.call(id, "bafyPNG", false)
	require.False(t, res.IsError, "%+v %+v", res.Content[0], res.StructuredContent)
	assert.EqualValues(t, 1, structured(t, res)["chunks_indexed"], "the OCR stage still produced text")
	assert.Empty(t, m.chat.requests(), "the describer received zero requests")
	assert.Len(t, m.x.requests(), 1)
	warnings, _ := res.Meta[stageWarningsMetaKey].([]any)
	require.Len(t, warnings, 1)
	assert.Equal(t, "describer:egress_not_acknowledged", warnings[0], "a stage and a fixed reason code")
	assert.NotContains(t, warnings[0], m.chat.host(), "no host or endpoint text rides in the result")
	rows := m.rows()
	require.Len(t, rows, 1)
	assert.Equal(t, "ocr", rows[0].Source)
}

func TestDescriberAloneUnacknowledgedIsEgressNotAcknowledgedNamingTheHost(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.files["/ipfs/bafyPNG"] = pngBytes
	id := indexIdentity(m.embed.host())

	res := m.call(id, "bafyPNG", false)
	require.True(t, res.IsError)
	te := toolErr(t, res)
	assert.Equal(t, CodeEgressNotAcknowledged, te.Code)
	assert.Contains(t, te.Message, m.chat.host())
	assert.Contains(t, te.Message, "embeddings.describer.base_url", "the remedy points at the right setting")
	assert.Equal(t, []any{m.chat.host()}, te.Details["missing_hosts"])
	assert.Empty(t, m.chat.requests())
	assert.Empty(t, m.rows())
}

func TestLoopbackDescriberIsCoveredByLocal(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.files["/ipfs/bafyPNG"] = pngBytes
	out := m.index(indexIdentity("local"), "bafyPNG", false)
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Len(t, m.chat.requests(), 1)
}

func TestDescriberHostJoinsTheBackendHostsOfAgentPrincipals(t *testing.T) {
	enabled := &Config{Embeddings: &EmbeddingsConfig{Describer: &DescriberConfig{BaseURL: "https://api.describer.test/v1", Model: "m"}}}
	assert.Contains(t, enabled.BackendHosts(), "api.describer.test")

	zero := 0
	disabled := &Config{Embeddings: &EmbeddingsConfig{Describer: &DescriberConfig{BaseURL: "https://api.describer.test/v1", Model: "m", MaxRequestsPerHour: &zero}}}
	assert.NotContains(t, disabled.BackendHosts(), "api.describer.test")
	empty := &Config{Embeddings: &EmbeddingsConfig{Describer: &DescriberConfig{BaseURL: "https://api.describer.test/v1", Model: "m", Kinds: []string{}}}}
	assert.NotContains(t, empty.BackendHosts(), "api.describer.test")
}

// --- 1.2: video -----------------------------------------------------------------------

func TestVideoIsDescribedOnlyWhenListedAndTheTranscriptIsIndexedEitherWay(t *testing.T) {
	policy := embeddings.Policy{MIMEAllowlist: []string{"video/mp4"}}
	transcribe := func(s *embeddings.ExtractorSettings) { s.Transcribe = true }

	t.Run("default kinds do not send the video", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: policy, extractor: transcribe, describer: func(*embeddings.DescriberSettings) {}})
		m.x.setReply(func(extractedFile) (int, string) {
			return 200, `{"results":[{"content":"cats spoken in the audio","mime_type":"video/mp4","metadata":{}}]}`
		})
		m.files["/ipfs/bafyMP4"] = mp4Bytes
		out := m.index(m.principal(), "bafyMP4", false)
		assert.EqualValues(t, 1, out["chunks_indexed"])
		assert.Empty(t, m.chat.requests(), "kinds is [image]: nothing is sent for a video")
		rows := m.rows()
		require.Len(t, rows, 1)
		assert.Equal(t, "transcript", rows[0].Source)
	})

	t.Run("video listed", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: policy, extractor: transcribe, describer: func(s *embeddings.DescriberSettings) {
			s.Kinds = []string{embeddings.KindImage, embeddings.KindVideo}
			s.VideoFPS = 2
		}})
		m.x.setReply(func(extractedFile) (int, string) {
			return 200, `{"results":[{"content":"dogs spoken in the audio","mime_type":"video/mp4","metadata":{}}]}`
		})
		m.files["/ipfs/bafyMP4"] = mp4Bytes
		out := m.index(m.principal(), "bafyMP4", false)
		assert.EqualValues(t, 2, out["chunks_indexed"])
		reqs := m.chat.requests()
		require.Len(t, reqs, 1)
		assert.Equal(t, []string{"text", "video_url"}, reqs[0].Parts)
		assert.True(t, strings.HasPrefix(reqs[0].URLs[0], "data:video/mp4;base64,"))
		assert.InDelta(t, 2, reqs[0].FPS, 0.001)
		assert.Equal(t, "default", reqs[0].Resolution)
		sources := map[string]bool{}
		for _, r := range m.rows() {
			sources[r.Source] = true
		}
		assert.Equal(t, map[string]bool{"transcript": true, "description": true}, sources)
	})

	t.Run("a describer that rejects the video part fails the stage and keeps the transcript", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: policy, extractor: transcribe, describer: func(s *embeddings.DescriberSettings) {
			s.Kinds = []string{embeddings.KindImage, embeddings.KindVideo}
		}})
		m.chat.setReply(func(describeReq) (int, string) { return 400, "unknown content part video_url" })
		m.files["/ipfs/bafyMP4"] = mp4Bytes
		res := m.call(m.principal(), "bafyMP4", false)
		require.False(t, res.IsError, "%+v", res.Content)
		assert.EqualValues(t, 1, structured(t, res)["chunks_indexed"])
		assert.Len(t, m.chat.requests(), 1)
		warnings, _ := res.Meta[stageWarningsMetaKey].([]any)
		require.Len(t, warnings, 1, "the rejected stage is a warning on the file")
		assert.Equal(t, "describer:extraction_failed", warnings[0])
		rows := m.rows()
		require.Len(t, rows, 1)
		assert.Equal(t, "transcript", rows[0].Source)
	})

	t.Run("an oversize video is not sent", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: policy, describer: func(s *embeddings.DescriberSettings) {
			s.Kinds = []string{embeddings.KindVideo}
			s.VideoMaxBytes = 16
		}})
		m.files["/ipfs/bafyMP4"] = mp4Bytes
		out := m.index(m.principal(), "bafyMP4", false)
		assert.Equal(t, "too_large", out["skipped_reason"])
		assert.Empty(t, m.chat.requests())
	})
}

// --- 1.6: disabled states -------------------------------------------------------------

func TestWithoutADescriberImagesAndVideosAreSkippedNamingIt(t *testing.T) {
	allowed := embeddings.Policy{MIMEAllowlist: []string{"image/png", "video/mp4"}}

	t.Run("nothing configured", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: allowed})
		m.files["/ipfs/bafyPNG"] = pngBytes
		m.files["/ipfs/bafyMP4"] = mp4Bytes
		id := indexIdentity(m.embed.host())
		for _, c := range []string{"bafyPNG", "bafyMP4"} {
			assert.Equal(t, "describer_not_configured", m.index(id, c, false)["skipped_reason"], c)
		}
		assert.Empty(t, m.chat.requests(), "no description request exists")
		assert.Zero(t, m.embed.requests.Load())
	})

	t.Run("an extractor with OCR off and no describer: the describer is what is missing", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: allowed, extractor: func(*embeddings.ExtractorSettings) {}})
		m.files["/ipfs/bafyPNG"] = pngBytes
		assert.Equal(t, "describer_not_configured", m.index(m.principal(), "bafyPNG", false)["skipped_reason"])
	})

	t.Run("OCR covers the image", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: allowed, extractor: func(s *embeddings.ExtractorSettings) { s.OCR = true }})
		m.files["/ipfs/bafyPNG"] = pngBytes
		out := m.index(m.principal(), "bafyPNG", false)
		assert.EqualValues(t, 1, out["chunks_indexed"])
	})

	t.Run("a block with no kinds describes nothing", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: allowed, describer: func(s *embeddings.DescriberSettings) { s.Kinds = []string{} }})
		m.files["/ipfs/bafyPNG"] = pngBytes
		assert.Equal(t, "describer_not_configured", m.index(m.principal(), "bafyPNG", false)["skipped_reason"])
		assert.Empty(t, m.chat.requests())
	})

	t.Run("a zero hourly limit disables the describer", func(t *testing.T) {
		zero := 0
		m := newMediaEnv(t, mediaOpts{policy: allowed, describer: func(s *embeddings.DescriberSettings) { s.MaxRequestsPerHour = &zero }})
		m.files["/ipfs/bafyPNG"] = pngBytes
		assert.Equal(t, "describer_not_configured", m.index(m.principal(), "bafyPNG", false)["skipped_reason"])
		assert.Empty(t, m.chat.requests())
	})

	t.Run("a type outside the allowlist is not described", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: embeddings.Policy{}, describer: func(*embeddings.DescriberSettings) {}})
		m.files["/ipfs/bafyPNG"] = pngBytes
		assert.Equal(t, "kind_not_enabled", m.index(m.principal(), "bafyPNG", false)["skipped_reason"])
		assert.Empty(t, m.chat.requests())
	})

	t.Run("an oversize image is not described", func(t *testing.T) {
		m := newMediaEnv(t, mediaOpts{policy: allowed, describer: func(s *embeddings.DescriberSettings) { s.ImageMaxBytes = 16 }})
		m.files["/ipfs/bafyPNG"] = pngBytes
		assert.Equal(t, "too_large", m.index(m.principal(), "bafyPNG", false)["skipped_reason"])
		assert.Empty(t, m.chat.requests())
	})
}

func TestDescriberDoesNotChangeTheTextPath(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{describer: func(*embeddings.DescriberSettings) {}})
	m.files["/ipfs/bafyText"] = []byte("cats purr when content")
	out := m.index(m.principal(), "bafyText", false)
	assert.EqualValues(t, 1, out["chunks_indexed"])
	assert.Empty(t, m.chat.requests(), "text is never described")
	assert.Equal(t, "text", m.rows()[0].Source)
}

// --- failures -------------------------------------------------------------------------

func TestDescriberFailureIsASkipNotAHardError(t *testing.T) {
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(*embeddings.DescriberSettings) {},
	})
	m.chat.setReply(func(describeReq) (int, string) { return 502, "model host down" })
	m.files["/ipfs/bafyPNG"] = pngBytes
	res := m.call(m.principal(), "bafyPNG", false)
	require.False(t, res.IsError, "%+v", res.Content)
	out := structured(t, res)
	assert.Equal(t, "extraction_failed", out["skipped_reason"])
	assert.EqualValues(t, 0, out["chunks_indexed"])
	assert.Empty(t, m.rows())
	warnings, _ := res.Meta[stageWarningsMetaKey].([]any)
	require.Len(t, warnings, 1)
	assert.Equal(t, "describer:extraction_failed", warnings[0])
	assert.NotContains(t, warnings[0], "model host down", "endpoint text stays in the log")
}

func TestDescriberHourlyLimitIsResourceExhausted(t *testing.T) {
	one := 1
	m := newMediaEnv(t, mediaOpts{
		policy:    embeddings.Policy{MIMEAllowlist: []string{"image/png"}},
		describer: func(s *embeddings.DescriberSettings) { s.MaxRequestsPerHour = &one },
	})
	m.files["/ipfs/bafyOne"] = pngBytes
	m.files["/ipfs/bafyTwo"] = append(append([]byte{}, pngBytes...), 1)
	assert.EqualValues(t, 1, m.index(m.principal(), "bafyOne", false)["chunks_indexed"])

	res := m.call(m.principal(), "bafyTwo", false)
	require.True(t, res.IsError)
	te := toolErr(t, res)
	assert.Equal(t, CodeResourceExhausted, te.Code)
	assert.True(t, te.Retryable)
	assert.Len(t, m.chat.requests(), 1, "the second file sent nothing")
	assert.Len(t, m.rows(), 1, "the file is not half-indexed")
}

// --- 1.8 support: classification by name -------------------------------------------------

func TestClassifyContentUsesTheNameOnlyWhenOneIsGiven(t *testing.T) {
	tests := []struct {
		name, file string
		data       string
		mime, kind string
	}{
		{"python by name", "tool.py", "def f():\n    return 1\n", "text/x-python", "code"},
		{"yaml by name", "cfg.yaml", "a: 1\nb: 2\n", "application/yaml", "data"},
		{"toml by name", "cfg.toml", "[x]\na = 1\n", "application/toml", "data"},
		{"markdown by name", "notes.md", "# title\n\nbody text\n", "text/markdown", "text"},
		{"png by name when bytes have no signature", "pic.png", "not a real png", "image/png", "image"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mime, kind := classifyContent(tt.file, []byte(tt.data))
			assert.Equal(t, tt.mime, mime)
			assert.Equal(t, tt.kind, kind)

			// The same bytes through a CID-only call keep the name-less result.
			mime, kind = classifyContent("", []byte(tt.data))
			assert.Equal(t, "text/plain", mime, "no name: sniffed as plain text")
			assert.Equal(t, "text", kind)
		})
	}
}
