package server

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

func init() { registerGroup(namesTools) }

// Tools of the names group. The group has no key export or import: private
// key material never leaves the node through this server.
func namesTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "ipns_publish", Call: s.ipnsPublish},
		{Name: "ipns_resolve", Call: s.ipnsResolve},
		{Name: "key_list", Call: s.keyList},
		{Name: "key_gen", Call: s.keyGen},
		{Name: "key_rename", Call: s.keyRename},
		{Name: "key_rm", Call: s.keyRm},
	}
}

const (
	// defaultIPNSKey is the key ipns_publish signs with when the caller names
	// none; it is the node's own identity key.
	defaultIPNSKey = "self"
	// defaultIPNSLifetime is the contract default record lifetime.
	defaultIPNSLifetime = "48h"
	// defaultRSAKeyBits is used when an rsa key is requested without a size.
	defaultRSAKeyBits = 2048
)

// codeAlreadyExists is the contract error code for a name that is taken.
const codeAlreadyExists = "already_exists"

// keyError maps the daemon's keystore errors to contract error codes and
// leaves other errors unchanged.
func keyError(err error) error {
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		return err
	}
	msg := strings.ToLower(rpcErr.Message)
	switch {
	case strings.Contains(msg, "already exists"):
		return &ToolError{Code: codeAlreadyExists, Message: rpcErr.Message, Details: map[string]any{"rpc_command": rpcErr.Command}}
	case strings.Contains(msg, "no key") && strings.Contains(msg, "found"):
		return &ToolError{Code: CodeNotFound, Message: rpcErr.Message, Details: map[string]any{"rpc_command": rpcErr.Command}}
	}
	return err
}

// ipfsContentPath turns a bare CID (with an optional sub-path) into an
// /ipfs/ path and leaves rooted paths alone.
func ipfsContentPath(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/ipfs/" + p
}

// ipnsNamePath turns a bare IPNS name into an /ipns/ path.
func ipnsNamePath(n string) string {
	if strings.HasPrefix(n, "/ipns/") {
		return n
	}
	return "/ipns/" + strings.TrimPrefix(n, "/")
}

type ipnsPublishInput struct {
	Path     string `json:"path"`
	Key      string `json:"key"`
	Lifetime string `json:"lifetime"`
	TTL      string `json:"ttl"`
}

type ipnsPublishOutput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ipnsPublishResponse is the name/publish response.
type ipnsPublishResponse struct {
	Name  string
	Value string
}

// ipnsPublish signs with the key the caller chose, or the node's own key when
// none is named, and uses the lifetime and TTL from the arguments. The node
// decides whether it can reach the network: an offline node refuses to
// publish, so nothing is stored or announced behind the caller's back.
func (s *Server) ipnsPublish(ctx context.Context, raw json.RawMessage) (any, error) {
	var in ipnsPublishInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	key := in.Key
	if key == "" {
		key = defaultIPNSKey
	}
	lifetime := in.Lifetime
	if lifetime == "" {
		lifetime = defaultIPNSLifetime
	}
	opts := map[string]any{"key": key, "lifetime": lifetime}
	if in.TTL != "" {
		opts["ttl"] = in.TTL
	}
	var resp ipnsPublishResponse
	if err := s.daemon.Call(ctx, "name/publish", []string{ipfsContentPath(in.Path)}, opts, &resp); err != nil {
		return nil, keyError(err)
	}
	return ipnsPublishOutput(resp), nil
}

type ipnsResolveInput struct {
	Name      string `json:"name"`
	Recursive *bool  `json:"recursive"`
	Nocache   bool   `json:"nocache"`
}

type ipnsResolveOutput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ipnsResolveResponse is the name/resolve response.
type ipnsResolveResponse struct {
	Path string
}

func (s *Server) ipnsResolve(ctx context.Context, raw json.RawMessage) (any, error) {
	var in ipnsResolveInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	opts := map[string]any{"recursive": boolOr(in.Recursive, true), "nocache": in.Nocache}
	var resp ipnsResolveResponse
	if err := s.daemon.Call(ctx, "name/resolve", []string{ipnsNamePath(in.Name)}, opts, &resp); err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && strings.Contains(rpcErr.Message, "could not resolve name") {
			return nil, &ToolError{Code: CodeNotFound, Message: rpcErr.Message, Details: map[string]any{"rpc_command": rpcErr.Command}}
		}
		return nil, err
	}
	return ipnsResolveOutput{Name: in.Name, Path: resp.Path}, nil
}

// keyInfo is one key of the key/list, key/gen and key/rm responses.
type keyInfo struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// keyEntry is the daemon's JSON for a key.
type keyEntry struct {
	Name string
	Id   string
}

type keyListOutput struct {
	Keys []keyInfo `json:"keys"`
}

// keyList lists names and IPNS key IDs only. key/list never carries private
// key material.
func (s *Server) keyList(ctx context.Context, _ json.RawMessage) (any, error) {
	var resp struct{ Keys []keyEntry }
	if err := s.daemon.Call(ctx, "key/list", nil, nil, &resp); err != nil {
		return nil, err
	}
	return keyListOutput{Keys: sortedKeyInfos(resp.Keys)}, nil
}

func sortedKeyInfos(entries []keyEntry) []keyInfo {
	keys := make([]keyInfo, 0, len(entries))
	for _, k := range entries {
		keys = append(keys, keyInfo{Name: k.Name, ID: k.Id})
	}
	slices.SortFunc(keys, func(a, b keyInfo) int { return strings.Compare(a.Name, b.Name) })
	return keys
}

type keyGenInput struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

type keyGenOutput struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

func (s *Server) keyGen(ctx context.Context, raw json.RawMessage) (any, error) {
	var in keyGenInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	typ := in.Type
	if typ == "" {
		typ = "ed25519"
	}
	opts := map[string]any{"type": typ}
	if typ == "rsa" {
		size := in.Size
		if size == 0 {
			size = defaultRSAKeyBits
		}
		opts["size"] = size
	}
	var resp keyEntry
	if err := s.daemon.Call(ctx, "key/gen", []string{in.Name}, opts, &resp); err != nil {
		return nil, keyError(err)
	}
	return keyGenOutput{Name: resp.Name, ID: resp.Id}, nil
}

type keyRenameInput struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
}

type keyRenameOutput struct {
	OldName string `json:"old_name"`
	NewName string `json:"new_name"`
	ID      string `json:"id"`
}

// keyRename never overwrites: force is sent as false so an existing key with
// the new name stays untouched and the call fails.
func (s *Server) keyRename(ctx context.Context, raw json.RawMessage) (any, error) {
	var in keyRenameInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	var resp struct {
		Was string
		Now string
		Id  string
	}
	if err := s.daemon.Call(ctx, "key/rename", []string{in.OldName, in.NewName}, map[string]any{"force": false}, &resp); err != nil {
		return nil, keyError(err)
	}
	return keyRenameOutput{OldName: in.OldName, NewName: in.NewName, ID: resp.Id}, nil
}

type keyRmInput struct {
	Name string `json:"name"`
}

type keyRmOutput struct {
	Removed []keyInfo `json:"removed"`
}

func (s *Server) keyRm(ctx context.Context, raw json.RawMessage) (any, error) {
	var in keyRmInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	var resp struct{ Keys []keyEntry }
	if err := s.daemon.Call(ctx, "key/rm", []string{in.Name}, nil, &resp); err != nil {
		return nil, keyError(err)
	}
	return keyRmOutput{Removed: sortedKeyInfos(resp.Keys)}, nil
}
