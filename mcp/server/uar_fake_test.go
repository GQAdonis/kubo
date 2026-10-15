//go:build !noagent

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// uarReleaseFixtureDir holds the recorded-shape responses of the UAR v1.1.0 contract
// (mcp/uar/v1.1.0-kubo.md). The AG-UI streams and interrupts follow its
// "Complete AG-UI interrupt contract" field for field; the management replies
// follow the paths it names.
var uarReleaseFixtureDir = filepath.Join("..", "uar", "fixtures", "v1.1.0")

func uarReleaseFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(uarReleaseFixtureDir, name))
	require.NoError(t, err)
	return b
}

const fakeUARToken = "uar-issuer-jwt-0123456789abcdef"

// uarRun is one recorded POST to the AG-UI agent path.
type uarRun struct {
	AgentID string
	Auth    string
	Body    map[string]any
}

// fakeUARRelease is an httptest stand-in for a UAR v1.1.0 instance: the agent
// card, agent CRUD, single-manifest skill upload, skill bindings and the AG-UI
// agent path, which answers each run with the fixture the script names.
type fakeUARRelease struct {
	srv *httptest.Server

	mu       sync.Mutex
	version  string
	agents   map[string]map[string]any
	etag     int
	skills   map[string]string
	bindings map[string][]string
	runs     []uarRun
	calls    []string
	// script chooses the answer of run number n (0-based).
	script func(n int, body map[string]any) (status int, stream []byte)
}

func newFakeUARRelease(t *testing.T, script func(n int, body map[string]any) (int, []byte)) *fakeUARRelease {
	t.Helper()
	u := &fakeUARRelease{
		version: "1.1.0", agents: map[string]map[string]any{}, skills: map[string]string{},
		bindings: map[string][]string{}, script: script,
	}
	u.srv = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *fakeUARRelease) edit(fn func(*fakeUARRelease)) {
	u.mu.Lock()
	defer u.mu.Unlock()
	fn(u)
}

func (u *fakeUARRelease) runCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.runs)
}

func (u *fakeUARRelease) run(n int) uarRun {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.runs[n]
}

func (u *fakeUARRelease) callLog() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.calls...)
}

var skillNameLine = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)

func writeJSONReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (u *fakeUARRelease) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls = append(u.calls, r.Method+" "+r.URL.Path)
	if r.URL.Path == uarCardPath {
		writeJSONReply(w, http.StatusOK, map[string]any{"name": "fake uar", "version": u.version})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+fakeUARToken {
		writeJSONReply(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	path := r.URL.Path
	switch {
	case path == "/api/agents" && r.Method == http.MethodPost:
		var art map[string]any
		_ = json.Unmarshal(body, &art)
		id, _ := art["id"].(string)
		if _, dup := u.agents[id]; dup {
			writeJSONReply(w, http.StatusConflict, map[string]any{"error": "exists"})
			return
		}
		u.etag++
		u.agents[id] = art
		writeJSONReply(w, http.StatusCreated, art)
	case strings.HasPrefix(path, "/api/agents/") && !strings.Contains(strings.TrimPrefix(path, "/api/agents/"), "/"):
		id := strings.TrimPrefix(path, "/api/agents/")
		art, ok := u.agents[id]
		switch {
		case !ok:
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "not found"})
		case r.Method == http.MethodGet:
			w.Header().Set("ETag", `"`+strconv.Itoa(u.etag)+`"`)
			writeJSONReply(w, http.StatusOK, art)
		case r.Method == http.MethodPut:
			if r.Header.Get("If-Match") != `"`+strconv.Itoa(u.etag)+`"` {
				writeJSONReply(w, http.StatusPreconditionFailed, map[string]any{"error": "stale"})
				return
			}
			var next map[string]any
			_ = json.Unmarshal(body, &next)
			u.etag++
			u.agents[id] = next
			writeJSONReply(w, http.StatusOK, next)
		}
	case path == uarSkillImportPath && r.Method == http.MethodPost:
		var in struct {
			SkillMarkdown string `json:"skill_markdown"`
		}
		_ = json.Unmarshal(body, &in)
		m := skillNameLine.FindStringSubmatch(in.SkillMarkdown)
		if m == nil {
			writeJSONReply(w, http.StatusBadRequest, map[string]any{"error": "no name"})
			return
		}
		if _, dup := u.skills[m[1]]; dup {
			writeJSONReply(w, http.StatusConflict, map[string]any{"error": "conflict"})
			return
		}
		u.skills[m[1]] = in.SkillMarkdown
		writeJSONReply(w, http.StatusCreated, map[string]any{"skill_id": "sk-" + m[1], "name": m[1], "artifact_digest": skillDigest(in.SkillMarkdown)})
	case strings.HasPrefix(path, uarSkillsPath+"/") && r.Method == http.MethodGet:
		name := strings.TrimPrefix(path, uarSkillsPath+"/")
		md, ok := u.skills[name]
		if !ok {
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSONReply(w, http.StatusOK, map[string]any{"skill_id": "sk-" + name, "name": name, "skill_markdown": md, "artifact_digest": skillDigest(md)})
	case strings.HasPrefix(path, "/api/uar/agents/") && strings.Contains(path, "/skills"):
		u.serveBindings(w, r, path, body)
	case strings.HasPrefix(path, "/api/uar/ag-ui/agents/") && r.Method == http.MethodPost:
		id := strings.TrimPrefix(path, "/api/uar/ag-ui/agents/")
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		n := len(u.runs)
		u.runs = append(u.runs, uarRun{AgentID: id, Auth: r.Header.Get("Authorization"), Body: in})
		if _, ok := u.agents[id]; !ok {
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "no such agent"})
			return
		}
		status, stream := u.script(n, in)
		if status != http.StatusOK {
			writeJSONReply(w, status, map[string]any{"error": "scripted"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(stream)
	default:
		http.NotFound(w, r)
	}
}

func (u *fakeUARRelease) serveBindings(w http.ResponseWriter, r *http.Request, path string, body []byte) {
	rest := strings.TrimPrefix(path, "/api/uar/agents/")
	agent, after, _ := strings.Cut(rest, "/skills")
	switch r.Method {
	case http.MethodGet:
		list := make([]map[string]string, 0, len(u.bindings[agent]))
		for _, id := range u.bindings[agent] {
			list = append(list, map[string]string{"skill_id": id})
		}
		writeJSONReply(w, http.StatusOK, list)
	case http.MethodPost:
		var in struct {
			SkillID string `json:"skill_id"`
		}
		_ = json.Unmarshal(body, &in)
		for _, id := range u.bindings[agent] {
			if id == in.SkillID {
				writeJSONReply(w, http.StatusConflict, map[string]any{"error": "bound"})
				return
			}
		}
		u.bindings[agent] = append(u.bindings[agent], in.SkillID)
		writeJSONReply(w, http.StatusCreated, map[string]any{"skill_id": in.SkillID})
	case http.MethodDelete:
		sid := strings.TrimPrefix(after, "/")
		kept := u.bindings[agent][:0]
		for _, id := range u.bindings[agent] {
			if id != sid {
				kept = append(kept, id)
			}
		}
		u.bindings[agent] = kept
		w.WriteHeader(http.StatusNoContent)
	}
}

// replay answers every run with the fixtures named, in order; the last one
// repeats.
func replay(t *testing.T, names ...string) func(int, map[string]any) (int, []byte) {
	t.Helper()
	streams := make([][]byte, len(names))
	for i, n := range names {
		streams[i] = uarReleaseFixture(t, n)
	}
	return func(n int, _ map[string]any) (int, []byte) {
		return http.StatusOK, streams[min(n, len(streams)-1)]
	}
}
