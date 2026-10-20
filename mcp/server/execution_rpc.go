package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// rpcProgress retains only mutation progress, never commands' arguments or
// response bytes. It helps distinguish a rejected command from an unproven
// partial operation when a later command/read-back fails.
type rpcProgress struct {
	mu        sync.Mutex
	succeeded int
	ambiguous bool
}

type rpcProgressKey struct{}

func (p *rpcProgress) record(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.succeeded++
		return
	}
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		p.ambiguous = true
	}
}

func (p *rpcProgress) partialOrAmbiguous() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.succeeded > 0 || p.ambiguous
}

func mutationRPC(command string) bool {
	switch command {
	case "add", "dag/put", "dag/import", "pin/add", "pin/rm", "pin/update",
		"files/write", "files/mkdir", "files/cp", "files/mv", "files/rm",
		"key/gen", "key/rename", "key/rm", "name/publish", "repo/gc", "provide/once", "routing/provide":
		return true
	}
	return false
}

func recordRPCProgress(ctx context.Context, command string, err error) {
	if p, ok := ctx.Value(rpcProgressKey{}).(*rpcProgress); ok && mutationRPC(command) {
		p.record(err)
	}
}

// executionDaemon observes calls without changing their RPC arguments,
// responses, retries or transport. Optional raw/upload operations still fail
// with tool_unavailable if the underlying connection does not support them.
type executionDaemon struct{ Daemon }

func (d *executionDaemon) Call(ctx context.Context, command string, args []string, opts map[string]any, out any) error {
	err := d.Daemon.Call(ctx, command, args, opts, out)
	recordRPCProgress(ctx, command, err)
	return err
}

func (d *executionDaemon) Stream(ctx context.Context, command string, args []string, opts map[string]any, fn func(*json.Decoder) error) error {
	err := d.Daemon.Stream(ctx, command, args, opts, fn)
	recordRPCProgress(ctx, command, err)
	return err
}

func (d *executionDaemon) CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	rd, ok := d.Daemon.(rawDaemon)
	if !ok {
		return &ToolError{Code: CodeToolUnavailable, Message: "this daemon connection cannot send request bodies or read binary responses"}
	}
	err := rd.CallRaw(ctx, command, args, opts, upload, fn)
	recordRPCProgress(ctx, command, err)
	return err
}

func (d *executionDaemon) CallUpload(ctx context.Context, command string, args []string, opts map[string]any, name string, data []byte, fn func(io.Reader) error) error {
	ud, ok := d.Daemon.(uploadDaemon)
	if !ok {
		return &ToolError{Code: CodeToolUnavailable, Message: "this daemon connection cannot send request bodies"}
	}
	err := ud.CallUpload(ctx, command, args, opts, name, data, fn)
	recordRPCProgress(ctx, command, err)
	return err
}
