//go:build !noagent

package server

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	untrustedTag    = "untrusted-content"
	truncatedMarker = "\n[truncated: the rest of this result was not sent]"
)

// closingTag matches a closing untrusted-content tag in any case and with
// whitespace around the slash or before the name, the forms a lenient reader
// would still take as the end of the frame.
var closingTag = regexp.MustCompile(`(?i)<\s*/\s*` + regexp.QuoteMeta(untrustedTag))

// frame bounds a result and, when it can carry text written by someone else,
// wraps it in delimiters the system prompt names as data. A closing tag inside
// the text is neutralised so content cannot end the frame early.
func frame(tool, text string, untrusted bool) string {
	if len(text) > maxToolResultBytes {
		cut := maxToolResultBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + truncatedMarker
	}
	if !untrusted {
		return text
	}
	text = closingTag.ReplaceAllString(text, `<\/`+untrustedTag)
	return "<" + untrustedTag + " tool=\"" + tool + "\">\n" + text + "\n</" + untrustedTag + ">"
}

// tokensOf counts what a model call cost: the backend's report, or an estimate
// of four bytes per token over the request and the reply.
func tokensOf(req []ChatMessage, res *ChatResult) int {
	if res.UsageReported && res.Usage.TotalTokens > 0 {
		return res.Usage.TotalTokens
	}
	n := len(res.Content)
	for _, tc := range res.ToolCalls {
		n += len(tc.Function.Name) + len(tc.Function.Arguments)
	}
	for _, m := range req {
		n += len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return n/4 + 1
}

func interruptOf(tc ChatToolCall, t contract.Tool, c *ConfirmationRequired) *Interrupt {
	return &Interrupt{
		ID:                   "int-" + c.ConfirmationID,
		ConfirmationID:       c.ConfirmationID,
		ToolCallID:           tc.ID,
		Tool:                 t.Name,
		Message:              c.Summary.Text,
		Class:                t.Confirmation,
		ExpiresAt:            c.ExpiresAt,
		RequiresNativePrompt: c.RequiresNativePrompt,
		Resolvers:            append([]string(nil), c.Resolvers...),
		EffectDigest:         c.EffectDigest,
		Destination:          cloneDestination(c.Destination),
	}
}

// errorOf extracts the ToolError of an isError result produced by this
// package.
func errorOf(res *mcp.CallToolResult) *ToolError {
	if r, ok := res.StructuredContent.(toolErrorResult); ok {
		return r.Error
	}
	return nil
}

// resultText renders a success result for the model: the JSON text of the
// structured result and a line per resource_link, never inlined blob bytes.
func resultText(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			parts = append(parts, v.Text)
		case *mcp.ResourceLink:
			parts = append(parts, "[resource_link] "+v.URI)
		}
	}
	return strings.Join(parts, "\n")
}
