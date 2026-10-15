package embeddings

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// describerFake is an OpenAI-compatible chat endpoint that records each request
// and answers with a canned description.
type describerFake struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	auth   []string
	paths  []string
	status int
	reply  string
}

func newDescriberFake(t *testing.T) *describerFake {
	t.Helper()
	f := &describerFake{status: http.StatusOK, reply: `{"choices":[{"message":{"role":"assistant","content":"a cat asleep on a red sofa"}}]}`}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, string(b))
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.paths = append(f.paths, r.URL.Path)
		status, reply := f.status, f.reply
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *describerFake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

func (f *describerFake) set(status int, reply string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status, f.reply = status, reply
}

func (f *describerFake) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[len(f.bodies)-1]
}

func (f *describerFake) host() string { return strings.TrimPrefix(f.URL, "http://") }

func covering(host string) func(string) bool { return func(h string) bool { return h == host } }

func newTestDescriber(t *testing.T, f *describerFake, mut func(*DescriberSettings)) *Describer {
	t.Helper()
	s := DescriberSettings{BaseURL: f.URL + "/v1", Model: "vision-test", APIKey: "sekrit"}
	if mut != nil {
		mut(&s)
	}
	d, err := NewDescriber(s, nil)
	require.NoError(t, err)
	return d
}

var png = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)

type chatBody struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string `json:"role"`
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL *struct {
				URL string `json:"url"`
			} `json:"image_url"`
			VideoURL *struct {
				URL string `json:"url"`
			} `json:"video_url"`
			FPS             *float64 `json:"fps"`
			MediaResolution string   `json:"media_resolution"`
		} `json:"content"`
	} `json:"messages"`
}

func decodeChat(t *testing.T, raw string) chatBody {
	t.Helper()
	var b chatBody
	require.NoError(t, json.Unmarshal([]byte(raw), &b), raw)
	return b
}

func TestDescribeImageRequestShape(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, func(s *DescriberSettings) { s.MaxOutputTokens = 77 })

	got, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	require.NoError(t, err)
	assert.Equal(t, "a cat asleep on a red sofa", got.Text)
	assert.Equal(t, "vision-test", got.Model, "described_by is the configured model")

	require.Equal(t, 1, f.count())
	assert.Equal(t, "/v1/chat/completions", f.paths[0])
	assert.Equal(t, "Bearer sekrit", f.auth[0])
	b := decodeChat(t, f.last())
	assert.Equal(t, "vision-test", b.Model)
	assert.Equal(t, 77, b.MaxTokens)
	require.Len(t, b.Messages, 1)
	parts := b.Messages[0].Content
	require.Len(t, parts, 2)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, DescriberInstruction(KindImage), parts[0].Text, "the instruction is fixed")
	require.NotNil(t, parts[1].ImageURL)
	url := parts[1].ImageURL.URL
	prefix := "data:image/png;base64,"
	require.True(t, strings.HasPrefix(url, prefix), url)
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, prefix))
	require.NoError(t, err)
	assert.Equal(t, png, decoded, "the bytes travel inside the request")
	assert.NotContains(t, f.last(), "http://", "no node or gateway URL appears anywhere in the body")
	assert.NotContains(t, f.last(), "https://")
	assert.NotContains(t, f.last(), "/ipfs/")
}

func TestDescribeVideoPartCarriesFPSAndResolution(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, func(s *DescriberSettings) {
		s.Kinds = []string{KindImage, KindVideo}
		s.VideoFPS = 2.5
	})
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindVideo, MIME: "video/mp4", Data: []byte("\x00\x00\x00\x18ftypmp42")})
	require.NoError(t, err)

	part := decodeChat(t, f.last()).Messages[0].Content[1]
	assert.Equal(t, "video_url", part.Type)
	require.NotNil(t, part.VideoURL)
	assert.True(t, strings.HasPrefix(part.VideoURL.URL, "data:video/mp4;base64,"))
	require.NotNil(t, part.FPS)
	assert.InDelta(t, 2.5, *part.FPS, 0.0001)
	assert.Equal(t, "default", part.MediaResolution)
	assert.Equal(t, DescriberInstruction(KindVideo), decodeChat(t, f.last()).Messages[0].Content[0].Text)
}

func TestDescribeVideoNeedsTheKindListed(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, nil) // kinds default to ["image"]
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindVideo, MIME: "video/mp4", Data: []byte("v")})
	assert.ErrorIs(t, err, ErrDescriberDisabled)
	assert.Zero(t, f.count(), "no video is sent when kinds is [image]")
}

func TestDescribeByteCapSendsNothing(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, func(s *DescriberSettings) {
		s.Kinds = []string{KindImage, KindVideo}
		s.ImageMaxBytes, s.VideoMaxBytes = 16, 32
	})
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: make([]byte, 17)})
	assert.ErrorIs(t, err, ErrDescribeTooLarge)
	_, err = d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindVideo, MIME: "video/mp4", Data: make([]byte, 33)})
	assert.ErrorIs(t, err, ErrDescribeTooLarge)
	assert.Zero(t, f.count(), "an oversize file builds no request")
	_, err = d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: make([]byte, 16)})
	assert.NoError(t, err, "the limit itself is allowed")
}

func TestDescribeHourlyLimitIsSharedAndSlides(t *testing.T) {
	f := newDescriberFake(t)
	limit := 3
	d := newTestDescriber(t, f, func(s *DescriberSettings) { s.MaxRequestsPerHour = &limit })
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return now }

	var wg sync.WaitGroup
	var okCount, limited int
	var mu sync.Mutex
	for range 6 { // six callers share the limit of three
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
			mu.Lock()
			defer mu.Unlock()
			switch err {
			case nil:
				okCount++
			default:
				if assert.ErrorIs(t, err, ErrDescribeRateLimited) {
					limited++
				}
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 3, okCount)
	assert.Equal(t, 3, limited)
	assert.Equal(t, 3, f.count(), "a refused call sends nothing")
	assert.Equal(t, time.Hour, d.RetryAfter())

	now = now.Add(61 * time.Minute)
	assert.Zero(t, d.RetryAfter(), "an hour later the window is empty")
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	assert.NoError(t, err)
}

func TestDescribeZeroLimitDisablesTheDescriber(t *testing.T) {
	f := newDescriberFake(t)
	zero := 0
	d := newTestDescriber(t, f, func(s *DescriberSettings) { s.MaxRequestsPerHour = &zero })
	assert.False(t, d.Enabled())
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	assert.ErrorIs(t, err, ErrDescriberDisabled)
	assert.Zero(t, f.count())
}

func TestDescribeEmptyKindsDescribesNothing(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, func(s *DescriberSettings) { s.Kinds = []string{} })
	assert.False(t, d.Enabled())
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	assert.ErrorIs(t, err, ErrDescriberDisabled)
	assert.Zero(t, f.count())
}

func TestDescribeRefusesAnUnacknowledgedHostBeforeSending(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, nil)
	for name, covers := range map[string]func(string) bool{"nil": nil, "other host": covering("elsewhere.example")} {
		t.Run(name, func(t *testing.T) {
			_, err := d.Describe(t.Context(), covers, DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
			var eg *EgressError
			require.ErrorAs(t, err, &eg)
			assert.Equal(t, []string{f.host()}, eg.Hosts)
			assert.Equal(t, "embeddings.describer.base_url", eg.Setting)
			assert.ErrorIs(t, err, ErrEgressNotAcknowledged)
		})
	}
	assert.Zero(t, f.count())
	assert.Zero(t, len(d.sent), "a refused call does not spend the hourly limit")
}

func TestDescribeResponseIsValidatedAsUntrusted(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"not json", 200, `<html>`, ErrDescribeFailed},
		{"no choices", 200, `{"choices":[]}`, ErrDescribeFailed},
		{"content is not text", 200, `{"choices":[{"message":{"content":[{"type":"text","text":"x"}]}}]}`, ErrDescribeFailed},
		{"empty text", 200, `{"choices":[{"message":{"content":"   "}}]}`, ErrDescribeFailed},
		{"NUL byte", 200, `{"choices":[{"message":{"content":"a\u0000b"}}]}`, ErrDescribeFailed},
		{"too long", 200, `{"choices":[{"message":{"content":"` + strings.Repeat("x", maxDescriptionBytes+1) + `"}}]}`, ErrDescribeFailed},
		{"bad request", 400, `{"error":{"message":"video_url is not supported\nsecond line"}}`, ErrDescribeFailed},
		{"gateway", 502, `{"error":{"message":"upstream down"}}`, ErrDescriberUnavailable},
		{"rate limited upstream", 429, `{}`, ErrDescriberUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDescriberFake(t)
			f.set(tt.status, tt.body)
			d := newTestDescriber(t, f, func(s *DescriberSettings) { s.Kinds = []string{KindImage, KindVideo} })
			_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindVideo, MIME: "video/mp4", Data: []byte("v")})
			require.ErrorIs(t, err, tt.want)
			assert.NotContains(t, err.Error(), "\n", "an endpoint message is cleaned before it reaches an error")
		})
	}
}

func TestDescribeRejectedVideoPartNamesTheLikelyCause(t *testing.T) {
	f := newDescriberFake(t)
	f.set(400, `{"error":{"message":"unknown content part"}}`)
	d := newTestDescriber(t, f, func(s *DescriberSettings) { s.Kinds = []string{KindVideo} })
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindVideo, MIME: "video/mp4", Data: []byte("v")})
	require.ErrorIs(t, err, ErrDescribeFailed)
	assert.Contains(t, err.Error(), "video_url")
}

func TestDescribeDoesNotFollowRedirects(t *testing.T) {
	target := newDescriberFake(t)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/chat/completions", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	d, err := NewDescriber(DescriberSettings{BaseURL: redirect.URL + "/v1", Model: "m"}, nil)
	require.NoError(t, err)
	_, err = d.Describe(t.Context(), func(string) bool { return true }, DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	require.ErrorIs(t, err, ErrDescribeFailed)
	assert.Zero(t, target.count(), "the media never reached a host the operator did not name")
}

func TestDescribeCancelledContext(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	t.Cleanup(func() { close(block); srv.Close() })
	d, err := NewDescriber(DescriberSettings{BaseURL: srv.URL + "/v1", Model: "m"}, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = d.Describe(ctx, func(string) bool { return true }, DescribeRequest{Kind: KindImage, MIME: "image/png", Data: png})
	require.ErrorIs(t, err, ErrDescriberUnavailable)
}

func TestDescribeRejectsAnUnsafeMediaType(t *testing.T) {
	f := newDescriberFake(t)
	d := newTestDescriber(t, f, nil)
	_, err := d.Describe(t.Context(), covering(f.host()), DescribeRequest{Kind: KindImage, MIME: "image/png\"}]}", Data: png})
	require.ErrorIs(t, err, ErrDescribeFailed)
	assert.Zero(t, f.count(), "a media type is never spliced into the request unchecked")
}

func TestDescriberSettingsValidate(t *testing.T) {
	neg := -1
	tests := []struct {
		name string
		mut  func(*DescriberSettings)
		want string
	}{
		{"valid", func(*DescriberSettings) {}, ""},
		{"no base url", func(s *DescriberSettings) { s.BaseURL = "" }, "embeddings.describer.base_url"},
		{"no model", func(s *DescriberSettings) { s.Model = " " }, "embeddings.describer.model"},
		{"plain http to a remote host", func(s *DescriberSettings) { s.BaseURL = "http://describer.example/v1" }, "plain http"},
		{"bad kind", func(s *DescriberSettings) { s.Kinds = []string{"audio"} }, "kinds entry"},
		{"tokens", func(s *DescriberSettings) { s.MaxOutputTokens = MaxDescriberMaxOutputTokens + 1 }, "max_output_tokens"},
		{"image bytes", func(s *DescriberSettings) { s.ImageMaxBytes = MaxDescriberMediaBytes + 1 }, "image_max_bytes"},
		{"fps low", func(s *DescriberSettings) { s.VideoFPS = 0.05 }, "video_fps"},
		{"fps high", func(s *DescriberSettings) { s.VideoFPS = 11 }, "video_fps"},
		{"negative limit", func(s *DescriberSettings) { s.MaxRequestsPerHour = &neg }, "max_requests_per_hour"},
		{"timeout", func(s *DescriberSettings) { s.Timeout = 2 * time.Hour }, "timeout_seconds"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DescriberSettings{BaseURL: "http://127.0.0.1:11434/v1", Model: "llava"}
			tt.mut(&s)
			err := s.Validate()
			if tt.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestDescriberDefaults(t *testing.T) {
	d, err := NewDescriber(DescriberSettings{BaseURL: "https://api.example.test/v1/", Model: "m"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.test/v1/chat/completions", d.endpoint)
	assert.Equal(t, DefaultDescriberMaxOutputTokens, d.maxTok)
	assert.Equal(t, DefaultDescriberImageMaxBytes, d.MaxBytes(KindImage))
	assert.Equal(t, DefaultDescriberVideoMaxBytes, d.MaxBytes(KindVideo))
	assert.Equal(t, DefaultDescriberMaxRequestsPerHour, d.limit)
	assert.Equal(t, DefaultDescriberTimeout, d.timeout)
	assert.True(t, d.Describes(KindImage))
	assert.False(t, d.Describes(KindVideo), "video is off unless listed")
}

func TestServiceStageHostsIncludeEveryRemoteStage(t *testing.T) {
	f := newDescriberFake(t)
	svc := NewService(Settings{Mode: ModeRemote, Remote: &RemoteSettings{Endpoint: "https://store.example.test:8000"}},
		nil, mustEmbedder(t, "https://embed.example.test/v1"))
	assert.ElementsMatch(t, []string{"embed.example.test", "store.example.test:8000"}, svc.StageHosts())

	svc.WithDescriber(newTestDescriber(t, f, nil))
	assert.Contains(t, svc.StageHosts(), f.host())
	err := svc.CheckStageEgress(covering("embed.example.test"))
	var eg *EgressError
	require.ErrorAs(t, err, &eg)
	assert.ElementsMatch(t, []string{"store.example.test:8000", f.host()}, eg.Hosts)

	zero := 0
	off := newTestDescriber(t, f, func(s *DescriberSettings) { s.MaxRequestsPerHour = &zero })
	svc.WithDescriber(off)
	assert.NotContains(t, svc.StageHosts(), f.host(), "a disabled describer contacts nothing, so it needs no acknowledgement")
}

func mustEmbedder(t *testing.T, base string) *Embedder {
	t.Helper()
	e, err := NewEmbedder(EmbedderSettings{BaseURL: base, Model: "m", Dimensions: 4}, nil)
	require.NoError(t, err)
	return e
}
