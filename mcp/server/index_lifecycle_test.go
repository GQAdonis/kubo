package server

import (
	"context"
	"slices"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/mcp/server/embeddings"
	mh "github.com/multiformats/go-multihash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (e *indexEnv) indexed(cid string) bool {
	e.t.Helper()
	n, err := e.store.CountCID(e.t.Context(), cid)
	require.NoError(e.t, err)
	return n > 0
}

func TestPinRmDeletesVectorsOfAnUnpinnedCID(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	a, b := testCID(t, 1), testCID(t, 2)
	e.files["/ipfs/"+a] = []byte("a") // the unpinned root is walked, whatever entries it has
	e.pins.pins[a] = fakePin{Type: "recursive"}
	e.pins.pins[b] = fakePin{Type: "recursive"}
	e.seed(a, 2)
	e.seed(b, 1)
	cs := e.connect(nil)

	res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + a})
	require.False(t, res.IsError, "%+v", res.Content)
	validateOutput(t, "pin_rm", structured(t, res))
	assert.False(t, e.indexed(a), "no pin covers it any more")
	assert.True(t, e.indexed(b), "other CIDs are untouched")
	assert.Empty(t, warningsOf(res))
}

func TestPinRmKeepsVectorsWhileAnotherPinCovers(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	a := testCID(t, 3)
	e.pins.pins[a] = fakePin{Type: "direct"}
	e.seed(a, 2)
	e.covered[a] = true // the same CID is also inside a recursive pin

	res := toolCall(t, e.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + a, "recursive": false})
	require.False(t, res.IsError, "%+v", res.Content)
	assert.True(t, e.indexed(a), "an indirect pin keeps the vectors")
	assert.Equal(t, 1, e.d.count("pin/ls"))
	last := e.d.lastCall("pin/ls")
	assert.Equal(t, "all", last.Opts["type"], "the check is pin ls --type=all")
	assert.Equal(t, []string{"/ipfs/" + a}, last.Args)
}

func TestPinRmKeepsVectorsWhenThePinCheckFails(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	a := testCID(t, 4)
	e.files["/ipfs/"+a] = []byte("a") // the unpinned root is walked, whatever entries it has
	e.pins.pins[a] = fakePin{Type: "recursive"}
	e.seed(a, 2)
	e.pinLsErr[a] = true

	res := toolCall(t, e.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + a})
	require.False(t, res.IsError, "the unpin itself succeeded: %+v", res.Content)
	assert.True(t, e.indexed(a), "never delete on a guess")
	warnings := warningsOf(res)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], a)
	assert.Contains(t, warnings[0], "kept")
}

func TestPinRmKeepsVectorsWhenTheStoreFails(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	a := testCID(t, 5)
	e.files["/ipfs/"+a] = []byte("a") // the unpinned root is walked, whatever entries it has
	e.pins.pins[a] = fakePin{Type: "recursive"}
	e.seed(a, 1)
	e.store.failCount.Store(true)

	res := toolCall(t, e.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + a})
	require.False(t, res.IsError)
	e.store.failCount.Store(false)
	assert.True(t, e.indexed(a))
	assert.Len(t, warningsOf(res), 1)
}

func TestPinRmWithoutEmbeddingsAsksNothing(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{noEmbeddings: true})
	a := testCID(t, 6)
	e.pins.pins[a] = fakePin{Type: "recursive"}
	res := toolCall(t, e.connect(nil), "pin_rm", map[string]any{"path": "/ipfs/" + a})
	require.False(t, res.IsError)
	assert.Zero(t, e.d.count("pin/ls"), "no extra pin check when nothing is indexed")
	assert.Empty(t, warningsOf(res))
}

func TestPinUpdateDeletesVectorsOfTheOldPinOnly(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	from, to, keep := testCID(t, 7), testCID(t, 8), testCID(t, 9)
	e.pins.pins[from] = fakePin{Type: "recursive"}
	e.pins.pins[keep] = fakePin{Type: "recursive"}
	e.seed(from, 1)
	e.seed(to, 1)
	e.seed(keep, 1)
	cs := e.connect(nil)

	callOK(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + from, "to": "/ipfs/" + to})
	assert.False(t, e.indexed(from))
	assert.True(t, e.indexed(to), "the new pin keeps its vectors")

	callOK(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + keep, "to": "/ipfs/" + testCID(t, 10), "unpin": false})
	assert.True(t, e.indexed(keep), "unpin=false leaves the old pin and its vectors")
}

func TestRepoGcDeletesVectorsOfRemovedIndexedCIDsOnly(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	sum, err := mh.Sum([]byte("gc doc"), mh.SHA2_256, -1)
	require.NoError(t, err)
	// The document was added as dag-pb CIDv0; garbage collection reports the
	// block as a raw-codec CIDv1 of the same multihash.
	indexedV0 := cid.NewCidV0(sum).String()
	removedKey := cid.NewCidV1(cid.Raw, sum).String()
	survivor, pinned := testCID(t, 11), testCID(t, 12)
	e.seed(indexedV0, 2)
	e.seed(survivor, 1)
	e.seed(pinned, 1)
	e.covered[pinned] = true
	e.gcKeys = []string{testCID(t, 500), removedKey, pinned}
	for i := range 250 { // far more than the sample of 100
		e.gcKeys = append(e.gcKeys, testCID(t, 1000+i))
	}
	e.gcKeys = append(e.gcKeys, cid.NewCidV1(cid.Raw, mustHash(t, "late")).String())

	res := toolCall(t, e.connect(nil), "repo_gc", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	validateOutput(t, "repo_gc", structured(t, res))
	assert.False(t, e.indexed(indexedV0), "removed and not pinned")
	assert.True(t, e.indexed(survivor), "not removed by the collection")
	assert.True(t, e.indexed(pinned), "reported removed but a pin still covers it: kept")
	assert.Empty(t, warningsOf(res))
}

func mustHash(t *testing.T, s string) mh.Multihash {
	t.Helper()
	sum, err := mh.Sum([]byte(s), mh.SHA2_256, -1)
	require.NoError(t, err)
	return sum
}

func TestRepoGcWithAnUnreachableStoreWarnsAndRemovesNothing(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	e.s.embeddingsExt = embeddings.NewService(embeddings.Settings{Mode: embeddings.ModeRemote}, brokenListStore{e.store}, mustEmbedder(t, e))
	e.gcKeys = []string{testCID(t, 600)}
	res := toolCall(t, e.connect(nil), "repo_gc", nil)
	require.False(t, res.IsError, "the collection itself is not affected")
	warnings := warningsOf(res)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "ipfs_index_reconcile")
}

func TestReconcile(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	cids := make([]string, 4)
	for i := range cids {
		cids[i] = testCID(t, 700+i)
		e.seed(cids[i], 1)
	}
	sorted := append([]string(nil), cids...)
	sortStrings(sorted)
	pinnedCID, unpinned1, errCID, unpinned2 := sorted[0], sorted[1], sorted[2], sorted[3]
	e.pins.pins[pinnedCID] = fakePin{Type: "recursive"}
	e.pinLsErr[errCID] = true
	cs := e.connect(nil)

	// A dry run reports and changes nothing.
	out := structured(t, toolCall(t, cs, "ipfs_index_reconcile", map[string]any{"dry_run": true}))
	validateOutput(t, "ipfs_index_reconcile", out)
	assert.Equal(t, true, out["dry_run"])
	assert.EqualValues(t, 4, out["checked"])
	assert.EqualValues(t, 2, out["kept"])
	assert.ElementsMatch(t, []any{unpinned1, unpinned2}, out["deleted"])
	assert.Len(t, e.store.cids(), 4)

	res := toolCall(t, cs, "ipfs_index_reconcile", nil)
	require.False(t, res.IsError, "%+v", res.Content)
	out = structured(t, res)
	validateOutput(t, "ipfs_index_reconcile", out)
	assert.ElementsMatch(t, []any{unpinned1, unpinned2}, out["deleted"])
	assert.ElementsMatch(t, []string{pinnedCID, errCID}, e.store.cids(), "a failed pin check keeps the vectors")
	warnings := warningsOf(res)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], errCID)
	assert.Equal(t, map[string]any{"has_more": false}, out["page"])
}

func TestReconcilePaginates(t *testing.T) {
	e := newIndexEnv(t, indexEnvOptions{})
	var cids []string
	for i := range 5 {
		c := testCID(t, 800+i)
		cids = append(cids, c)
		e.seed(c, 1)
	}
	sortStrings(cids)
	cs := e.connect(nil)

	var deleted []string
	cursor := ""
	for page := 0; ; page++ {
		args := map[string]any{"limit": 2}
		if cursor != "" {
			args["cursor"] = cursor
		}
		out := structured(t, toolCall(t, cs, "ipfs_index_reconcile", args))
		validateOutput(t, "ipfs_index_reconcile", out)
		assert.LessOrEqual(t, out["checked"], float64(2))
		for _, c := range out["deleted"].([]any) {
			deleted = append(deleted, c.(string))
		}
		p := out["page"].(map[string]any)
		if p["has_more"] != true {
			assert.Equal(t, 2, page, "five CIDs in pages of two are three pages")
			break
		}
		cursor = p["next_cursor"].(string)
		require.NotEmpty(t, cursor)
		require.Less(t, page, 10)
	}
	assert.Equal(t, cids, deleted, "every indexed CID is visited once, in order, even as pages delete")
	assert.Empty(t, e.store.cids())

	res := toolCall(t, cs, "ipfs_index_reconcile", map[string]any{"cursor": "!!"})
	assert.Equal(t, CodeInvalidArgument, decodeToolError(t, res).Code)
}

func sortStrings(s []string) { slices.Sort(s) }

// brokenListStore cannot list its CIDs.
type brokenListStore struct{ *memStore }

func (brokenListStore) ListCIDs(context.Context, string, int) ([]string, error) {
	return nil, assert.AnError
}
