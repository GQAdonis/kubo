//go:build !noagent

package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent surfaces (A2A tasks and the AG-UI chat loop) reach tools through
// Server.CallTool; both bulk write tools must be refused there without an
// approval that arrives out of band.
func TestBulkWriteToolsAreRefusedForAgentPrincipalsWithoutAnApproval(t *testing.T) {
	n, _ := threeFiles()
	res := newFakeResolver(ResolverLinuxPolkit)
	b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.options = Options{Resolvers: []Resolver{res}} })

	for _, kind := range []string{KindA2A, KindUI} {
		t.Run(kind, func(t *testing.T) {
			id := a2aIdentity("agent-"+kind, kind, []string{AllGroups}, []string{"index"}, nil, []string{b.embed.host()})
			noShell := false
			id.ShellCapable = &noShell
			for _, tool := range []string{"ipfs_index_bulk_start", "ipfs_index_bulk_cancel"} {
				out, err := b.s.CallTool(t.Context(), id, "k-"+kind, tool, json.RawMessage(`{}`))
				require.NoError(t, err)
				require.True(t, out.IsError, tool)
				te := errorOf(out)
				require.NotNil(t, te, tool)
				assert.Contains(t, []string{CodeConfirmRequired, CodeConfirmUnavailable, CodeConfirmPending}, te.Code, "%s: %s", tool, te.Message)
				if kind == KindA2A && te.Confirmation != nil {
					assert.Empty(t, te.Confirmation.ConfirmToken, "an a2a principal is never issued a token")
				}
				if tok := ""; te.Confirmation != nil {
					// The chat is handed a token to present after a human approved;
					// presenting it before that approves nothing.
					tok = te.Confirmation.ConfirmToken
					if tok != "" {
						again, err := b.s.CallTool(t.Context(), id, "k-"+kind, tool, json.RawMessage(`{"confirm_token":"`+tok+`"}`))
						require.NoError(t, err)
						assert.True(t, again.IsError, "%s: an unapproved token runs nothing", tool)
					}
				}
			}
		})
	}
	assert.Nil(t, b.store.stored(t), "no run began")
	assert.Nil(t, b.s.bulk.active())
	assert.Zero(t, b.embed.requests.Load())
}

func TestBulkStartForAnAgentRunsOnceAfterThePresenceResolverApproves(t *testing.T) {
	n, _ := threeFiles()
	res := newFakeResolver(ResolverLinuxPolkit)
	b := newTextBulkEnv(t, n, func(o *mediaOpts) { o.options = Options{Resolvers: []Resolver{res}} })
	id := a2aIdentity("agent", KindA2A, []string{AllGroups}, []string{"index"}, nil, []string{b.embed.host()})
	noShell := false
	id.ShellCapable = &noShell

	out, err := b.s.CallTool(t.Context(), id, "k", "ipfs_index_bulk_start", json.RawMessage(`{"sources":["pins"]}`))
	require.NoError(t, err)
	te := errorOf(out)
	require.Equal(t, CodeConfirmRequired, te.Code)
	assert.Contains(t, te.Confirmation.Summary.Text, "pins")
	assert.Contains(t, te.Confirmation.Summary.Text, "Contacts only this machine", "the approver sees the hosts")
	assert.Nil(t, b.store.stored(t))

	require.NoError(t, b.s.ResolveConfirmation(te.Confirmation.ConfirmationID, ResolverLinuxPolkit, true))
	out, err = b.s.CallTool(t.Context(), id, "k", "ipfs_index_bulk_start", json.RawMessage(`{"sources":["pins"]}`))
	require.NoError(t, err)
	require.False(t, out.IsError, "%+v", out.StructuredContent)
	b.wait()
	assert.Equal(t, 3, b.store.stored(t).FilesIndexed)
}
