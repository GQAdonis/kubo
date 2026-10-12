package confirm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs/kubo/mcp/server"
)

// Channel protocol: HTTP over a Unix socket in a private directory.
//
//	GET  /v1/pending            list the confirmations waiting on this client
//	POST /v1/resolve            {"id": "...", "approve": true|false}
//
// A client authenticates one of two ways:
//
//	X-IPFS-MCP-Auth-Version: 2 ...        the paired IPFS Desktop window; each
//	                                      request is signed with the stored
//	                                      verifier and each response carries a
//	                                      server proof (channel_auth.go)
//	X-IPFS-MCP-Confirm: cli               `ipfs mcp confirm --confirm=cli`
//
// A request with neither is rejected, a bearer key is refused, and the cli
// form is rejected unless the operator started the server with
// --confirm=cli. The cli form carries no secret: it is the explicit headless
// opt-in, whose only factor is the terminal the person types into (see
// MECHANISMS.md for its limit).
const (
	cliHeader   = "X-IPFS-MCP-Confirm"
	cliValue    = "cli"
	pendingPath = "/v1/pending"
	resolvePath = "/v1/resolve"
	maxBody     = 4 << 10
)

// item is one confirmation a channel client may decide.
type item struct {
	p       server.Pending
	resolve server.ResolveFunc
	// decided hides the item from the pending list once any client resolved
	// it. It stays addressable until the confirmation expires, so a repeat
	// resolve gets the broker's "already resolved" answer.
	decided bool
}

// PendingView is the wire form of a waiting confirmation.
type PendingView struct {
	ID            string    `json:"id"`
	Summary       string    `json:"summary"`
	Tool          string    `json:"tool"`
	Class         string    `json:"class"`
	ExpiresAt     time.Time `json:"expires_at"`
	NativePrompt  bool      `json:"native_prompt,omitempty"`
	PrincipalKind string    `json:"principal_kind,omitempty"`
	// Targets are the summary's structured targets; always present.
	Targets []server.Target `json:"targets"`
}

// channelSeq numbers the channels of this process.
var channelSeq atomic.Uint64

// channel serves the desktop and cli resolvers.
type channel struct {
	log *slog.Logger
	// pairingPath is the file that holds the key hash. It is read again for
	// every request, so a replaced pairing revokes the old key at once and a
	// removed one revokes it entirely. Empty means desktop is not offered.
	pairingPath string
	cliOn       bool
	sockPath    string
	replay      replayCache
	// now is the clock of the timestamp window; tests replace it.
	now func() time.Time

	ln  net.Listener
	srv *http.Server

	mu    sync.Mutex
	items map[string]map[string]item // resolver name -> id -> item
}

// newChannel listens on a private socket. pairingPath may be empty (desktop
// requests are then all rejected).
func (c *channel) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// socketName is the base name of the socket, which a signature binds.
func (c *channel) socketName() string { return filepath.Base(c.sockPath) }

func newChannel(log *slog.Logger, dir, pairingPath string, cliOn bool) (*channel, error) {
	if err := ensurePrivateDir(dir); err != nil {
		return nil, err
	}
	// One socket per channel instance: a second channel in this process must not
	// replace the first one's socket, and closing either removes only its own.
	path := filepath.Join(dir, fmt.Sprintf("%d-%d.sock", os.Getpid(), channelSeq.Add(1)))
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on confirmation socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("restrict confirmation socket: %w", err)
	}
	c := &channel{
		log: log, pairingPath: pairingPath, cliOn: cliOn, sockPath: path, ln: ln,
		items: map[string]map[string]item{server.ResolverDesktop: {}, server.ResolverCLI: {}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+pendingPath, c.serve(c.handlePending))
	mux.HandleFunc("POST "+resolvePath, c.serve(c.handleResolve))
	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := c.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("confirmation channel stopped", "error", err)
		}
	}()
	return c, nil
}

// ensurePrivateDir creates dir 0700 and refuses a symlink or a directory that
// other users can use.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create confirmation directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("confirmation directory %s is a symbolic link; refusing to use it", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("confirmation path %s is not a directory", dir)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("confirmation directory %s is accessible to other users (mode %v); refusing to use it", dir, fi.Mode().Perm())
	}
	return dirOwnedBy(fi, dir, currentUID())
}

// dirOwnedBy refuses a directory that another user owns: a shared location
// such as /tmp lets someone pre-create it with a mode that looks private and
// then block, or watch, the server. Where the platform reports no owner the
// mode check above stands alone.
func dirOwnedBy(fi os.FileInfo, dir string, uid uint32) error {
	if owner, known := fileOwner(fi); known && owner != uid {
		return fmt.Errorf("confirmation directory %s is owned by uid %d, not by this user (uid %d); refusing to use it", dir, owner, uid)
	}
	return nil
}

func (c *channel) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = c.srv.Shutdown(ctx)
	_ = os.Remove(c.sockPath)
}

// add makes a confirmation visible to the clients of resolver name until ctx
// ends (the confirmation expired).
func (c *channel) add(ctx context.Context, name string, p server.Pending, resolve server.ResolveFunc) {
	c.mu.Lock()
	c.items[name][p.ID] = item{p: p, resolve: resolve}
	c.mu.Unlock()
	go func() {
		<-ctx.Done()
		c.mu.Lock()
		delete(c.items[name], p.ID)
		c.mu.Unlock()
	}()
}

// serve authenticates a request, runs h, and writes the response with its
// server proof. h returns the status and the JSON value to send.
func (c *channel) serve(h func(name string, body []byte) (int, any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		var auth authResult
		if err != nil {
			auth = reject(http.StatusBadRequest, codeMalformed, "the request body is too large")
		} else {
			auth = c.authenticate(r, body)
		}
		status, v := auth.status, any(nil)
		if auth.resolver != "" {
			status, v = h(auth.resolver, body)
		} else {
			e := map[string]string{"error": auth.msg}
			if auth.code != "" {
				e["code"] = auth.code
			}
			v = e
		}
		c.write(w, auth, status, v)
	}
}

// write encodes v as the response, adding the server proof when the request
// passed the form check and a pairing exists.
func (c *channel) write(w http.ResponseWriter, auth authResult, status int, v any) {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(v)
	for k, vals := range auth.header {
		w.Header()[k] = vals
	}
	if auth.key != nil && auth.nonce != "" {
		w.Header().Set(proofHeader, serverProof(auth.key, c.socketName(), auth.nonce, status, buf.Bytes()))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func (c *channel) handlePending(name string, _ []byte) (int, any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]PendingView, 0, len(c.items[name]))
	for _, it := range c.items[name] {
		if it.decided {
			continue
		}
		targets := slices.Clone(it.p.Summary.Targets)
		if targets == nil {
			targets = []server.Target{}
		}
		out = append(out, PendingView{
			ID:            it.p.ID,
			Summary:       it.p.Summary.Text,
			Tool:          it.p.Summary.Tool,
			Class:         it.p.Summary.Class,
			ExpiresAt:     it.p.ExpiresAt,
			NativePrompt:  it.p.NativePrompt,
			PrincipalKind: it.p.Principal.Kind,
			Targets:       targets,
		})
	}
	return http.StatusOK, map[string]any{"pending": out}
}

func (c *channel) handleResolve(name string, raw []byte) (int, any) {
	var body struct {
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.ID == "" {
		return http.StatusBadRequest, errBody("body must be {\"id\": string, \"approve\": bool}")
	}
	c.mu.Lock()
	it, ok := c.items[name][body.ID]
	c.mu.Unlock()
	if !ok {
		return http.StatusNotFound, errBody("no such pending confirmation for this client")
	}
	err := it.resolve(body.Approve)
	if err == nil || errors.Is(err, server.ErrConfirmationResolved) || errors.Is(err, server.ErrConfirmationExpired) {
		// Decided or over: stop listing it to every client of the channel.
		c.forget(body.ID)
	}
	if err != nil {
		code := http.StatusConflict
		if errors.Is(err, server.ErrConfirmationExpired) {
			code = http.StatusGone
		}
		return code, errBody(err.Error())
	}
	return http.StatusOK, map[string]any{"id": body.ID, "approved": body.Approve}
}

// forget hides a decided confirmation from the pending list of every resolver.
func (c *channel) forget(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, m := range c.items {
		if it, ok := m[id]; ok {
			it.decided = true
			m[id] = it
		}
	}
}

// channelResolver is the desktop or cli resolver: it publishes a confirmation
// on the channel and waits for a client to decide.
type channelResolver struct {
	resolverName string
	ch           *channel
}

// Name implements server.Resolver.
func (r *channelResolver) Name() string { return r.resolverName }

// Request implements server.Resolver.
func (r *channelResolver) Request(ctx context.Context, p server.Pending, resolve server.ResolveFunc) error {
	if reason, off := disabledMechanism(r.resolverName); off {
		return fmt.Errorf("%w: disabled after a failed HG-1 check: %s", ErrUnavailable, reason)
	}
	r.ch.add(ctx, r.resolverName, p, resolve)
	return nil
}

// verifier returns the stored key hash, or nil when there is no valid pairing.
func (c *channel) verifier() []byte {
	if c.pairingPath == "" {
		return nil
	}
	v, err := loadVerifier(c.pairingPath)
	if err != nil {
		return nil
	}
	return v
}
