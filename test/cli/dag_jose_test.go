package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multicodecDagJOSE is the dag-jose multicodec code.
const multicodecDagJOSE = 0x85

// TestDagJose round-trips a dag-jose object through the real binary. The
// dag-jose codec parses JWS with go-jose; the build pins go-jose v4.1.6 through
// the temporary p2p-forge fork replace, and this keeps the codec honest under it.
func TestDagJose(t *testing.T) {
	t.Parallel()
	node := harness.NewT(t).NewNode().Init().StartDaemon()
	defer node.StopDaemon()

	// A dag-jose payload is the binary form of a CID.
	link := node.PipeStrToIPFS(`{"a":1}`, "dag", "put", "--store-codec", "dag-cbor").Stdout.Trimmed()
	linkCID, err := cid.Decode(link)
	require.NoError(t, err)
	b64 := base64.RawURLEncoding.EncodeToString
	jws, err := json.Marshal(map[string]any{
		"payload": b64(linkCID.Bytes()),
		"signatures": []map[string]string{{
			"protected": b64([]byte(`{"alg":"EdDSA"}`)),
			"signature": b64(bytes.Repeat([]byte{0x5a}, 64)),
		}},
	})
	require.NoError(t, err)

	// The general JWS form goes in as dag-json and comes out as dag-jose.
	stored := node.PipeToIPFS(bytes.NewReader(jws), "dag", "put", "--input-codec", "dag-json", "--store-codec", "dag-jose").Stdout.Trimmed()
	storedCID, err := cid.Decode(stored)
	require.NoError(t, err)
	assert.EqualValues(t, multicodecDagJOSE, storedCID.Prefix().Codec, "the CID's codec is dag-jose")

	// The raw block is the dag-jose encoding.
	block := node.IPFS("block", "get", stored).Stdout.Bytes()
	require.NotEmpty(t, block)

	// Feeding the block back as dag-jose gives the same CID and the same bytes.
	again := node.PipeToIPFS(bytes.NewReader(block), "dag", "put", "--input-codec", "dag-jose", "--store-codec", "dag-jose").Stdout.Trimmed()
	assert.Equal(t, stored, again)
	assert.Equal(t, block, node.IPFS("block", "get", again).Stdout.Bytes(), "the bytes round-trip unchanged")
	assert.Equal(t, block, node.IPFS("dag", "get", "--output-codec", "dag-jose", stored).Stdout.Bytes())

	// The decoded view names the payload link and the signature.
	var view struct {
		Link       map[string]string `json:"link"`
		Payload    string            `json:"payload"`
		Signatures []map[string]any  `json:"signatures"`
	}
	require.NoError(t, json.Unmarshal(node.IPFS("dag", "get", stored).Stdout.Bytes(), &view))
	assert.Equal(t, link, view.Link["/"])
	assert.Equal(t, b64(linkCID.Bytes()), view.Payload)
	require.Len(t, view.Signatures, 1)
}
