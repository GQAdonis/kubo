package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/ipfs/kubo/client/rpc"
	ma "github.com/multiformats/go-multiaddr"
)

// Daemon is the narrow view of a Kubo daemon that tool handlers use. The
// production implementation is RPCDaemon over client/rpc; tests use a fake.
type Daemon interface {
	// Call runs one /api/v0 command and decodes its JSON response into out
	// (nil discards the output). Command is the RPC path such as "pin/ls".
	Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error
	// Stream runs one /api/v0 command whose response is a stream of JSON
	// values and hands the decoder to fn. fn may return ErrStopStream to end
	// the stream early; that is not an error. The command is cancelled when
	// Stream returns.
	Stream(ctx context.Context, command string, args []string, opts map[string]any, fn func(*json.Decoder) error) error
	// Endpoint describes the daemon API address for messages.
	Endpoint() string
}

// ErrStopStream is returned by a Stream callback to stop reading early.
var ErrStopStream = errors.New("stop reading the stream")

// RPCDaemon is a Daemon backed by client/rpc.
type RPCDaemon struct {
	api      *rpc.HttpApi
	endpoint string
}

// NewRPCDaemon wraps an existing client/rpc API. The endpoint is used in
// messages only.
func NewRPCDaemon(api *rpc.HttpApi, endpoint string) *RPCDaemon {
	return &RPCDaemon{api: api, endpoint: endpoint}
}

// API returns the underlying client/rpc API for tools that need CoreAPI.
func (d *RPCDaemon) API() *rpc.HttpApi { return d.api }

// Endpoint implements Daemon.
func (d *RPCDaemon) Endpoint() string { return d.endpoint }

// Call implements Daemon.
func (d *RPCDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	rb := d.api.Request(command, args...)
	for k, v := range opts {
		rb = rb.Option(k, v)
	}
	err := rb.Exec(ctx, out)
	if err == nil {
		return nil
	}
	return d.mapError(ctx, command, err)
}

// Stream implements Daemon.
func (d *RPCDaemon) Stream(ctx context.Context, command string, args []string, opts map[string]any, fn func(*json.Decoder) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rb := d.api.Request(command, args...)
	for k, v := range opts {
		rb = rb.Option(k, v)
	}
	resp, err := rb.Send(ctx)
	if err != nil {
		return d.mapError(ctx, command, err)
	}
	defer resp.Close()
	if resp.Error != nil {
		return d.mapError(ctx, command, resp.Error)
	}
	if err := fn(json.NewDecoder(resp.Output)); err != nil && !errors.Is(err, ErrStopStream) {
		return err
	}
	return nil
}

// mapError turns a client/rpc error into the package's error types.
func (d *RPCDaemon) mapError(ctx context.Context, command string, err error) error {
	var ce *cmds.Error
	if errors.As(err, &ce) {
		return &RPCError{Command: command, Message: ce.Message, Code: ce.Code}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("%w at %s: %w", ErrDaemonUnreachable, d.endpoint, err)
}

// DiscoverOptions says where the daemon API is.
type DiscoverOptions struct {
	// API is a multiaddr (/ip4/127.0.0.1/tcp/5001) or an http(s) URL. Empty
	// means read the api file under IPFSPath.
	API string
	// IPFSPath is the repo directory whose api file names a running daemon.
	// Empty means $IPFS_PATH, then ~/.ipfs. The directory is only read for
	// its api file; the repo is never opened or locked.
	IPFSPath string
	// Authorization is the full Authorization header value, for example
	// "Bearer token". Use config.ConvertAuthSecret to build it from an
	// --api-auth secret. It is never logged.
	Authorization string
}

// Discover builds a client for the daemon API named by opts. It does not
// contact the daemon.
func Discover(opts DiscoverOptions) (*RPCDaemon, error) {
	var api *rpc.HttpApi
	var endpoint string
	var err error
	switch {
	case strings.HasPrefix(opts.API, "http://") || strings.HasPrefix(opts.API, "https://"):
		endpoint = redactEndpoint(opts.API)
		api, err = rpc.NewURLApiWithClient(opts.API, &http.Client{Transport: &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
		}})
	case opts.API != "":
		var addr ma.Multiaddr
		addr, err = ma.NewMultiaddr(opts.API)
		if err != nil {
			return nil, fmt.Errorf("--api %q is neither a multiaddr nor an http(s) URL: %w", opts.API, err)
		}
		endpoint = addr.String()
		api, err = rpc.NewApi(addr)
	default:
		dir := opts.IPFSPath
		if dir == "" {
			dir = os.Getenv(rpc.EnvDir)
		}
		if dir == "" {
			dir = rpc.DefaultPathRoot
		}
		var addr ma.Multiaddr
		addr, err = rpc.ApiAddr(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: no api file at %s and no --api given; start `ipfs daemon` or pass --api", ErrDaemonUnreachable, filepath.Join(dir, rpc.DefaultApiFile))
			}
			return nil, fmt.Errorf("read api file in %s: %w", dir, err)
		}
		endpoint = addr.String()
		api, err = rpc.NewApi(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("create RPC client for %s: %w", endpoint, err)
	}
	if opts.Authorization != "" {
		api.Headers.Set("Authorization", opts.Authorization)
	}
	return NewRPCDaemon(api, endpoint), nil
}

// redactEndpoint drops the userinfo of an http(s) URL so a secret given as
// http://user:pass@host never reaches a log line or an error message.
func redactEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		// The text may still carry a secret; say nothing about it.
		return "(unparsable URL)"
	}
	u.User = nil
	return u.String()
}

// DiscoveringDaemon is a Daemon for a daemon whose API address is found in the
// repo's api file. It reads the file on first use and again after a call finds
// the daemon unreachable, so a daemon that starts after this process, or
// restarts on another port, is found without a restart. With an explicit API
// address there is nothing to rediscover; use Discover.
type DiscoveringDaemon struct {
	opts DiscoverOptions

	mu  sync.Mutex
	cur *RPCDaemon
}

// NewDiscoveringDaemon returns a DiscoveringDaemon for opts, which must not
// name an API address. It does not read the api file or contact the daemon.
func NewDiscoveringDaemon(opts DiscoverOptions) *DiscoveringDaemon {
	return &DiscoveringDaemon{opts: opts}
}

// current returns the daemon the api file names now, reading the file when no
// daemon is cached.
func (d *DiscoveringDaemon) current() (*RPCDaemon, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cur != nil {
		return d.cur, nil
	}
	cur, err := Discover(d.opts)
	if err != nil {
		return nil, err
	}
	d.cur = cur
	return cur, nil
}

// forget drops cur when err says the daemon is unreachable, so the next call
// re-reads the api file.
func (d *DiscoveringDaemon) forget(cur *RPCDaemon, err error) error {
	if errors.Is(err, ErrDaemonUnreachable) {
		d.mu.Lock()
		if d.cur == cur {
			d.cur = nil
		}
		d.mu.Unlock()
	}
	return err
}

// Endpoint implements Daemon.
func (d *DiscoveringDaemon) Endpoint() string {
	d.mu.Lock()
	cur := d.cur
	d.mu.Unlock()
	if cur != nil {
		return cur.Endpoint()
	}
	return "the api file in " + d.apiDir()
}

func (d *DiscoveringDaemon) apiDir() string {
	dir := d.opts.IPFSPath
	if dir == "" {
		dir = os.Getenv(rpc.EnvDir)
	}
	if dir == "" {
		dir = rpc.DefaultPathRoot
	}
	return dir
}

// Call implements Daemon.
func (d *DiscoveringDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	cur, err := d.current()
	if err != nil {
		return err
	}
	return d.forget(cur, cur.Call(ctx, command, args, opts, out))
}

// Stream implements Daemon.
func (d *DiscoveringDaemon) Stream(ctx context.Context, command string, args []string, opts map[string]any, fn func(*json.Decoder) error) error {
	cur, err := d.current()
	if err != nil {
		return err
	}
	return d.forget(cur, cur.Stream(ctx, command, args, opts, fn))
}

// CallRaw implements rawDaemon.
func (d *DiscoveringDaemon) CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	cur, err := d.current()
	if err != nil {
		return err
	}
	return d.forget(cur, cur.CallRaw(ctx, command, args, opts, upload, fn))
}

// CallUpload implements uploadDaemon.
func (d *DiscoveringDaemon) CallUpload(ctx context.Context, command string, args []string, opts map[string]any, name string, data []byte, fn func(io.Reader) error) error {
	cur, err := d.current()
	if err != nil {
		return err
	}
	return d.forget(cur, cur.CallUpload(ctx, command, args, opts, name, data, fn))
}

var (
	_ rawDaemon    = (*DiscoveringDaemon)(nil)
	_ uploadDaemon = (*DiscoveringDaemon)(nil)
)
