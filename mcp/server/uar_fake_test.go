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
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// uarReleaseFixtureDir holds the recorded-shape responses of the UAR v1.1.0 contract
// (mcp/uar/v1.1.0-kubo.md). The AG-UI streams and interrupts follow its
// "Complete AG-UI interrupt contract" field for field; the management replies
// follow the route handlers and the release acceptance program at the tag
// (mcp/uar/v1.1.0-image-config.md).
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
	skills   map[string]string
	bindings map[string][]string
	runs     []uarRun
	calls    []string
	// noDigest and digestOverride make the skill record report no
	// artifact_digest, or a different one, for a name.
	noDigest       map[string]bool
	digestOverride map[string]string
	// script chooses the answer of run number n (0-based).
	script func(n int, body map[string]any) (status int, stream []byte)
}

func newFakeUARRelease(t *testing.T, script func(n int, body map[string]any) (int, []byte)) *fakeUARRelease {
	t.Helper()
	u := &fakeUARRelease{
		version: "1.1.0", agents: map[string]map[string]any{}, skills: map[string]string{},
		bindings: map[string][]string{}, script: script,
		noDigest: map[string]bool{}, digestOverride: map[string]string{},
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

var skillVersionLine = regexp.MustCompile(`(?m)^version:\s*\S+`)
var skillNameLine = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)

func writeJSONReply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// agentArtifactErrors lists what UAR's serde parse of an AgentArtifact
// (src/uar/domain/artifact.rs:37-50, :213-354 at v1.1.0) would reject: a missing
// required field. The fake answers 422 for it, as the axum Json extractor does.
func agentArtifactErrors(art map[string]any) []string {
	var errs []string
	need := func(path ...string) {
		var cur any = art
		for _, k := range path {
			m, _ := cur.(map[string]any)
			v, ok := m[k]
			if !ok {
				errs = append(errs, strings.Join(path, "."))
				return
			}
			cur = v
		}
	}
	for _, p := range [][]string{
		{"version"}, {"kind"}, {"id"}, {"metadata", "title"}, {"metadata", "description"}, {"runtime", "entry"},
		{"policy", "provider", "default", "provider"}, {"policy", "provider", "default", "model"},
		{"policy", "tools"}, {"policy", "skills"}, {"schemas"}, {"prompt", "system"}, {"memory", "conversation", "enabled"},
		{"memory", "kb", "enabled"}, {"tools", "bundles"}, {"ui", "forms", "enabled"}, {"ui", "artifacts", "enabled"},
	} {
		need(p...)
	}
	return errs
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
		if errs := agentArtifactErrors(art); len(errs) > 0 {
			http.Error(w, "missing "+strings.Join(errs, ","), http.StatusUnprocessableEntity)
			return
		}
		id, _ := art["id"].(string)
		if _, dup := u.agents[id]; dup {
			writeJSONReply(w, http.StatusConflict, map[string]any{"error": "exists"})
			return
		}
		u.agents[id] = art
		writeJSONReply(w, http.StatusCreated, art)
	case strings.HasPrefix(path, "/api/agents/") && !strings.Contains(strings.TrimPrefix(path, "/api/agents/"), "/"):
		id := strings.TrimPrefix(path, "/api/agents/")
		art, ok := u.agents[id]
		switch {
		case !ok:
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "not found"})
		case r.Method == http.MethodGet:
			writeJSONReply(w, http.StatusOK, art)
		case r.Method == http.MethodPut:
			// Discovery.rs:326: an If-Match must be a catalog sha256 revision.
			if m := r.Header.Get("If-Match"); m != "" && !strings.HasPrefix(strings.Trim(m, `"`), "sha256:") {
				http.Error(w, "If-Match must contain a catalog SHA-256 revision", http.StatusBadRequest)
				return
			}
			var next map[string]any
			_ = json.Unmarshal(body, &next)
			if errs := agentArtifactErrors(next); len(errs) > 0 {
				http.Error(w, "missing "+strings.Join(errs, ","), http.StatusUnprocessableEntity)
				return
			}
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
			writeJSONReply(w, http.StatusBadRequest, map[string]any{"error": "skill_markdown_invalid"})
			return
		}
		// UAR parses name, version and description as required keys
		// (src/uar/domain/skills.rs:241-246) and answers 400 with serde's text.
		if !skillVersionLine.MatchString(in.SkillMarkdown) {
			writeJSONReply(w, http.StatusBadRequest, map[string]any{"error": "skill_markdown_invalid", "message": "missing field `version` at line 2 column 1"})
			return
		}
		if _, dup := u.skills[m[1]]; dup {
			writeJSONReply(w, http.StatusConflict, map[string]any{"error": "skill_id_conflict"})
			return
		}
		u.skills[m[1]] = in.SkillMarkdown
		writeJSONReply(w, http.StatusCreated, u.skillRecord(m[1]))
	case strings.HasPrefix(path, uarSkillsPath+"/") && r.Method == http.MethodGet:
		name := strings.TrimPrefix(path, uarSkillsPath+"/")
		if _, ok := u.skills[name]; !ok {
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSONReply(w, http.StatusOK, u.skillRecord(name))
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
			if len(stream) > 0 { // a scripted raw error body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write(stream)
				return
			}
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

// skillRecord is the reply of the skill routes: SkillResponse at
// src/uar/api/skills.rs:132-148, the fields kubo reads. UAR derives skill_id
// from the manifest name; for a lowercase-hyphen name it is the name.
func (u *fakeUARRelease) skillRecord(name string) map[string]any {
	md := u.skills[name]
	rec := map[string]any{"skill_id": name, "title": name, "skill_markdown": md}
	if !u.noDigest[name] {
		rec["artifact_digest"] = skillDigest(md)
	}
	if d, ok := u.digestOverride[name]; ok {
		rec["artifact_digest"] = d
	}
	return rec
}

// serveBindings is the agent-skills router (src/uar/api/skills.rs:123-126):
// GET answers a JSON array of ids, PUT replaces them from {"skill_ids"}, POST
// /{id} binds one (201) and DELETE /{id} unbinds it (204).
func (u *fakeUARRelease) serveBindings(w http.ResponseWriter, r *http.Request, path string, body []byte) {
	rest := strings.TrimPrefix(path, "/api/uar/agents/")
	agent, after, _ := strings.Cut(rest, "/skills")
	sid := strings.TrimPrefix(after, "/")
	if _, ok := u.agents[agent]; !ok {
		writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "agent not found"})
		return
	}
	switch {
	case r.Method == http.MethodGet && sid == "":
		list := u.bindings[agent]
		if list == nil {
			list = []string{}
		}
		writeJSONReply(w, http.StatusOK, list)
	case r.Method == http.MethodPut && sid == "":
		var in struct {
			SkillIDs []string `json:"skill_ids"`
		}
		_ = json.Unmarshal(body, &in)
		u.bindings[agent] = in.SkillIDs
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && sid != "":
		if _, ok := u.skills[sid]; !ok {
			writeJSONReply(w, http.StatusNotFound, map[string]any{"error": "skill not found"})
			return
		}
		if !slices.Contains(u.bindings[agent], sid) {
			u.bindings[agent] = append(u.bindings[agent], sid)
		}
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodDelete && sid != "":
		u.bindings[agent] = slices.DeleteFunc(u.bindings[agent], func(id string) bool { return id == sid })
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
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
