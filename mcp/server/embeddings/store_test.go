package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T, rs *replayServer, dims int, cred Secret) *surrealStore {
	t.Helper()
	c := newSurrealClient(rs.URL, "nsx", "dbx", parseCredential(cred), nil)
	return newSurrealStore(c, dims)
}

func TestBootstrapSendsSchemaAndChecksDimension(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t,
		reply{match: "INFO FOR TABLE", body: fixture(t, "rpc_info_table_dim4.json")},
		reply{match: "DEFINE", body: fixture(t, "rpc_ddl_ok.json")},
	)
	s := testStore(t, rs, 4, "root:pw")
	require.NoError(t, s.Bootstrap(t.Context()))
	reqs := rs.requests()
	require.Len(t, reqs, 3)
	require.Equal(t, "/rpc", reqs[0].Path)
	require.Equal(t, "nsx", reqs[0].NS)
	require.Equal(t, "dbx", reqs[0].DB)
	require.True(t, strings.HasPrefix(reqs[0].Auth, "Basic "))
	require.Contains(t, reqs[1].Body, "HNSW DIMENSION 4 DIST COSINE TYPE F32")
	require.Contains(t, reqs[1].Body, `array\u003cfloat, 4\u003e`) // JSON-escaped

	// An index built for another dimension is reported, not reused.
	s = testStore(t, rs, 1024, "root:pw")
	err := s.Bootstrap(t.Context())
	require.ErrorIs(t, err, ErrDimensionMismatch)
	require.ErrorContains(t, err, "4-dimensional")
}

func TestStatementErrorsAreClippedAndNamed(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 1000)
	rs := newReplayServer(t,
		reply{match: "SELECT count", body: `{"id":1,"result":[{"result":"` + long + `","status":"ERR"}]}`},
		reply{match: "DELETE", body: fixture(t, "rpc_stmt_error.json")},
		reply{match: "INFO", body: fixture(t, "rpc_parse_error.json"), status: 400},
	)
	s := testStore(t, rs, 4, "")
	_, err := s.Count(t.Context())
	require.Error(t, err)
	require.Less(t, len(err.Error()), 500)
	_, err = s.DeleteCID(t.Context(), "bafy")
	require.ErrorContains(t, err, "nosuch_table_x")
	err = s.checkDimension(t.Context())
	require.ErrorContains(t, err, "Parse error")
	for _, r := range rs.requests() {
		require.Empty(t, r.Auth, "no credential configured means no Authorization header")
	}
}

func TestUnauthorizedIsErrAuth(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t, reply{match: "", status: 401, body: "The password did not verify"})
	s := testStore(t, rs, 4, "root:bad")
	_, err := s.Count(t.Context())
	require.True(t, errors.Is(err, ErrAuth))
	require.NotContains(t, err.Error(), "bad")
}

func TestBearerCredential(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t, reply{match: "count", body: fixture(t, "rpc_count_empty.json")})
	s := testStore(t, rs, 4, "tok.en.value")
	n, err := s.Count(t.Context())
	require.NoError(t, err)
	require.Zero(t, n)
	require.Equal(t, "Bearer tok.en.value", rs.requests()[0].Auth)
}

func TestReplaceSearchDelete(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t,
		reply{match: "vector::distance::knn", body: fixture(t, "rpc_knn_hits.json")},
		reply{match: "BEGIN TRANSACTION", body: `{"id":1,"result":[{"result":null,"status":"OK"},{"result":[],"status":"OK"},{"result":null,"status":"OK"},{"result":null,"status":"OK"}]}`},
		reply{match: "DELETE", body: `{"id":1,"result":[{"result":[{"cid":"bafyA"},{"cid":"bafyA"}],"status":"OK"}]}`},
	)
	s := testStore(t, rs, 2, "")
	ctx := context.Background()

	rows := []Row{
		{CID: "bafyA", Path: "/a", MIME: "text/plain", ChunkIndex: 0, Model: "m", Vector: []float32{1, 0}},
		{CID: "bafyA", Path: "/a", MIME: "text/plain", ChunkIndex: 1, Model: "m", Vector: []float32{0, 1}},
	}
	require.NoError(t, s.Replace(ctx, "bafyA", rows))
	body := rs.requests()[0].Body
	var env struct {
		Params []json.RawMessage `json:"params"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &env))
	var vars struct {
		CID  string `json:"cid"`
		Rows []struct {
			ID   string `json:"id"`
			Dims int    `json:"dims"`
		} `json:"rows"`
	}
	require.NoError(t, json.Unmarshal(env.Params[1], &vars))
	require.Equal(t, "bafyA", vars.CID)
	require.Len(t, vars.Rows, 2)
	require.NotEqual(t, vars.Rows[0].ID, vars.Rows[1].ID)
	require.Equal(t, rowID("bafyA", 0, "m"), vars.Rows[0].ID)
	require.Equal(t, 2, vars.Rows[0].Dims)

	require.ErrorContains(t, s.Replace(ctx, "bafyA", []Row{{CID: "other", Vector: []float32{1, 0}}}), "other")
	require.ErrorContains(t, s.Replace(ctx, "bafyA", []Row{{CID: "bafyA", Vector: []float32{1}}}), "dimensions")

	hits, err := s.Search(ctx, []float32{1, 0}, 2)
	require.NoError(t, err)
	require.Len(t, hits, 2)
	require.Equal(t, "bafyB", hits[1].CID)
	_, err = s.Search(ctx, []float32{1, 0, 0}, 2)
	require.Error(t, err)
	_, err = s.Search(ctx, []float32{1, 0}, 0)
	require.Error(t, err)

	n, err := s.DeleteCID(ctx, "bafyA")
	require.NoError(t, err)
	require.Equal(t, 2, n)
}

func TestListAndCountStatements(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t,
		reply{match: "GROUP BY cid ORDER BY cid", body: `{"id":1,"result":[{"result":[{"cid":"bafyA"},{"cid":"bafyB"}],"status":"OK"}]}`},
		reply{match: "FROM (SELECT cid", body: `{"id":1,"result":[{"result":[{"n":7}],"status":"OK"}]}`},
		reply{match: "WHERE cid = $cid GROUP ALL", body: `{"id":1,"result":[{"result":[],"status":"OK"}]}`},
		reply{match: "vector::similarity::cosine", body: `{"id":1,"result":[{"result":[{"cid":"bafyA","path":"/a","mime":"text/plain","chunk_index":1,"model":"m","distance":0.25}],"status":"OK"}]}`},
	)
	s := testStore(t, rs, 2, "")
	ctx := context.Background()

	cids, err := s.ListCIDs(ctx, "bafy0", 2)
	require.NoError(t, err)
	require.Equal(t, []string{"bafyA", "bafyB"}, cids)
	last := rs.requests()[len(rs.requests())-1].Body
	require.Contains(t, last, "LIMIT 2")
	require.Contains(t, last, `"after":"bafy0"`, "the cursor is a bound variable, not SurrealQL text")
	_, err = s.ListCIDs(ctx, "", 0)
	require.Error(t, err)

	n, err := s.CountCIDs(ctx)
	require.NoError(t, err)
	require.Equal(t, 7, n)

	n, err = s.CountCID(ctx, "bafyZ")
	require.NoError(t, err)
	require.Zero(t, n, "no rows means zero")
	_, err = s.CountCID(ctx, "")
	require.Error(t, err)

	hits, err := s.SearchCID(ctx, []float32{1, 0}, "bafyA", 3)
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.InDelta(t, 0.25, hits[0].Distance, 1e-9)
	body := rs.requests()[len(rs.requests())-1].Body
	require.Contains(t, body, "LIMIT 3")
	require.Contains(t, body, `"cid":"bafyA"`)
	_, err = s.SearchCID(ctx, []float32{1, 0}, "", 3)
	require.Error(t, err)
	_, err = s.SearchCID(ctx, []float32{1}, "bafyA", 3)
	require.ErrorContains(t, err, "dimensions")
}
