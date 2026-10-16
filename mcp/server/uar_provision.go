//go:build !noagent

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Wire shapes of the UAR management API that kubo writes and reads. Every
// shape below is the one UAR v1.1.0 accepted in its own release acceptance
// (scripts/release/uar_1_1_acceptance.py at the tag, noted "acc." below) and
// is cross-checked against the Rust route handlers at the same tag. Paths are
// relative to the tag, repository universal-agent-runtime, commit d815ef72.
//
// What kubo owns on the instance. Kubo owns the agent whose id is
// llm.uar.agent_id: every provisioning replaces that agent's artifact
// (prompt, tool allowlist, skill preferences) with kubo's, and the operator
// edits it through kubo's settings, not on UAR. Kubo also owns the skills it
// uploaded: it binds them to its agent and unbinds one it bound earlier in this
// process when the skills directory no longer holds it. Skill bindings that
// kubo did not make are left in place. Two nodes that share a UAR instance need
// distinct llm.uar.agent_id values, or each overwrites the other's agent.
//
// What kubo does not do. It does not register its MCP endpoint on the
// instance: PUT /api/uar/mcp/servers/{name} needs the kubo URL as the instance
// reaches it and the trusted host id, which are deployment facts kubo does not
// know (ADR-0010 section 1 and the 2026-10-16 amendment). The operator
// registers it once, and before the first run: a run whose grant names an
// unregistered destination answers 422 run_mcp_destination_unregistered. The
// registration may name the agent in grant_policy.allowed_agent_ids before the
// agent exists (checked against the v1.1.0 image).
// It does not send X-UAR-Admin-Key: only /api/uar/settings* require it
// (src/uar/api/settings.rs:364), and kubo calls no settings route.
const (
	// uarAgentsPath: GET/POST /api/agents, GET/PUT /api/agents/{id}
	// (src/server.rs:2013-2017; handlers src/uar/api/discovery.rs:285,309,326).
	uarAgentsPath = "/api/agents"
	// uarSkillImportPath: POST, body {"skill_markdown": "<SKILL.md>"} whose
	// frontmatter has name, version and description (withUARSkillVersion), answers 201
	// with the skill record or 409 skill_id_conflict, never an overwrite
	// (src/uar/api/skills.rs:66 route, :452 handler; acc. 590-591).
	uarSkillImportPath = "/api/uar/skills/import-content"
	// uarSkillsPath: GET /api/uar/skills/{skill_id} answers the record with
	// skill_id and artifact_digest (src/uar/api/skills.rs:53, :132-148).
	uarSkillsPath = "/api/uar/skills"
	// uarAgentSkillsRoot: GET lists the bound ids as a JSON array of strings
	// (skills.rs:123, :746); POST /{agent}/skills/{id} binds one and answers 201
	// (:125, :770); DELETE /{agent}/skills/{id} unbinds and answers 204 (:126,
	// :783); PUT /{agent}/skills with {"skill_ids":[...]} replaces the whole set
	// (:124, :756; acc. 593-594), which kubo does not use because it would unbind
	// skills it does not own. Mounted at src/server.rs:1811.
	uarAgentSkillsRoot = "/api/uar/agents"
	// maxSkillBytes bounds one SKILL.md kubo uploads.
	maxSkillBytes = 256 << 10
	// maxSkillsUploaded bounds the skills one provisioning uploads.
	maxSkillsUploaded = 64
	// maxUARActiveSkills bounds policy.skills.max_active.
	maxUARActiveSkills = 3
)

func uarAgentPath(id string) string { return uarAgentsPath + "/" + url.PathEscape(id) }
func uarAgentSkillsPath(id string) string {
	return uarAgentSkillsRoot + "/" + url.PathEscape(id) + "/skills"
}

// uarSkill is one skill directory read from llm.uar.skills_dir.
type uarSkill struct {
	Name     string
	Markdown string
}

// loadUARSkills reads <dir>/<name>/SKILL.md for every subdirectory, sorted by
// name. The name is the directory name, which is the skill name (agentskills.io
// requires them to match). Only the SKILL.md is uploaded: the pinned contract
// accepts a single manifest and no supporting files. A directory without a
// SKILL.md is skipped. A SKILL.md that is a symlink or any other non-regular
// file is refused: the operator-named directory is the only thing kubo reads,
// and a link could name any file the process can read.
func loadUARSkills(dir string) ([]uarSkill, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read llm.uar.skills_dir: %w", err)
	}
	var out []uarSkill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "SKILL.md")
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", e.Name(), err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("skill %s: SKILL.md is not a regular file", e.Name())
		}
		if info.Size() > maxSkillBytes {
			return nil, fmt.Errorf("skill %s: SKILL.md is larger than %d bytes", e.Name(), maxSkillBytes)
		}
		data, err := readSkillFile(path)
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", e.Name(), err)
		}
		out = append(out, uarSkill{Name: e.Name(), Markdown: withUARSkillVersion(string(data))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > maxSkillsUploaded {
		return nil, fmt.Errorf("llm.uar.skills_dir holds %d skills; at most %d are uploaded", len(out), maxSkillsUploaded)
	}
	return out, nil
}

// uarSkillVersion is the version kubo gives a skill whose frontmatter has none.
const uarSkillVersion = "1.0.0"

// withUARSkillVersion adds the top-level "version" key UAR requires. UAR parses
// the frontmatter into a manifest whose name, version and description are all
// required (src/uar/domain/skills.rs:241-246 SkillManifest; parser
// src/uar/runtime/skills/storage/filesystem.rs:567-581) and answers 400
// skill_markdown_invalid "missing field `version`" otherwise, while an
// agentskills.io SKILL.md has no such key. The key goes right after the opening
// delimiter; every other byte is kept, and a document that already has a
// top-level version, or no frontmatter, is returned unchanged (UAR then
// rejects the latter itself).
func withUARSkillVersion(md string) string {
	rest, ok := strings.CutPrefix(md, "---")
	if !ok {
		return md
	}
	nl := "\n"
	if strings.HasPrefix(rest, "\r\n") {
		nl = "\r\n"
	}
	rest, ok = strings.CutPrefix(rest, nl)
	if !ok {
		return md
	}
	front, _, found := strings.Cut(rest, "---")
	if !found {
		return md
	}
	for line := range strings.SplitSeq(front, nl) {
		if strings.HasPrefix(line, "version:") {
			return md
		}
	}
	return "---" + nl + "version: " + uarSkillVersion + nl + rest
}

// readSkillFile reads at most maxSkillBytes+1 bytes of a file that was a
// regular file a moment ago; a file that grew meanwhile is refused.
func readSkillFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSkillBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSkillBytes {
		return nil, fmt.Errorf("SKILL.md is larger than %d bytes", maxSkillBytes)
	}
	return data, nil
}

// skillDigest is the artifact_digest UAR reports for a SKILL.md: "sha256:" and
// the hex digest of its bytes.
func skillDigest(markdown string) string {
	sum := sha256.Sum256([]byte(markdown))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// uarSkillConflictError is a same-name skill on the instance whose content is
// not the one kubo ships (or whose digest UAR does not report). Provisioning
// stops: binding it would run the agent with a skill kubo did not write.
type uarSkillConflictError struct{ Name string }

func (e *uarSkillConflictError) Error() string {
	return fmt.Sprintf("the UAR instance already holds a skill %q with different content", e.Name)
}

// uarProviderModel is the provider and model of the agent's default selection.
// UAR's artifact needs both (src/uar/domain/artifact.rs:227-230): the provider
// is llm.uar.provider, or the part of llm.model before the first slash.
func uarProviderModel(set *AgentSettings) (provider, model string, err error) {
	provider, model = set.UAR.Provider, set.Model
	if provider == "" {
		if p, m, ok := strings.Cut(model, "/"); ok && p != "" && m != "" {
			provider, model = p, m
		}
	}
	if provider == "" || model == "" {
		return "", "", errors.New("llm.model and llm.uar.provider (or llm.model as provider/model) are required: the UAR agent artifact needs both")
	}
	return provider, model, nil
}

// agentArtifact is the agent kubo writes, in the shape the acceptance posted
// (acc. 419-436 agent_artifact; struct src/uar/domain/artifact.rs:37-50). The
// system prompt is the one the openai backend would send: the fixed safety
// prefix, then the operator's custom part. tools lists the kubo MCP tools by
// their UAR names (<server>__<tool>, acc. 574-577), which makes them the only
// tools the agent may use; skillIDs are preferred skills.
func agentArtifact(set *AgentSettings, systemPrompt string, tools, skillIDs []string) (map[string]any, error) {
	u := set.UAR
	provider, model, err := uarProviderModel(set)
	if err != nil {
		return nil, err
	}
	if tools == nil {
		tools = []string{}
	}
	if skillIDs == nil {
		skillIDs = []string{}
	}
	return map[string]any{
		"version": "1.0.0",
		"kind":    "agent",
		"id":      u.AgentID,
		"metadata": map[string]any{
			"title":       u.AgentID,
			"description": "IPFS agent for a Kubo node, provisioned by ipfs mcp.",
			"tags":        []string{"ipfs", "kubo"},
		},
		"runtime": map[string]any{"entry": "agent", "protocols": map[string]any{}},
		"policy": map[string]any{
			"provider": map[string]any{
				"default":   map[string]any{"provider": provider, "model": model},
				"fallbacks": []any{},
			},
			"tools": map[string]any{
				"allow": tools, "deny": []string{}, "max_concurrent": 1, "execution_mode": "direct",
			},
			"skills": map[string]any{"prefer": skillIDs, "max_active": min(max(len(skillIDs), 1), maxUARActiveSkills)},
		},
		"schemas": map[string]any{"inputs": nil, "outputs": nil, "state": nil},
		"prompt":  map[string]any{"system": systemPrompt, "instructions": []string{}},
		"memory": map[string]any{
			"conversation": map[string]any{"enabled": false},
			"kb":           map[string]any{"enabled": false, "knowledge_bases": []string{}, "citation_required": false},
		},
		"tools":      map[string]any{"bundles": []any{}},
		"ui":         map[string]any{"forms": map[string]any{"enabled": false}, "artifacts": map[string]any{"enabled": false, "preferred_types": []string{}}},
		"extensions": map[string]any{},
	}, nil
}

// uarToolNames are the UAR names of the kubo tools the agent may call.
func (b *uarBackend) uarToolNames() []string {
	names := b.srv.ToolNames()
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, b.set.UAR.MCPName+"__"+n)
	}
	return out
}

// provisionUAR makes the UAR instance hold the kubo agent: the release check,
// the skills, the agent with kubo's prompt and tool allowlist, and the skill
// bindings. It is idempotent: a second call replaces the agent and binds what
// is missing. Skills already on the instance are never overwritten (the
// contract's upload does not overwrite): an identical copy is reused, a
// different one stops provisioning. The caller holds the provisioning slot.
func (b *uarBackend) provisionUAR(ctx context.Context) error {
	set := b.set
	u := set.UAR
	if _, err := b.cli.checkRelease(ctx, u.MinVersion); err != nil {
		return fmt.Errorf("release check: %w", err)
	}
	skills, err := loadUARSkills(u.SkillsDir)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(skills))
	for _, sk := range skills {
		id, err := b.uploadSkill(ctx, sk)
		if err != nil {
			return fmt.Errorf("skill %s: %w", sk.Name, err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	if err := b.putAgent(ctx, ids); err != nil {
		return fmt.Errorf("agent %s: %w", u.AgentID, err)
	}
	if err := b.bindSkills(ctx, ids); err != nil {
		return fmt.Errorf("agent %s skills: %w", u.AgentID, err)
	}
	return nil
}

// uarSkillRecord is the part of a skill record kubo reads.
type uarSkillRecord struct {
	SkillID        string `json:"skill_id"`
	ArtifactDigest string `json:"artifact_digest"`
}

// uploadSkill uploads one SKILL.md and returns the id to bind. A name that
// already exists is read back and reused only when UAR reports exactly the
// digest of the bytes kubo holds; a missing or different digest is a conflict
// and provisioning fails closed.
func (b *uarBackend) uploadSkill(ctx context.Context, sk uarSkill) (string, error) {
	r, err := b.cli.call(ctx, http.MethodPost, uarSkillImportPath, map[string]string{"skill_markdown": sk.Markdown}, nil)
	if err != nil {
		return "", err
	}
	switch {
	case r.Status == http.StatusCreated:
		var rec uarSkillRecord
		if json.Unmarshal(r.Body, &rec) != nil || rec.SkillID == "" {
			return "", errors.New("the UAR skill import reply has no skill_id")
		}
		return rec.SkillID, nil
	case r.Status != http.StatusConflict:
		return "", r.httpErr("skill import")
	}
	got, err := b.cli.call(ctx, http.MethodGet, uarSkillsPath+"/"+url.PathEscape(sk.Name), nil, nil)
	if err != nil {
		return "", err
	}
	if !got.ok() {
		return "", got.httpErr("skill read")
	}
	var rec uarSkillRecord
	if json.Unmarshal(got.Body, &rec) != nil || rec.SkillID == "" {
		return "", errors.New("the UAR skill reply has no skill_id")
	}
	if rec.ArtifactDigest == "" || rec.ArtifactDigest != skillDigest(sk.Markdown) {
		return "", &uarSkillConflictError{Name: sk.Name}
	}
	return rec.SkillID, nil
}

// putAgent creates the agent (POST answers 201) or replaces it in full (PUT
// answers 200). No If-Match is sent: the header carries a catalog sha256
// revision that GET does not return (discovery.rs:285-306, :380-395), and
// kubo owns this agent.
func (b *uarBackend) putAgent(ctx context.Context, skillIDs []string) error {
	art, err := agentArtifact(b.set, b.prompt, b.uarToolNames(), skillIDs)
	if err != nil {
		return err
	}
	path := uarAgentPath(b.set.UAR.AgentID)
	cur, err := b.cli.call(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	var r uarReply
	switch {
	case cur.Status == http.StatusNotFound:
		r, err = b.cli.call(ctx, http.MethodPost, uarAgentsPath, art, nil)
	case cur.ok():
		r, err = b.cli.call(ctx, http.MethodPut, path, art, nil)
	default:
		return cur.httpErr("agent read")
	}
	if err != nil {
		return err
	}
	if !r.ok() {
		return r.httpErr("agent write")
	}
	return nil
}

// bindSkills binds the given skill ids to the agent one by one, and unbinds
// the skills kubo bound earlier in this process that it no longer ships. A
// binding kubo did not make is never touched.
func (b *uarBackend) bindSkills(ctx context.Context, want []string) error {
	path := uarAgentSkillsPath(b.set.UAR.AgentID)
	cur, err := b.cli.call(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	if !cur.ok() {
		return cur.httpErr("skill bindings read")
	}
	var bound []string
	if err := json.Unmarshal(cur.Body, &bound); err != nil {
		return errors.New("the UAR skill bindings reply is not a list of skill ids")
	}
	for _, id := range want {
		if slices.Contains(bound, id) {
			continue
		}
		r, err := b.cli.call(ctx, http.MethodPost, path+"/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		if !r.ok() && r.Status != http.StatusConflict {
			return r.httpErr("skill bind")
		}
	}
	for _, id := range b.ownedSkills {
		if slices.Contains(want, id) || !slices.Contains(bound, id) {
			continue
		}
		r, err := b.cli.call(ctx, http.MethodDelete, path+"/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		if !r.ok() && r.Status != http.StatusNotFound {
			return r.httpErr("skill unbind")
		}
	}
	b.ownedSkills = slices.Clone(want)
	return nil
}

// trimSnippet shortens s for a log line or a relayed message.
func trimSnippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
