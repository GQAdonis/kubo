//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// validateWithAGUICore checks events and inputs against the published
// @ag-ui/core schemas (G9-1) when AGUI_CORE_DIR names a directory that holds
// them (see agui/SOURCES.md). It skips otherwise: Kubo depends on neither node
// nor the package.
func validateWithAGUICore(t *testing.T, inputs []map[string]any, events []aguiEvent) {
	t.Helper()
	dir := os.Getenv("AGUI_CORE_DIR")
	if dir == "" {
		return
	}
	node, err := exec.LookPath("node")
	require.NoError(t, err, "AGUI_CORE_DIR is set but node is not installed")
	doc, err := json.Marshal(map[string]any{"inputs": inputs, "events": events})
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), node, "agui/validate.cjs")
	cmd.Env = append(os.Environ(), "AGUI_CORE_DIR="+dir)
	cmd.Stdin = bytes.NewReader(doc)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), "@ag-ui/core rejected the events:\n%s%s", out.String(), errb.String())
	t.Log(errb.String())
}
