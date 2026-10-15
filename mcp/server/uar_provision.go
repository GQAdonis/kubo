//go:build !noagent

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Wire shapes of the UAR management API that kubo writes and reads.
//
// Stated by the pinned contract (mcp/uar/v1.1.0-kubo.md): the paths
// POST /api/agents, POST /api/uar/skills/import-content (body: one
// skill_markdown string; a name collision answers conflict and never
// overwrites), GET /api/uar/skills/{skill_id} (includes skill_markdown and
// artifact_digest) and /api/uar/agents/{agent_id}/skills.
//
// Not stated by it, and so read tolerantly: the agent artifact's field names
// beyond the ones recorded in the 2026-10-13 audit (prompt.system,
// policy.provider.default{provider,model}), the skill record's id field, and
// the bind and unbind bodies. The real-service run (test/cli, agent-backend-uar
// 1.5) is the first check of these against the published image.
const (
	uarAgentsPath      = "/api/agents"
	uarSkillImportPath = "/api/uar/skills/import-content"
	uarSkillsPath      = "/api/uar/skills"
	// maxSkillBytes bounds one SKILL.md kubo uploads.
	maxSkillBytes = 256 << 10
	// maxSkillsUploaded bounds the skills one provisioning uploads.
	maxSkillsUploaded = 64
)

func uarAgentPath(id string) string       { return uarAgentsPath + "/" + url.PathEscape(id) }
func uarAgentSkillsPath(id string) string { return "/api/uar/agents/" + url.PathEscape(id) + "/skills" }

// uarSkill is one skill directory read from llm.uar.skills_dir.
type uarSkill struct {
	Name     string
	Markdown string
}

// loadUARSkills reads <dir>/<name>/SKILL.md for every subdirectory, sorted by
// name. The name is the directory name, which is the skill name (agentskills.io
// requires them to match). Only the SKILL.md is uploaded: the pinned contract
// accepts a single manifest and no supporting files. A directory without a
// SKILL.md is skipped.
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
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), "SKILL.md"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read skill %s: %w", e.Name(), err)
		}
		if len(data) > maxSkillBytes {
			return nil, fmt.Errorf("skill %s: SKILL.md is larger than %d bytes", e.Name(), maxSkillBytes)
		}
		out = append(out, uarSkill{Name: e.Name(), Markdown: string(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > maxSkillsUploaded {
		return nil, fmt.Errorf("llm.uar.skills_dir holds %d skills; at most %d are uploaded", len(out), maxSkillsUploaded)
	}
	return out, nil
}

// skillDigest is the artifact_digest UAR reports for a SKILL.md: "sha256:" and
// the hex digest of its bytes.
func skillDigest(markdown string) string {
	sum := sha256.Sum256([]byte(markdown))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// agentArtifact is the agent kubo writes. The system prompt is the one the
// openai backend would send: the fixed safety prefix, then the operator's
// custom part. The MCP binding names the UAR-side destination of this server.
func agentArtifact(set *AgentSettings, systemPrompt string) map[string]any {
	u := set.UAR
	art := map[string]any{
		"id":          u.AgentID,
		"name":        u.AgentID,
		"description": "IPFS agent for a Kubo node, provisioned by ipfs mcp.",
		"prompt":      map[string]any{"system": systemPrompt},
	}
	if set.Model != "" {
		def := map[string]any{"model": set.Model}
		if u.Provider != "" {
			def["provider"] = u.Provider
		}
		art["policy"] = map[string]any{"provider": map[string]any{"default": def}}
	}
	if u.MCPCredential != nil {
		art["extensions"] = map[string]any{"mcp_servers": []string{u.MCPName}}
	}
	return art
}

// provisionUAR makes the UAR instance hold the kubo agent: the release check,
// the skills, the agent with kubo's prompt and MCP binding, and the skill
// bindings. It is idempotent: a second call updates what exists. Skills
// already on the instance are never overwritten (the contract's upload does
// not overwrite); a changed SKILL.md needs a new name or the UAR administrator.
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
	ids := make(map[string]string, len(skills))
	for _, sk := range skills {
		id, err := b.uploadSkill(ctx, sk)
		if err != nil {
			return fmt.Errorf("skill %s: %w", sk.Name, err)
		}
		ids[sk.Name] = id
	}
	if err := b.putAgent(ctx); err != nil {
		return fmt.Errorf("agent %s: %w", u.AgentID, err)
	}
	if err := b.bindSkills(ctx, ids); err != nil {
		return fmt.Errorf("agent %s skills: %w", u.AgentID, err)
	}
	return nil
}

// uploadSkill uploads one SKILL.md and returns the id to bind. A skill that
// already exists is read back: identical bytes are the same skill, different
// bytes are logged (the instance keeps its copy).
func (b *uarBackend) uploadSkill(ctx context.Context, sk uarSkill) (string, error) {
	r, err := b.cli.call(ctx, http.MethodPost, uarSkillImportPath, map[string]string{"skill_markdown": sk.Markdown}, nil)
	if err != nil {
		return "", err
	}
	switch {
	case r.ok():
		return skillID(r.Body, sk.Name), nil
	case r.Status != http.StatusConflict:
		return "", &uarHTTPError{Op: "skill import", Status: r.Status}
	}
	got, err := b.cli.call(ctx, http.MethodGet, uarSkillsPath+"/"+url.PathEscape(sk.Name), nil, nil)
	if err != nil {
		return "", err
	}
	if !got.ok() {
		return "", &uarHTTPError{Op: "skill read", Status: got.Status}
	}
	var rec struct {
		ArtifactDigest string `json:"artifact_digest"`
	}
	_ = json.Unmarshal(got.Body, &rec)
	if rec.ArtifactDigest != "" && rec.ArtifactDigest != skillDigest(sk.Markdown) {
		b.log.Warn("the UAR instance holds a different version of this skill and keeps it; upload does not overwrite", "skill", sk.Name)
	}
	return skillID(got.Body, sk.Name), nil
}

// skillID reads the id of a skill record, falling back to the skill name.
func skillID(body []byte, name string) string {
	var rec struct {
		SkillID string `json:"skill_id"`
		ID      string `json:"id"`
	}
	if json.Unmarshal(body, &rec) == nil {
		if rec.SkillID != "" {
			return rec.SkillID
		}
		if rec.ID != "" {
			return rec.ID
		}
	}
	return name
}

// putAgent creates the agent, or replaces it when it exists (If-Match with the
// ETag of the read).
func (b *uarBackend) putAgent(ctx context.Context) error {
	art := agentArtifact(b.set, b.prompt)
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
		hdr := map[string]string{}
		if etag := cur.Header.Get("ETag"); etag != "" {
			hdr["If-Match"] = etag
		}
		r, err = b.cli.call(ctx, http.MethodPut, path, art, hdr)
	default:
		return &uarHTTPError{Op: "agent read", Status: cur.Status}
	}
	if err != nil {
		return err
	}
	if !r.ok() {
		return &uarHTTPError{Op: "agent write", Status: r.Status}
	}
	return nil
}

// bindSkills makes the agent's skill bindings exactly the given skills: it
// binds the missing ones and unbinds any other.
func (b *uarBackend) bindSkills(ctx context.Context, ids map[string]string) error {
	path := uarAgentSkillsPath(b.set.UAR.AgentID)
	cur, err := b.cli.call(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	if !cur.ok() {
		return &uarHTTPError{Op: "skill bindings read", Status: cur.Status}
	}
	bound := boundSkills(cur.Body)
	want := make([]string, 0, len(ids))
	for _, id := range ids {
		want = append(want, id)
	}
	slices.Sort(want)
	for _, id := range want {
		if slices.Contains(bound, id) {
			continue
		}
		r, err := b.cli.call(ctx, http.MethodPost, path, map[string]string{"skill_id": id}, nil)
		if err != nil {
			return err
		}
		if !r.ok() && r.Status != http.StatusConflict {
			return &uarHTTPError{Op: "skill bind", Status: r.Status}
		}
	}
	for _, id := range bound {
		if slices.Contains(want, id) {
			continue
		}
		r, err := b.cli.call(ctx, http.MethodDelete, path+"/"+url.PathEscape(id), nil, nil)
		if err != nil {
			return err
		}
		if !r.ok() && r.Status != http.StatusNotFound {
			return &uarHTTPError{Op: "skill unbind", Status: r.Status}
		}
	}
	return nil
}

// boundSkills reads the ids in a bindings reply: a JSON array, or an object
// holding one under "skills" or "bindings"; each entry is an id string or an
// object with skill_id, id or name.
func boundSkills(body []byte) []string {
	var list []json.RawMessage
	if json.Unmarshal(body, &list) != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(body, &obj) != nil {
			return nil
		}
		for _, k := range []string{"skills", "bindings"} {
			if raw, ok := obj[k]; ok && json.Unmarshal(raw, &list) == nil {
				break
			}
		}
	}
	var out []string
	for _, raw := range list {
		var s string
		if json.Unmarshal(raw, &s) == nil && s != "" {
			out = append(out, s)
			continue
		}
		var rec struct {
			SkillID string `json:"skill_id"`
			ID      string `json:"id"`
			Name    string `json:"name"`
		}
		if json.Unmarshal(raw, &rec) != nil {
			continue
		}
		if id := firstNonEmpty(rec.SkillID, rec.ID, rec.Name); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// trimSnippet shortens s for a log line.
func trimSnippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}
