package server

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ipfs/kubo/mcp/server/embeddings"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// memRuns is an in-memory embeddings.RunStore: the bulk run state a store keeps.
type memRuns struct {
	runMu sync.Mutex
	run   *embeddings.Run
	saves int
}

func (m *memRuns) SaveRun(_ context.Context, r embeddings.Run) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	r.Sources = slices.Clone(r.Sources)
	r.SkippedByReason = maps.Clone(r.SkippedByReason)
	m.run = &r
	m.saves++
	return nil
}

func (m *memRuns) LoadRun(context.Context) (*embeddings.Run, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	if m.run == nil {
		return nil, nil
	}
	r := *m.run
	r.Sources = slices.Clone(r.Sources)
	r.SkippedByReason = maps.Clone(r.SkippedByReason)
	return &r, nil
}

// stored returns a copy of the stored run, or nil.
func (m *memRuns) stored(t testing.TB) *embeddings.Run {
	t.Helper()
	r, err := m.LoadRun(context.Background())
	require.NoError(t, err)
	return r
}

// chatFake is an OpenAI-compatible /chat/completions describer endpoint. It
// records the parts of every request and answers reply says.
type chatFake struct {
	*httptest.Server
	mu    sync.Mutex
	seen  []describeReq
	reply func(describeReq) (status int, text string)
}

type describeReq struct {
	Model     string
	MaxTokens int
	Auth      string
	Raw       string
	// Parts are the types of the user message's content parts, in order.
	Parts []string
	// URLs are the data URLs of the media parts.
	URLs []string
	// FPS and Resolution are those of a video part.
	FPS        float64
	Resolution string
}

func newChatFake(t *testing.T) *chatFake {
	t.Helper()
	c := &chatFake{reply: func(describeReq) (int, string) { return 200, "cats asleep on a sofa" }}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		req := describeReq{Raw: string(raw), Auth: r.Header.Get("Authorization")}
		var body struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Content []struct {
					Type     string                `json:"type"`
					ImageURL *struct{ URL string } `json:"image_url"`
					VideoURL *struct{ URL string } `json:"video_url"`
					FPS      float64               `json:"fps"`
					Res      string                `json:"media_resolution"`
				} `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(raw, &body) == nil {
			req.Model, req.MaxTokens = body.Model, body.MaxTokens
			for _, m := range body.Messages {
				for _, p := range m.Content {
					req.Parts = append(req.Parts, p.Type)
					if p.ImageURL != nil {
						req.URLs = append(req.URLs, p.ImageURL.URL)
					}
					if p.VideoURL != nil {
						req.URLs = append(req.URLs, p.VideoURL.URL)
						req.FPS, req.Resolution = p.FPS, p.Res
					}
				}
			}
		}
		c.mu.Lock()
		c.seen = append(c.seen, req)
		reply := c.reply
		c.mu.Unlock()
		status, text := reply(req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != 200 {
			_, _ = io.WriteString(w, `{"error":{"message":`+quoteJSON(text)+`}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":`+quoteJSON(text)+`}}]}`)
	}))
	t.Cleanup(c.Close)
	return c
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (c *chatFake) setReply(f func(describeReq) (int, string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reply = f
}

func (c *chatFake) requests() []describeReq {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.seen)
}

func (c *chatFake) host() string { return strings.TrimPrefix(c.URL, "http://") }

// toolErr decodes the ToolError of an isError result a client session got.
func toolErr(t testing.TB, res *mcp.CallToolResult) *ToolError {
	t.Helper()
	require.True(t, res.IsError, "want an error result, got %+v", res.StructuredContent)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	var out toolErrorResult
	require.NoError(t, json.Unmarshal(raw, &out))
	require.NotNil(t, out.Error)
	return out.Error
}
