package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ipfs/go-cid"
	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	multicodec "github.com/multiformats/go-multicodec"
)

func init() { registerGroup(dagTools) }

func dagTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "dag_get", Call: s.dagGet},
		{Name: "dag_stat", Call: s.dagStat},
		{Name: "dag_put", Call: s.dagPut},
		{Name: "car_import", Call: s.carImport},
		{Name: "car_export", Call: s.carExport},
	}
}

const (
	// defaultDagInlineBytes is the contract default max_inline_bytes of dag_get.
	defaultDagInlineBytes = 256 * 1024
	// defaultCarInlineBytes is the contract default max_inline_bytes of car_export.
	defaultCarInlineBytes = 1024 * 1024
	// maxCarImportBytes is the decoded size cap of car_import.
	maxCarImportBytes = 8 * 1024 * 1024
	// maxSmallResponseBytes bounds the daemon's answer to dag_put and
	// car_import, which is a few lines of JSON.
	maxSmallResponseBytes = 1 << 20

	codeSizeLimitExceeded = CodeSizeLimitExceeded
)

// rawDaemon is the optional capability of a Daemon to send a request body
// and to hand the raw, possibly binary, response to the caller. dag_put,
// car_import and car_export need it; the production daemon has it.
type rawDaemon interface {
	// CallRaw runs one /api/v0 command. A non-nil upload is sent as the
	// request's single multipart file. fn reads the response body; the
	// command is cancelled, without draining the body, when fn returns.
	CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error
}

// CallRaw implements rawDaemon.
func (d *RPCDaemon) CallRaw(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rb := d.api.Request(command, args...)
	for k, v := range opts {
		rb = rb.Option(k, v)
	}
	if upload != nil {
		rb = rb.FileBody(bytes.NewReader(upload))
	}
	resp, err := rb.Send(ctx)
	if err != nil {
		return d.mapError(ctx, command, err)
	}
	// Cancel before closing: Close would drain a large body to its end.
	defer func() { _ = resp.Cancel() }()
	if resp.Error != nil {
		return d.mapError(ctx, command, resp.Error)
	}
	return fn(resp.Output)
}

// rawCall runs a raw command or reports that the daemon connection cannot.
func (s *Server) rawCall(ctx context.Context, command string, args []string, opts map[string]any, upload []byte, fn func(io.Reader) error) error {
	rd, ok := s.daemon.(rawDaemon)
	if !ok {
		return &ToolError{Code: CodeToolUnavailable, Message: "this daemon connection cannot send request bodies or read binary responses"}
	}
	return rd.CallRaw(ctx, command, args, opts, upload, fn)
}

// resourceRef is ResourceRef in the contract's common definitions.
type resourceRef struct {
	URI      string `json:"uri"`
	MimeType string `json:"mime_type,omitempty"`
	Size     *int64 `json:"size,omitempty"`
}

// link returns the resource_link content block for the reference.
func (r *resourceRef) link(name string) *mcp.ResourceLink {
	return &mcp.ResourceLink{URI: r.URI, Name: name, MIMEType: r.MimeType, Size: r.Size}
}

// resourceURI returns the ipfs:// or ipns:// URI of a content path, or of
// the fallback CID when the path has another root.
func resourceURI(contentPath, fallbackCID string) string {
	switch {
	case strings.HasPrefix(contentPath, "/ipfs/"):
		return "ipfs://" + strings.TrimPrefix(contentPath, "/ipfs/")
	case strings.HasPrefix(contentPath, "/ipns/"):
		return "ipns://" + strings.TrimPrefix(contentPath, "/ipns/")
	case strings.HasPrefix(contentPath, "/"):
		return "ipfs://" + fallbackCID
	}
	return "ipfs://" + contentPath
}

// cidString reads a CID the daemon wrote either as a string or as the
// dag-json link {"/": "..."}.
func cidString(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var link struct {
		Slash string `json:"/"`
	}
	if json.Unmarshal(raw, &link) == nil {
		return link.Slash
	}
	return ""
}

type dagGetInput struct {
	Path           string `json:"path"`
	MaxInlineBytes *int   `json:"max_inline_bytes"`
}

type dagGetOutput struct {
	Path      string          `json:"path"`
	CID       string          `json:"cid"`
	Codec     string          `json:"codec,omitempty"`
	Node      json.RawMessage `json:"node,omitempty"`
	Resource  *resourceRef    `json:"resource,omitempty"`
	Truncated bool            `json:"truncated"`
}

// decorateResult adds the resource_link block of a node too large to inline.
func (o dagGetOutput) decorateResult(res *mcp.CallToolResult) {
	if o.Resource != nil {
		res.Content = append(res.Content, o.Resource.link(o.CID))
	}
}

// dagGet returns the node as dag-json up to max_inline_bytes. A larger node
// is not inlined: the result carries a resource link and truncated=true.
func (s *Server) dagGet(ctx context.Context, raw json.RawMessage) (any, error) {
	var in dagGetInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	limit := defaultDagInlineBytes
	if in.MaxInlineBytes != nil {
		limit = *in.MaxInlineBytes
	}
	p := ipfsContentPath(in.Path)

	nodeCID, err := s.resolveNodeCID(ctx, p)
	if err != nil {
		return nil, err
	}

	var body []byte
	err = s.rawCall(ctx, "dag/get", []string{p}, map[string]any{"output-codec": "dag-json"}, nil, func(r io.Reader) error {
		var rerr error
		body, rerr = io.ReadAll(io.LimitReader(r, int64(limit)+1))
		return rerr
	})
	if err != nil {
		return nil, err
	}

	out := dagGetOutput{Path: in.Path, CID: nodeCID, Codec: codecName(nodeCID)}
	if len(body) > limit {
		out.Truncated = true
		out.Resource = &resourceRef{URI: resourceURI(p, nodeCID)}
		return out, nil
	}
	body = bytes.TrimSpace(body)
	if !json.Valid(body) {
		return nil, fmt.Errorf("dag/get returned a node that is not dag-json for %s", p)
	}
	out.Node = body
	return out, nil
}

// resolveNodeCID returns the CID of the node a content path addresses. A
// daemon connection whose API authorization does not allow dag/resolve (the
// contract lists only dag/get for dag_get) can still serve a bare
// /ipfs/<cid> path, where the answer is the path's own CID.
func (s *Server) resolveNodeCID(ctx context.Context, p string) (string, error) {
	var resolved struct {
		Cid json.RawMessage
	}
	err := s.daemon.Call(ctx, "dag/resolve", []string{p}, nil, &resolved)
	if err != nil {
		if bare, ok := bareIPFSCID(p); ok && isForbidden(err) {
			return bare, nil
		}
		return "", err
	}
	c := cidString(resolved.Cid)
	if c == "" {
		return "", fmt.Errorf("dag/resolve returned no CID for %s", p)
	}
	return c, nil
}

// bareIPFSCID returns the CID of an /ipfs/<cid> path without a sub-path.
func bareIPFSCID(p string) (string, bool) {
	rest, ok := strings.CutPrefix(p, "/ipfs/")
	if !ok || rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	if _, err := cid.Decode(rest); err != nil {
		return "", false
	}
	return rest, true
}

// isForbidden reports whether the daemon refused the command for lack of
// API authorization.
func isForbidden(err error) bool {
	var rpcErr *RPCError
	return errors.As(err, &rpcErr) && rpcErr.Code == cmds.ErrForbidden
}

// codecName returns the multicodec name of a CID's codec, or empty.
func codecName(c string) string {
	parsed, err := cid.Decode(c)
	if err != nil {
		return ""
	}
	return multicodec.Code(parsed.Prefix().Codec).String()
}

type dagStatInput struct {
	Paths []string `json:"paths"`
}

type dagStatEntry struct {
	CID       string `json:"cid"`
	Size      uint64 `json:"size"`
	NumBlocks int64  `json:"num_blocks"`
}

type dagStatOutput struct {
	DAGs         []dagStatEntry `json:"dags"`
	TotalSize    uint64         `json:"total_size"`
	UniqueBlocks int            `json:"unique_blocks"`
	SharedSize   uint64         `json:"shared_size"`
	Ratio        float64        `json:"ratio"`
}

// dagStatResponse is the dag/stat summary.
type dagStatResponse struct {
	UniqueBlocks int
	TotalSize    uint64
	SharedSize   uint64
	Ratio        float64
	DagStats     []struct {
		Cid       json.RawMessage
		Size      uint64
		NumBlocks int64
	}
}

func (s *Server) dagStat(ctx context.Context, raw json.RawMessage) (any, error) {
	var in dagStatInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	paths := make([]string, 0, len(in.Paths))
	for _, p := range in.Paths {
		paths = append(paths, ipfsContentPath(p))
	}
	var resp dagStatResponse
	if err := s.daemon.Call(ctx, "dag/stat", paths, map[string]any{"progress": false}, &resp); err != nil {
		return nil, err
	}
	out := dagStatOutput{
		DAGs:         make([]dagStatEntry, 0, len(resp.DagStats)),
		TotalSize:    resp.TotalSize,
		UniqueBlocks: resp.UniqueBlocks,
		SharedSize:   resp.SharedSize,
		Ratio:        resp.Ratio,
	}
	for _, d := range resp.DagStats {
		out.DAGs = append(out.DAGs, dagStatEntry{CID: cidString(d.Cid), Size: d.Size, NumBlocks: d.NumBlocks})
	}
	return out, nil
}

type dagPutInput struct {
	Data       string `json:"data"`
	StoreCodec string `json:"store_codec"`
	Pin        bool   `json:"pin"`
}

type dagPutOutput struct {
	CID    string `json:"cid"`
	Pinned bool   `json:"pinned"`
}

// dagPut stores one node. The caller states pin explicitly; the hash
// function is the node's own import default, so the CID recipe is not
// changed by this tool.
func (s *Server) dagPut(ctx context.Context, raw json.RawMessage) (any, error) {
	var in dagPutInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	codec := in.StoreCodec
	if codec == "" {
		codec = "dag-cbor"
	}
	if !json.Valid([]byte(in.Data)) {
		return nil, invalidArgument("data is not valid dag-json text")
	}
	opts := map[string]any{"input-codec": "dag-json", "store-codec": codec, "pin": in.Pin}

	var out dagPutOutput
	err := s.rawCall(ctx, "dag/put", nil, opts, []byte(in.Data), func(r io.Reader) error {
		var resp struct{ Cid json.RawMessage }
		dec := json.NewDecoder(io.LimitReader(r, maxSmallResponseBytes))
		if err := dec.Decode(&resp); err != nil {
			return fmt.Errorf("decode dag/put response: %w", err)
		}
		out.CID = cidString(resp.Cid)
		// The daemon emits the CID before it commits the batch (and pins).
		// Read to EOF so a failure after the CID, which the daemon reports
		// in the stream trailer, is surfaced here and not lost to the
		// cancel that follows this function.
		if _, err := io.Copy(io.Discard, io.LimitReader(r, maxSmallResponseBytes)); err != nil {
			return fmt.Errorf("dag/put failed after emitting %s: %w", out.CID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if out.CID == "" {
		return nil, errors.New("dag/put returned no CID")
	}
	// Reached only after the whole stream succeeded, so the node is
	// committed and, when requested, pinned.
	out.Pinned = in.Pin
	return out, nil
}

type carImportInput struct {
	CAR      string `json:"car"`
	PinRoots bool   `json:"pin_roots"`
}

type carImportRoot struct {
	CID      string `json:"cid"`
	Pinned   bool   `json:"pinned"`
	PinError string `json:"pin_error,omitempty"`
}

type carImportOutput struct {
	Roots      []carImportRoot `json:"roots"`
	BlockCount int             `json:"block_count"`
}

// carImport imports a CAR given as base64 data in the arguments, never from a
// local path. Roots are pinned only when pin_roots is true. The import does
// not announce its content: the fast-provide options are switched off, so
// the node's normal Provide.Strategy is the only thing that can announce it.
func (s *Server) carImport(ctx context.Context, raw json.RawMessage) (any, error) {
	var in carImportInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if base64.StdEncoding.DecodedLen(len(in.CAR)) > maxCarImportBytes+3 {
		return nil, carTooLarge()
	}
	car, err := base64.StdEncoding.DecodeString(in.CAR)
	if err != nil {
		return nil, invalidArgument("car is not valid base64: " + err.Error())
	}
	if len(car) > maxCarImportBytes {
		return nil, carTooLarge()
	}
	if len(car) == 0 {
		return nil, invalidArgument("car is empty")
	}

	opts := map[string]any{
		"pin-roots":         in.PinRoots,
		"stats":             true,
		"fast-provide-root": false,
		"fast-provide-dag":  false,
	}
	out := carImportOutput{Roots: []carImportRoot{}}
	err = s.rawCall(ctx, "dag/import", nil, opts, car, func(r io.Reader) error {
		dec := json.NewDecoder(io.LimitReader(r, maxSmallResponseBytes))
		for {
			var line struct {
				Root *struct {
					Cid         json.RawMessage
					PinErrorMsg string
				}
				Stats *struct{ BlockCount int }
			}
			if err := dec.Decode(&line); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("decode dag/import response: %w", err)
			}
			if line.Root != nil {
				out.Roots = append(out.Roots, carImportRoot{
					CID:      cidString(line.Root.Cid),
					Pinned:   in.PinRoots && line.Root.PinErrorMsg == "",
					PinError: line.Root.PinErrorMsg,
				})
			}
			if line.Stats != nil {
				out.BlockCount = line.Stats.BlockCount
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func carTooLarge() *ToolError {
	return &ToolError{
		Code:    codeSizeLimitExceeded,
		Message: fmt.Sprintf("the CAR is larger than %d bytes; import smaller CARs, or use the node's own `ipfs dag import`", maxCarImportBytes),
	}
}

type carExportInput struct {
	Root           string `json:"root"`
	MaxInlineBytes *int   `json:"max_inline_bytes"`
}

type carExportOutput struct {
	Root      string       `json:"root"`
	Size      *int64       `json:"size,omitempty"`
	CarBase64 string       `json:"car_base64,omitempty"`
	Resource  *resourceRef `json:"resource,omitempty"`
	Inline    bool         `json:"inline"`
}

// decorateResult adds the resource_link block of a CAR too large to inline.
func (o carExportOutput) decorateResult(res *mcp.CallToolResult) {
	if o.Resource != nil {
		res.Content = append(res.Content, o.Resource.link(o.Root))
	}
}

// carExport returns the CAR inline when it fits in max_inline_bytes. A larger
// CAR is never held in memory or sent: reading stops one byte past the cap
// and the result is a resource link.
func (s *Server) carExport(ctx context.Context, raw json.RawMessage) (any, error) {
	var in carExportInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	limit := defaultCarInlineBytes
	if in.MaxInlineBytes != nil {
		limit = *in.MaxInlineBytes
	}
	var car []byte
	err := s.rawCall(ctx, "dag/export", []string{in.Root}, map[string]any{"progress": false}, nil, func(r io.Reader) error {
		var rerr error
		car, rerr = io.ReadAll(io.LimitReader(r, int64(limit)+1))
		return rerr
	})
	if err != nil {
		return nil, err
	}
	if len(car) > limit {
		return carExportOutput{
			Root:     in.Root,
			Resource: &resourceRef{URI: "ipfs://" + in.Root},
		}, nil
	}
	size := int64(len(car))
	return carExportOutput{
		Root:      in.Root,
		Size:      &size,
		CarBase64: base64.StdEncoding.EncodeToString(car),
		Inline:    true,
	}, nil
}
