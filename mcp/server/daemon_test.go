package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rpcServer is an httptest stand-in for the /api/v0 surface this package uses.
// When authz is set, every command requires that Authorization header.
func rpcServer(t *testing.T, version string, authz string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, r *http.Request, v any) {
		if authz != "" && r.Header.Get("Authorization") != authz {
			http.Error(w, "missing authorization", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/api/v0/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, map[string]string{"Version": version, "Commit": "c0ffee", "Repo": "18"})
	})
	mux.HandleFunc("/api/v0/commands", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, commandTree([]string{"id", "version", "commands", "pin/ls"}))
	})
	mux.HandleFunc("/api/v0/id", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, map[string]any{"ID": testPeerID, "Addresses": []string{"/ip4/127.0.0.1/tcp/4001"}, "AgentVersion": "kubo/test"})
	})
	mux.HandleFunc("/api/v0/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom went the daemon"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRPCDaemonOverHTTP(t *testing.T) {
	srv := rpcServer(t, "0.44.0-dev", "")
	d, err := Discover(DiscoverOptions{API: srv.URL})
	require.NoError(t, err)

	s := mustNew(t, d, Options{})
	cs := connect(t, s)
	assert.ElementsMatch(t, []string{"ipfs_node_info", "ipfs_node_status", "pin_ls"}, toolNames(t, cs), "only tools whose RPC commands the daemon lists are offered")

	var out nodeInfoOutput
	require.NoError(t, json.Unmarshal(callOK(t, cs, "ipfs_node_info", nil), &out))
	assert.Equal(t, testPeerID, out.PeerID)
	assert.Equal(t, "0.44.0-dev", out.Version)
	validateOutput(t, "ipfs_node_info", out)
}

func TestRPCDaemonErrorMapping(t *testing.T) {
	srv := rpcServer(t, "0.44.0-dev", "")
	d, err := Discover(DiscoverOptions{API: srv.URL})
	require.NoError(t, err)

	err = d.Call(t.Context(), "boom", nil, nil, nil)
	var rpcErr *RPCError
	require.ErrorAs(t, err, &rpcErr)
	assert.Equal(t, "boom", rpcErr.Command)
	assert.Contains(t, rpcErr.Message, "boom went the daemon")
}

func TestRPCDaemonUnreachable(t *testing.T) {
	srv := rpcServer(t, "0.44.0-dev", "")
	url := srv.URL
	srv.Close()
	d, err := Discover(DiscoverOptions{API: url})
	require.NoError(t, err)

	_, err = New(t.Context(), d, Options{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonUnreachable)
	assert.Contains(t, err.Error(), url, "the error names the API address it tried")
}

func TestDiscoverSendsAuthorization(t *testing.T) {
	srv := rpcServer(t, "0.44.0-dev", "Bearer s3cret")

	noAuth, err := Discover(DiscoverOptions{API: srv.URL})
	require.NoError(t, err)
	_, err = New(t.Context(), noAuth, Options{})
	require.Error(t, err)

	withAuth, err := Discover(DiscoverOptions{API: srv.URL, Authorization: "Bearer s3cret"})
	require.NoError(t, err)
	_, err = New(t.Context(), withAuth, Options{})
	require.NoError(t, err)
}

func TestDiscoverFromAPIFile(t *testing.T) {
	srv := rpcServer(t, "0.44.0-dev", "")
	hostParts := strings.Split(strings.TrimPrefix(srv.URL, "http://"), ":")
	want := "/ip4/" + hostParts[0] + "/tcp/" + hostParts[1]
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "api"), []byte(want+"\n"), 0o600))

	d, err := Discover(DiscoverOptions{IPFSPath: dir})
	require.NoError(t, err)
	assert.Equal(t, want, d.Endpoint())
	mustNew(t, d, Options{})
}

func TestDiscoverUsesIPFSPathEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IPFS_PATH", dir)
	_, err := Discover(DiscoverOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDaemonUnreachable)
	assert.Contains(t, err.Error(), filepath.Join(dir, "api"))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "api"), []byte("/ip4/127.0.0.1/tcp/5999"), 0o600))
	d, err := Discover(DiscoverOptions{})
	require.NoError(t, err)
	assert.Equal(t, "/ip4/127.0.0.1/tcp/5999", d.Endpoint())
}

func TestDiscoverDoesNotCreateRepoFiles(t *testing.T) {
	dir := t.TempDir()
	_, _ = Discover(DiscoverOptions{IPFSPath: dir})
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "discovery only reads the api file; it never opens the repo")
}

func TestDiscoverRejectsGarbage(t *testing.T) {
	_, err := Discover(DiscoverOptions{API: "not an address"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--api")
}
