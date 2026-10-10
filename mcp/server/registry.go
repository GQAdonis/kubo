package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callTimeout bounds one tool call against the daemon.
const callTimeout = 60 * time.Second

// toolImpl is the Go side of one catalog tool. Its name must match a catalog
// entry; New refuses to start otherwise.
type toolImpl struct {
	Name string
	// Call decodes the raw arguments (already validated against the input
	// schema), talks to the daemon and returns a value that marshals to the
	// output schema, or an error.
	Call func(ctx context.Context, args json.RawMessage) (any, error)
}

// groupFactory builds the tools of one group for a server.
type groupFactory func(*Server) []toolImpl

var groupFactories []groupFactory

// registerGroup adds a tool group. Each tools_<group>.go file calls it from an
// init function.
func registerGroup(f groupFactory) {
	groupFactories = append(groupFactories, f)
}

// buildHooks run once the SDK server exists, so a tool group can add
// resources or other MCP features next to its tools. A hook decides itself
// whether its group is enabled (see Server.toolRegistered).
var buildHooks []func(*Server)

// registerBuildHook adds a function that build calls after registering tools.
func registerBuildHook(f func(*Server)) {
	buildHooks = append(buildHooks, f)
}

// resultDecorator is implemented by tool outputs that add MCP-level parts to
// their result, such as resource_link content blocks or _meta. Unexported
// fields and methods do not affect the JSON the output marshals to.
type resultDecorator interface {
	decorateResult(*mcp.CallToolResult)
}

// schemas holds the resolved input schema of a tool.
type schemas struct {
	input  *jsonschema.Resolved
	rawIn  json.RawMessage
	rawOut json.RawMessage
}

// loadSchemas bundles and resolves the tool's contract schemas.
func loadSchemas(t contract.Tool) (*schemas, error) {
	rawIn, err := contract.BundleSchema(t.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("tool %s input schema: %w", t.Name, err)
	}
	rawOut, err := contract.BundleSchema(t.OutputSchema)
	if err != nil {
		return nil, fmt.Errorf("tool %s output schema: %w", t.Name, err)
	}
	resolved, err := resolveSchema(rawIn)
	if err != nil {
		return nil, fmt.Errorf("tool %s input schema: %w", t.Name, err)
	}
	return &schemas{input: resolved, rawIn: rawIn, rawOut: rawOut}, nil
}

func resolveSchema(raw []byte) (*jsonschema.Resolved, error) {
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolve schema: %w", err)
	}
	return r, nil
}

// validateArgs checks raw arguments against the input schema.
func (s *schemas) validateArgs(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, invalidArgument("arguments are not valid JSON: " + err.Error())
	}
	if err := s.input.Validate(v); err != nil {
		return nil, invalidArgument(err.Error())
	}
	return raw, nil
}

// mcpTool converts a catalog entry to the SDK's tool description. appViews
// false (tools.app_views, --no-app-views) leaves out the _meta.ui link.
func mcpTool(t contract.Tool, sc *schemas, appViews bool) *mcp.Tool {
	destructive, openWorld := t.Annotations.DestructiveHint, t.Annotations.OpenWorldHint
	var meta mcp.Meta
	if appViews {
		meta = appToolMeta(t.Name)
	}
	return &mcp.Tool{
		Name:         t.Name,
		Title:        t.Title,
		Description:  t.Description,
		InputSchema:  sc.rawIn,
		OutputSchema: sc.rawOut,
		Meta:         meta,
		Annotations: &mcp.ToolAnnotations{
			Title:           t.Title,
			ReadOnlyHint:    t.Annotations.ReadOnlyHint,
			DestructiveHint: &destructive,
			IdempotentHint:  t.Annotations.IdempotentHint,
			OpenWorldHint:   &openWorld,
		},
	}
}

// handler wraps a toolImpl: it validates input, bounds the call, and builds
// the result with structuredContent plus a JSON text fallback, or an isError
// result carrying the contract's ToolError.
func (s *Server) handler(t contract.Tool, sc *schemas, impl toolImpl) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var raw json.RawMessage
		if req != nil && req.Params != nil {
			raw = req.Params.Arguments
		}
		if te := s.egressRefusal(ctx, req, t); te != nil {
			te.Tool = t.Name
			return errorResult(t.Name, te), nil
		}
		args, err := sc.validateArgs(raw)
		if err != nil {
			return errorResult(t.Name, err), nil
		}
		if t.Confirmation != contract.ClassNone {
			// The gate runs before the daemon call and outside its timeout:
			// a person may take up to the confirmation TTL to answer.
			ask, gerr := s.gate.authorize(ctx, req, t, sc, args)
			if gerr != nil {
				gerr.Tool = t.Name
				return errorResult(t.Name, gerr), nil
			}
			if ask != nil {
				return ask, nil
			}
		}
		ctx, cancel := context.WithTimeout(ctx, callTimeout)
		defer cancel()
		out, err := impl.Call(ctx, args)
		if err != nil {
			s.log.Debug("tool call failed", "tool", t.Name, "error", err)
			return errorResult(t.Name, err), nil
		}
		res := successResult(t.Name, out)
		if t.ReturnsUntrustedContent {
			markUntrusted(res)
		}
		return res, nil
	}
}

// untrustedMetaKey is the _meta key that marks a result carrying bytes or
// names controlled by someone other than the operator. The catalog's
// returnsUntrustedContent decides which tools set it.
const untrustedMetaKey = contract.UntrustedContentMetaKey

func markUntrusted(r *mcp.CallToolResult) {
	if r.Meta == nil {
		r.Meta = mcp.Meta{}
	}
	r.Meta[untrustedMetaKey] = true
}

func successResult(tool string, out any) *mcp.CallToolResult {
	text, err := json.Marshal(out)
	if err != nil {
		return errorResult(tool, fmt.Errorf("encode result: %w", err))
	}
	res := &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: out,
		Meta:              mcp.Meta{toolNameMetaKey: tool},
	}
	if d, ok := out.(resultDecorator); ok {
		d.decorateResult(res)
	}
	return res
}

func errorResult(tool string, err error) *mcp.CallToolResult {
	te := toToolError(tool, err)
	return &mcp.CallToolResult{
		IsError:           true,
		Content:           []mcp.Content{&mcp.TextContent{Text: te.Code + ": " + te.Message}},
		StructuredContent: toolErrorResult{Error: te},
		Meta:              mcp.Meta{toolNameMetaKey: tool},
	}
}
