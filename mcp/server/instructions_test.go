package server

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstructionsNameSkills(t *testing.T) {
	s := mustNew(t, newFakeDaemon(t), Options{Version: "test"})
	cs := connect(t, s)

	got := cs.InitializeResult().Instructions
	require.NotEmpty(t, got)
	assert.Equal(t, instructions, got)
	assert.Less(t, len(instructions), 1100)
	assert.Contains(t, got, "untrusted peers")

	// Every ipfs-* skill directory is named, and every named skill exists.
	entries, err := os.ReadDir(filepath.Join("..", "skills"))
	require.NoError(t, err)
	dirs := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) > 5 && e.Name()[:5] == "ipfs-" {
			dirs[e.Name()] = struct{}{}
		}
	}
	named := regexp.MustCompile(`ipfs-[a-z-]+`).FindAllString(got, -1)
	assert.Len(t, named, 8)
	assert.Len(t, dirs, 8)
	for _, n := range named {
		assert.Contains(t, dirs, n)
		assert.FileExists(t, filepath.Join("..", "skills", n, "SKILL.md"))
	}
}
