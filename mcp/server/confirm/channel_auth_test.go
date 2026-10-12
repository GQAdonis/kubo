package confirm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The MECHANISMS.md test vector (channel authentication, version 2).
const (
	vecKey       = "example-pairing-key"
	vecKHex      = "bb7a643be3798381e468615dc1397a9e2cb519d7d2a927fcb7f5ea7a7a6d847f"
	vecSocket    = "4242-1.sock"
	vecTimestamp = "1760270400"
	vecNonce     = "AAECAwQFBgcICQoLDA0ODw"
	vecSignature = "kJV7qp4TLPIC1phDIUnyT_W0YR9_eMnJlKfzv4uWtzs"
	vecProof     = "paaByo8rrHXnI8pwzkw0SFyWQ7DUnkM3CwbGy0bZsbs"
)

func TestAuthV2TestVector(t *testing.T) {
	k := sha256.Sum256([]byte(vecKey))
	assert.Equal(t, vecKHex, hex.EncodeToString(k[:]))

	nonce := make([]byte, 16)
	for i := range nonce {
		nonce[i] = byte(i)
	}
	assert.Equal(t, vecNonce, b64.EncodeToString(nonce))

	assert.Equal(t, vecSignature, signRequest(k[:], vecSocket, "GET", "/v1/pending", vecTimestamp, vecNonce, nil))
	assert.Equal(t, vecProof, serverProof(k[:], vecSocket, vecNonce, 200, []byte("{\"pending\":[]}\n")))
}

// headers builds version 2 headers signed over signedBody.
func (e *chanEnv) headers(key, method, path string, ts time.Time, nonce string, signedBody []byte) http.Header {
	k := sha256.Sum256([]byte(key))
	tss := strconv.FormatInt(ts.Unix(), 10)
	h := http.Header{}
	h.Set(authVersionHeader, authVersion)
	h.Set(timestampHeader, tss)
	h.Set(nonceHeader, nonce)
	h.Set(signatureHeader, signRequest(k[:], e.m.ch.socketName(), method, path, tss, nonce, signedBody))
	return h
}

func errCode(t *testing.T, body []byte) string {
	t.Helper()
	m := map[string]string{}
	require.NoError(t, json.Unmarshal(body, &m))
	return m["code"]
}

func TestAuthV2ServerProofOnEveryResponse(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	k := sha256.Sum256([]byte(e.key))
	nonce := newNonce()
	h := e.headers(e.key, http.MethodGet, pendingPath, time.Now(), nonce, nil)
	status, hdr, body := e.raw(t, http.MethodGet, pendingPath, h, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, serverProof(k[:], e.m.ch.socketName(), nonce, status, body), hdr.Get(proofHeader))

	// An error after the form check carries a proof too (stale timestamp).
	nonce = newNonce()
	h = e.headers(e.key, http.MethodGet, pendingPath, time.Now().Add(-time.Minute), nonce, nil)
	status, hdr, body = e.raw(t, http.MethodGet, pendingPath, h, nil)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, codeStale, errCode(t, body))
	assert.Equal(t, serverProof(k[:], e.m.ch.socketName(), nonce, status, body), hdr.Get(proofHeader))

	// No proof before the form check passes.
	status, hdr, _ = e.raw(t, http.MethodGet, pendingPath, http.Header{}, nil)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Empty(t, hdr.Get(proofHeader))
}

func TestAuthV2Rejections(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	resolveBody := []byte(`{"id":"cf_1","approve":true}`)
	good := func() http.Header {
		return e.headers(e.key, http.MethodPost, resolvePath, time.Now(), newNonce(), resolveBody)
	}
	send := func(h http.Header) (int, http.Header, []byte) {
		return e.raw(t, http.MethodPost, resolvePath, h, resolveBody)
	}

	t.Run("bearer", func(t *testing.T) {
		h := good()
		h.Set("Authorization", "Bearer "+e.key)
		status, hdr, body := send(h)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, codeUnsupported, errCode(t, body))
		assert.Equal(t, authVersion, hdr.Get(versionsHeader))
	})
	t.Run("version", func(t *testing.T) {
		h := good()
		h.Set(authVersionHeader, "1")
		status, _, body := send(h)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, codeUnsupported, errCode(t, body))
	})
	t.Run("stale", func(t *testing.T) {
		for _, d := range []time.Duration{-31 * time.Second, 31 * time.Second} {
			h := e.headers(e.key, http.MethodPost, resolvePath, time.Now().Add(d), newNonce(), resolveBody)
			status, _, body := send(h)
			assert.Equal(t, http.StatusUnauthorized, status, d)
			assert.Equal(t, codeStale, errCode(t, body), d)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		for name, mutate := range map[string]func(http.Header){
			"short nonce":         func(h http.Header) { h.Set(nonceHeader, "AAEC") },
			"padded nonce":        func(h http.Header) { h.Set(nonceHeader, vecNonce+"==") },
			"signature not b64":   func(h http.Header) { h.Set(signatureHeader, "!!!") },
			"leading zero":        func(h http.Header) { h.Set(timestampHeader, "0"+h.Get(timestampHeader)) },
			"signed timestamp":    func(h http.Header) { h.Set(timestampHeader, "+"+h.Get(timestampHeader)) },
			"missing signature":   func(h http.Header) { h.Del(signatureHeader) },
			"duplicate timestamp": func(h http.Header) { h.Add(timestampHeader, h.Get(timestampHeader)) },
		} {
			h := good()
			mutate(h)
			status, _, body := send(h)
			assert.Equal(t, http.StatusBadRequest, status, name)
			assert.Equal(t, codeMalformed, errCode(t, body), name)
		}
	})
	t.Run("signature for another socket", func(t *testing.T) {
		k := sha256.Sum256([]byte(e.key))
		ts, nonce := strconv.FormatInt(time.Now().Unix(), 10), newNonce()
		h := good()
		h.Set(timestampHeader, ts)
		h.Set(nonceHeader, nonce)
		h.Set(signatureHeader, signRequest(k[:], "other-9.sock", http.MethodPost, resolvePath, ts, nonce, resolveBody))
		status, _, body := send(h)
		assert.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, codeInvalid, errCode(t, body))
	})
	t.Run("tampered body", func(t *testing.T) {
		status, _, body := e.raw(t, http.MethodPost, resolvePath, good(), []byte(`{"id":"cf_1","approve":false}`))
		assert.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, codeInvalid, errCode(t, body))
	})
	t.Run("wrong key", func(t *testing.T) {
		h := e.headers("another-key", http.MethodPost, resolvePath, time.Now(), newNonce(), resolveBody)
		status, _, body := send(h)
		assert.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, codeInvalid, errCode(t, body))
	})
	t.Run("query string", func(t *testing.T) {
		h := e.headers(e.key, http.MethodGet, pendingPath, time.Now(), newNonce(), nil)
		status, _, body := e.raw(t, http.MethodGet, pendingPath+"?x=1", h, nil)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, codeMalformed, errCode(t, body))
	})
	e.none(t)

	t.Run("no pairing", func(t *testing.T) {
		require.NoError(t, os.Remove(filepath.Join(e.dir, "pairing.json")))
		h := e.headers(e.key, http.MethodGet, pendingPath, time.Now(), newNonce(), nil)
		status, _, body := e.raw(t, http.MethodGet, pendingPath, h, nil)
		assert.Equal(t, http.StatusForbidden, status)
		assert.Equal(t, codeInvalid, errCode(t, body))
	})
}

func TestAuthV2ReplayedNonce(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	h := e.headers(e.key, http.MethodGet, pendingPath, time.Now(), newNonce(), nil)
	status, _, _ := e.raw(t, http.MethodGet, pendingPath, h, nil)
	require.Equal(t, http.StatusOK, status)
	status, _, body := e.raw(t, http.MethodGet, pendingPath, h, nil)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, codeReplayed, errCode(t, body))
}

func TestAuthV2ReplayCacheBusyAndExpiry(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	now := time.Now()
	e.m.ch.now = func() time.Time { return now }
	for i := range replayCapacity {
		replayed, full := e.m.ch.replay.admit("n"+strconv.Itoa(i), now)
		require.False(t, replayed)
		require.False(t, full)
	}
	h := e.headers(e.key, http.MethodGet, pendingPath, now, newNonce(), nil)
	status, hdr, body := e.raw(t, http.MethodGet, pendingPath, h, nil)
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Equal(t, codeBusy, errCode(t, body))
	assert.Equal(t, "1", hdr.Get("Retry-After"))

	// A full cache evicts nothing early; entries leave after 60 s.
	now = now.Add(replayTTL + time.Second)
	h = e.headers(e.key, http.MethodGet, pendingPath, now, newNonce(), nil)
	status, _, _ = e.raw(t, http.MethodGet, pendingPath, h, nil)
	assert.Equal(t, http.StatusOK, status)
}

func TestAuthV2CLIPathUnchanged(t *testing.T) {
	e := newChanEnv(t, ModeDesktop, ModeCLI)
	status, out := e.do(t, http.MethodGet, pendingPath, "", cliValue, nil)
	assert.Equal(t, http.StatusOK, status)
	require.Len(t, out["pending"], 1)

	// Version 2 headers win: the cli marker on them is ignored, so this
	// request speaks for desktop and lists desktop's item only.
	h := e.headers(e.key, http.MethodGet, pendingPath, time.Now(), newNonce(), nil)
	h.Set(cliHeader, cliValue)
	status, _, body := e.raw(t, http.MethodGet, pendingPath, h, nil)
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), "Remove the pin desktop")
	assert.NotContains(t, string(body), "Remove the pin cli")

	// No key, no marker.
	status, _, body = e.raw(t, http.MethodGet, pendingPath, http.Header{}, nil)
	assert.Equal(t, http.StatusUnauthorized, status)
	assert.Equal(t, codeRequired, errCode(t, body))
}

func TestPendingViewCarriesTargets(t *testing.T) {
	e := newChanEnv(t, ModeDesktop)
	p := pending("Add a pin")
	p.ID = "cf_t"
	p.Summary.Targets = []server.Target{{Kind: "cid", Value: "bafy"}, {Kind: "mfs_path", Value: "/docs/a.txt"}}
	require.NoError(t, e.m.Resolvers()[0].Request(t.Context(), p, func(bool) error { return nil }))

	status, out := e.do(t, http.MethodGet, pendingPath, "v2:"+e.key, "", nil)
	require.Equal(t, http.StatusOK, status)
	var withTargets, without map[string]any
	for _, it := range out["pending"].([]any) {
		m := it.(map[string]any)
		if m["id"] == "cf_t" {
			withTargets = m
		} else {
			without = m
		}
	}
	require.NotNil(t, withTargets)
	assert.Equal(t, []any{
		map[string]any{"kind": "cid", "value": "bafy"},
		map[string]any{"kind": "mfs_path", "value": "/docs/a.txt"},
	}, withTargets["targets"])
	require.NotNil(t, without)
	assert.Equal(t, []any{}, without["targets"], "targets is always present, possibly empty")
}

func TestEnsurePrivateDirRefusesASymlink(t *testing.T) {
	base := shortDir(t)
	target := filepath.Join(base, "real")
	require.NoError(t, os.Mkdir(target, 0o700))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(target, link))

	err := ensurePrivateDir(link)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symbolic link")
	require.NoError(t, ensurePrivateDir(target))
}
