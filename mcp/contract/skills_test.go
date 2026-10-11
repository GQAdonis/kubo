package contract

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toolLike matches the names that look like contract tools. It is the same
// pattern as mcp/skills/scripts/check_skills.py.
var toolLike = regexp.MustCompile(`\b(?:ipfs|pin|ipns|key|mfs|dag|car|routing|swarm|stats|repo)_[a-z0-9_]+\b`)

const (
	skillsDir       = "../skills"
	pluginSkillsDir = "../plugin/ipfs/skills"
)

// skillFiles lists the files of every skill under root, as paths relative to
// root, in order. The evals and scripts directories hold no skill.
func skillFiles(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		require.NoError(t, filepath.WalkDir(filepath.Join(root, e.Name()), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		}))
	}
	slices.Sort(out)
	return out
}

// schemaWords returns every property name in the tool schemas, the error
// codes and the unavailability reasons: the non-tool names a skill may use
// that look like tool names.
func schemaWords(t *testing.T) map[string]struct{} {
	t.Helper()
	words := map[string]struct{}{}
	var walk func(n any)
	walk = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			if props, ok := x["properties"].(map[string]any); ok {
				for k := range props {
					words[k] = struct{}{}
				}
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	paths, err := fs.Glob(FS(), "schemas/tools/*.json")
	require.NoError(t, err)
	for _, p := range paths {
		raw, err := fs.ReadFile(FS(), p)
		require.NoError(t, err)
		var doc any
		require.NoError(t, json.Unmarshal(raw, &doc))
		walk(doc)
	}
	raw, err := fs.ReadFile(FS(), "schemas/common.json")
	require.NoError(t, err)
	var common struct {
		Defs struct {
			ErrorCode struct {
				Enum []string `json:"enum"`
			} `json:"ErrorCode"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(raw, &common))
	for _, c := range common.Defs.ErrorCode.Enum {
		words[c] = struct{}{}
	}
	raw, err = fs.ReadFile(FS(), "schemas/tools/ipfs_node_status.output.json")
	require.NoError(t, err)
	var status struct {
		Properties struct {
			Unavailable struct {
				Items struct {
					Properties struct {
						Reason struct {
							Enum []string `json:"enum"`
						} `json:"reason"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"unavailable_tools"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &status))
	for _, r := range status.Properties.Unavailable.Items.Properties.Reason.Enum {
		words[r] = struct{}{}
	}
	return words
}

// TestSkillsNameOnlyCatalogTools fails when a skill uses a tool-like name that
// is neither a catalog tool nor a schema field, error code or reason, so a
// tool rename cannot silently break the skills.
func TestSkillsNameOnlyCatalogTools(t *testing.T) {
	cat, err := Load()
	require.NoError(t, err)
	words := schemaWords(t)

	for _, root := range []string{skillsDir, pluginSkillsDir} {
		files := skillFiles(t, root)
		require.NotEmpty(t, files, root)
		for _, rel := range files {
			if !strings.HasSuffix(rel, ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, rel))
			require.NoError(t, err)
			for _, tok := range toolLike.FindAllString(string(raw), -1) {
				if _, ok := cat.Tool(tok); ok {
					continue
				}
				_, known := words[tok]
				assert.True(t, known, "%s: unknown tool-like name %q (not a catalog tool, schema field, error code or reason)", filepath.Join(root, rel), tok)
			}
		}
	}
}

// TestSkillMetadataToolsAreCatalogTools checks the metadata.tools list of each
// SKILL.md against the catalog.
func TestSkillMetadataToolsAreCatalogTools(t *testing.T) {
	cat, err := Load()
	require.NoError(t, err)
	listed := regexp.MustCompile(`(?m)^\s+tools:\s*(.+)$`)
	skills := 0
	for _, rel := range skillFiles(t, skillsDir) {
		if !strings.HasSuffix(rel, "/SKILL.md") {
			continue
		}
		skills++
		raw, err := os.ReadFile(filepath.Join(skillsDir, rel))
		require.NoError(t, err)
		m := listed.FindSubmatch(raw)
		require.NotNil(t, m, "%s has no metadata.tools", rel)
		for _, name := range strings.Fields(string(m[1])) {
			_, ok := cat.Tool(name)
			assert.True(t, ok, "%s: metadata.tools names %q, which the catalog does not define", rel, name)
		}
	}
	assert.NotZero(t, skills)
}

// TestPluginSkillsAreByteIdenticalToSource keeps mcp/plugin/ipfs/skills, which
// plugin hosts load, identical to mcp/skills, the source of truth. Refresh with
// mcp/plugin/sync-skills.sh.
func TestPluginSkillsAreByteIdenticalToSource(t *testing.T) {
	src := skillFiles(t, skillsDir)
	dst := skillFiles(t, pluginSkillsDir)
	require.NotEmpty(t, src)
	assert.Equal(t, src, dst, "the plugin carries exactly the skills of mcp/skills; run mcp/plugin/sync-skills.sh")
	for _, rel := range src {
		want, err := os.ReadFile(filepath.Join(skillsDir, rel))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(pluginSkillsDir, rel))
		if err != nil {
			continue // reported by the file list comparison above
		}
		assert.True(t, bytes.Equal(want, got), "%s differs between mcp/skills and the plugin copy; run mcp/plugin/sync-skills.sh", rel)
	}
}
