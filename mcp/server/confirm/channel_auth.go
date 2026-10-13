package confirm

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ipfs/kubo/mcp/server"
)

// Channel authentication version 2 (MECHANISMS.md, "Channel authentication,
// version 2"). The key never crosses the socket: each request carries an
// HMAC-SHA-256, keyed with the stored verifier, over the socket name, method,
// path, timestamp, nonce and body hash, and each response carries a proof that
// the server holds the verifier.
const (
	authVersionHeader = "X-IPFS-MCP-Auth-Version"
	timestampHeader   = "X-IPFS-MCP-Timestamp"
	nonceHeader       = "X-IPFS-MCP-Nonce"
	signatureHeader   = "X-IPFS-MCP-Signature"
	proofHeader       = "X-IPFS-MCP-Server-Proof"
	versionsHeader    = "X-IPFS-MCP-Auth-Versions"

	authVersion = "2"
	authDomain  = "ipfs-mcp-confirm-v2"

	authWindow     = 30 * time.Second
	replayTTL      = 2 * authWindow
	replayCapacity = 4096

	nonceBytes = 16
)

// Error codes of a rejected request.
const (
	codeRequired    = "auth_required"
	codeUnsupported = "auth_version_unsupported"
	codeMalformed   = "auth_malformed"
	codeStale       = "auth_stale"
	codeInvalid     = "auth_invalid"
	codeReplayed    = "auth_replayed"
	codeBusy        = "auth_busy"
)

// authResult is the outcome of authenticating one request.
type authResult struct {
	resolver string // set when the request is authenticated
	status   int    // HTTP status of a rejection
	code     string
	msg      string
	header   http.Header // extra response headers of a rejection
	// proof is set once the request passed the form check and a valid pairing
	// exists: every response then carries a server proof.
	key   []byte
	nonce string
}

func reject(status int, code, msg string) authResult {
	return authResult{status: status, code: code, msg: msg}
}

// replayCache remembers accepted nonces for replayTTL, at most replayCapacity.
type replayCache struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

// admit records nonce unless it was seen; full reports a cache with no room
// once expired entries are dropped. Nothing is evicted early.
func (rc *replayCache) admit(nonce string, now time.Time) (replayed, full bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.seen == nil {
		rc.seen = map[string]time.Time{}
	}
	for n, exp := range rc.seen {
		if !now.Before(exp) {
			delete(rc.seen, n)
		}
	}
	if _, ok := rc.seen[nonce]; ok {
		return true, false
	}
	if len(rc.seen) >= replayCapacity {
		return false, true
	}
	rc.seen[nonce] = now.Add(replayTTL)
	return false, false
}

func hmacSum(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func bodyHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var b64 = base64.RawURLEncoding.Strict()

// requestString is the canonical string a version 2 signature covers.
func requestString(socket, method, path, timestamp, nonce string, body []byte) string {
	return authDomain + "\nrequest\n" + socket + "\n" + method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + bodyHash(body)
}

// responseString is the canonical string a server proof covers.
func responseString(socket, nonce string, status int, body []byte) string {
	return authDomain + "\nresponse\n" + socket + "\n" + nonce + "\n" + strconv.Itoa(status) + "\n" + bodyHash(body)
}

// signRequest returns the signature header value; Desktop and the tests use it.
func signRequest(key []byte, socket, method, path, timestamp, nonce string, body []byte) string {
	return b64.EncodeToString(hmacSum(key, requestString(socket, method, path, timestamp, nonce, body)))
}

// serverProof returns the proof header value of a response.
func serverProof(key []byte, socket, nonce string, status int, body []byte) string {
	return b64.EncodeToString(hmacSum(key, responseString(socket, nonce, status, body)))
}

// newNonce returns 16 random bytes, base64url without padding.
func newNonce() string {
	b := make([]byte, nonceBytes)
	_, _ = rand.Read(b)
	return b64.EncodeToString(b)
}

// single returns the only value of a header, or false.
func single(r *http.Request, name string) (string, bool) {
	v := r.Header.Values(name)
	if len(v) != 1 || v[0] == "" {
		return "", false
	}
	return v[0], true
}

// wellFormedTimestamp accepts whole decimal seconds with no sign or leading
// zeros.
func wellFormedTimestamp(s string) (int64, bool) {
	if len(s) == 0 || len(s) > 15 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// wellFormedB64 reports whether s is base64url without padding of n bytes.
func wellFormedB64(s string, n int) bool {
	b, err := b64.DecodeString(s)
	return err == nil && len(b) == n
}

// authenticate checks a request in the order MECHANISMS.md fixes. body is the
// raw request body.
func (c *channel) authenticate(r *http.Request, body []byte) authResult {
	if r.Header.Get("Authorization") != "" {
		res := reject(http.StatusUnauthorized, codeUnsupported, "bearer keys are retired; sign each request (channel authentication version 2)")
		res.header = http.Header{versionsHeader: {authVersion}}
		return res
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		return reject(http.StatusBadRequest, codeMalformed, "the path must not carry a query string")
	}
	version, hasVersion := r.Header[http.CanonicalHeaderKey(authVersionHeader)]
	if !hasVersion {
		if r.Header.Get(cliHeader) == cliValue {
			if !c.cliOn {
				return reject(http.StatusForbidden, "", "cli confirmation is not enabled on this server; enable it with --confirm=cli or confirmation.cli")
			}
			return authResult{resolver: server.ResolverCLI}
		}
		return reject(http.StatusUnauthorized, codeRequired, "authentication is required")
	}
	if len(version) != 1 || version[0] != authVersion {
		res := reject(http.StatusUnauthorized, codeUnsupported, "unsupported authentication version")
		res.header = http.Header{versionsHeader: {authVersion}}
		return res
	}
	ts, tsOK := single(r, timestampHeader)
	nonce, nonceOK := single(r, nonceHeader)
	sig, sigOK := single(r, signatureHeader)
	tsVal, tsFormed := wellFormedTimestamp(ts)
	if !tsOK || !nonceOK || !sigOK || !tsFormed || !wellFormedB64(nonce, nonceBytes) || !wellFormedB64(sig, sha256.Size) {
		return reject(http.StatusBadRequest, codeMalformed, "a version 2 authentication header is missing or malformed")
	}
	key := c.verifier()
	res := authResult{key: key, nonce: nonce}
	fail := func(status int, code, msg string) authResult {
		out := reject(status, code, msg)
		out.key, out.nonce = key, nonce
		return out
	}
	now := c.clock()
	if d := now.Sub(time.Unix(tsVal, 0)); d > authWindow || d < -authWindow {
		return fail(http.StatusUnauthorized, codeStale, "the timestamp is outside the allowed window")
	}
	if key == nil {
		return fail(http.StatusForbidden, codeInvalid, "no valid pairing")
	}
	want := hmacSum(key, requestString(c.socketName(), r.Method, r.URL.Path, ts, nonce, body))
	got, _ := b64.DecodeString(sig)
	if subtle.ConstantTimeCompare(want, got) != 1 {
		return fail(http.StatusForbidden, codeInvalid, "the signature does not match")
	}
	replayed, full := c.replay.admit(nonce, now)
	switch {
	case replayed:
		return fail(http.StatusUnauthorized, codeReplayed, "this nonce was already used")
	case full:
		out := fail(http.StatusServiceUnavailable, codeBusy, "too many recent requests; retry shortly")
		out.header = http.Header{"Retry-After": {"1"}}
		return out
	}
	res.resolver = server.ResolverDesktop
	return res
}
