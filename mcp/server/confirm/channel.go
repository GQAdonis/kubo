package confirm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
//	Authorization: Bearer <pairing key>   the paired IPFS Desktop window
//	X-IPFS-MCP-Confirm: cli               `ipfs mcp confirm --confirm=cli`
//
// A request with neither is rejected, a request with a key that does not match
// the stored verifier is rejected, and the cli form is rejected unless the
// operator started the server with --confirm=cli. The cli form carries no
// secret: it is the explicit headless opt-in, whose only factor is the
// terminal the person types into (see MECHANISMS.md for its limit).
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

	ln  net.Listener
	srv *http.Server

	mu    sync.Mutex
	items map[string]map[string]item // resolver name -> id -> item
}

// newChannel listens on a private socket. pairingPath may be empty (desktop
// requests are then all rejected).
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
	mux.HandleFunc("GET "+pendingPath, c.handlePending)
	mux.HandleFunc("POST "+resolvePath, c.handleResolve)
	c.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := c.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("confirmation channel stopped", "error", err)
		}
	}()
	return c, nil
}

// ensurePrivateDir creates dir 0700 and refuses one that other users can use.
func ensurePrivateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create confirmation directory: %w", err)
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
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

// authenticate returns the resolver a request speaks for, or an HTTP status.
func (c *channel) authenticate(r *http.Request) (string, int, string) {
	if h := r.Header.Get("Authorization"); h != "" {
		key, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || !keyMatches(c.verifier(), key) {
			return "", http.StatusForbidden, "the pairing key is not valid"
		}
		return server.ResolverDesktop, 0, ""
	}
	if r.Header.Get(cliHeader) == cliValue {
		if !c.cliOn {
			return "", http.StatusForbidden, "cli confirmation is not enabled on this server; enable it with --confirm=cli or confirmation.cli"
		}
		return server.ResolverCLI, 0, ""
	}
	return "", http.StatusUnauthorized, "a pairing key is required"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (c *channel) handlePending(w http.ResponseWriter, r *http.Request) {
	name, status, msg := c.authenticate(r)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	c.mu.Lock()
	out := make([]PendingView, 0, len(c.items[name]))
	for _, it := range c.items[name] {
		out = append(out, PendingView{
			ID:            it.p.ID,
			Summary:       it.p.Summary.Text,
			Tool:          it.p.Summary.Tool,
			Class:         it.p.Summary.Class,
			ExpiresAt:     it.p.ExpiresAt,
			NativePrompt:  it.p.NativePrompt,
			PrincipalKind: it.p.Principal.Kind,
		})
	}
	c.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"pending": out})
}

func (c *channel) handleResolve(w http.ResponseWriter, r *http.Request) {
	name, status, msg := c.authenticate(r)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	var body struct {
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || body.ID == "" {
		writeError(w, http.StatusBadRequest, "body must be {\"id\": string, \"approve\": bool}")
		return
	}
	c.mu.Lock()
	it, ok := c.items[name][body.ID]
	c.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, "no such pending confirmation for this client")
		return
	}
	if err := it.resolve(body.Approve); err != nil {
		code := http.StatusConflict
		if errors.Is(err, server.ErrConfirmationExpired) {
			code = http.StatusGone
		}
		writeError(w, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": body.ID, "approved": body.Approve})
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
