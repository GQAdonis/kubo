package embeddings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	replaced map[string][]Row
	hits     []Hit
	searched [][]float32
}

func (f *fakeStore) Bootstrap(context.Context) error { return nil }
func (f *fakeStore) Replace(_ context.Context, cid string, rows []Row) error {
	if f.replaced == nil {
		f.replaced = map[string][]Row{}
	}
	f.replaced[cid] = rows
	return nil
}
func (f *fakeStore) Search(_ context.Context, v []float32, _ int) ([]Hit, error) {
	f.searched = append(f.searched, v)
	return f.hits, nil
}
func (f *fakeStore) DeleteCID(context.Context, string) (int, error) { return 0, nil }
func (f *fakeStore) Count(context.Context) (int, error)             { return 0, nil }
func (f *fakeStore) SearchCID(_ context.Context, v []float32, _ string, _ int) ([]Hit, error) {
	f.searched = append(f.searched, v)
	return f.hits, nil
}
func (f *fakeStore) CountCID(context.Context, string) (int, error) { return 0, nil }
func (f *fakeStore) CountCIDs(context.Context) (int, error)        { return 0, nil }
func (f *fakeStore) CountCIDsByKind(context.Context) (map[string]int, error) {
	return map[string]int{}, nil
}
func (f *fakeStore) ListCIDs(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func TestServiceIndexAndSearchRespectEgress(t *testing.T) {
	t.Parallel()
	rs := newReplayServer(t, reply{match: "bge-m3", body: `{"data":[{"embedding":[1,0,0,0],"index":0}]}`})
	st := &fakeStore{hits: []Hit{{CID: "bafyA"}}}
	e := testEmbedder(t, rs.URL, "")
	svc := &Service{embedder: e, store: st, storeHost: "db.example:8000"}

	doc := Document{CID: "bafyA", Path: "/a", MIME: "text/plain", Chunks: []string{"one"}}
	ctx := t.Context()

	// The store host is covered but the embedder is not: refused before any request.
	onlyStore := func(h string) bool { return h == "db.example:8000" }
	_, err := svc.Index(ctx, onlyStore, doc)
	var eg *EgressError
	require.ErrorAs(t, err, &eg)
	require.Equal(t, []string{e.Host()}, eg.Hosts)
	_, err = svc.Search(ctx, nil, "q", 3)
	require.ErrorIs(t, err, ErrEgressNotAcknowledged)
	require.Empty(t, rs.requests())
	require.Empty(t, st.replaced)

	// Both covered: embeds, then replaces.
	both := func(h string) bool { return h == "db.example:8000" || h == e.Host() }
	n, err := svc.Index(ctx, both, doc)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, "bge-m3", st.replaced["bafyA"][0].Model)
	hits, err := svc.Search(ctx, both, "query", 3)
	require.NoError(t, err)
	require.Equal(t, "bafyA", hits[0].CID)
	require.Len(t, rs.requests(), 2)

	_, err = svc.Index(ctx, both, Document{CID: "x"})
	require.Error(t, err)
	_, err = svc.Search(ctx, both, "  ", 3)
	require.Error(t, err)
}

func TestServiceHostsOmitsLocalStore(t *testing.T) {
	t.Parallel()
	e := testEmbedder(t, "http://127.0.0.1:9", "")
	require.Equal(t, []string{"127.0.0.1:9"}, (&Service{embedder: e}).Hosts())
	require.Equal(t, []string{"127.0.0.1:9", "db:1"}, (&Service{embedder: e, storeHost: "db:1"}).Hosts())
}

func TestOpenRefusesInvalidSettingsBeforeAnyIO(t *testing.T) {
	t.Parallel()
	svc, err := Open(t.Context(), Settings{Mode: ModeRemote, Embedder: validEmbedder()}, Options{})
	require.Nil(t, svc)
	require.ErrorContains(t, err, "embeddings.remote.endpoint")
	require.NoError(t, (*Service)(nil).Close())
}
