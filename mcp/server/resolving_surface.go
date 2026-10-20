package server

import (
	"encoding/json"
	"errors"
)

// ResolvingSurface is safe display attribution, never identity or permission.
// Trusted adapters select the kind; only chat/inbox may carry a client hint.
type ResolvingSurface struct {
	Kind     string `json:"kind"`
	Instance string `json:"instance,omitempty"`
}

// ErrInvalidResolvingSurface indicates malformed or mechanism-inconsistent
// attribution. A surface string cannot select a human-presence resolver.
var ErrInvalidResolvingSurface = errors.New("invalid resolving surface attribution")

// ResolverSurface returns the trusted adapter's known kind without inventing a
// pane instance or exposing a session ID. It is display data, not authorization.
func ResolverSurface(resolver string) *ResolvingSurface {
	kind := ""
	switch resolver {
	case ResolverElicitation:
		kind = "mcp_host"
	case ResolverAGUIInterrupt:
		kind = "chat"
	case ResolverDesktop:
		kind = "desktop"
	case ResolverMacOSLocalAuth, ResolverLinuxPolkit:
		kind = "native_prompt"
	case ResolverCLI:
		kind = "cli"
	default:
		return nil
	}
	return &ResolvingSurface{Kind: kind}
}

func cloneSurface(surface *ResolvingSurface) *ResolvingSurface {
	if surface == nil {
		return nil
	}
	out := *surface
	return &out
}

func validSurfaceInstance(instance string) bool {
	if instance == "" || len(instance) > 128 {
		return false
	}
	for _, c := range instance {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validateResolverSurface(resolver string, surface *ResolvingSurface) error {
	if surface == nil {
		return nil
	}
	if surface.Instance != "" && !validSurfaceInstance(surface.Instance) {
		return ErrInvalidResolvingSurface
	}
	expected := ResolverSurface(resolver)
	if expected == nil {
		return ErrInvalidResolvingSurface
	}
	if resolver == ResolverAGUIInterrupt && (surface.Kind == "chat" || surface.Kind == "inbox") {
		return nil
	}
	if surface.Kind != expected.Kind {
		return ErrInvalidResolvingSurface
	}
	return nil
}

// forwardedSurface decodes only the explicitly readable surface branch. Generic
// forwarded props and other branches retain their independent legacy behavior.
func forwardedSurface(raw json.RawMessage) (*ResolvingSurface, error) {
	var forwarded map[string]json.RawMessage
	if json.Unmarshal(raw, &forwarded) != nil {
		return nil, nil
	}
	var ipfs map[string]json.RawMessage
	if json.Unmarshal(forwarded["ipfs"], &ipfs) != nil {
		return nil, nil
	}
	surfaceRaw, ok := ipfs["surface"]
	if !ok {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(surfaceRaw, &fields) != nil || len(fields) != 2 {
		return nil, ErrInvalidResolvingSurface
	}
	var surface ResolvingSurface
	if json.Unmarshal(fields["kind"], &surface.Kind) != nil || json.Unmarshal(fields["instance"], &surface.Instance) != nil {
		return nil, ErrInvalidResolvingSurface
	}
	if !validSurfaceInstance(surface.Instance) || validateResolverSurface(ResolverAGUIInterrupt, &surface) != nil {
		return nil, ErrInvalidResolvingSurface
	}
	return &surface, nil
}
