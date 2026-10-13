package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
		endpoint = opts.API
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
