//go:build !noagent

package server

import (
	"bytes"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

var (
	a2uiCatalogVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	a2uiWireVersionPattern    = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)
)

// forwardedA2UI never reports a payload error: incompatible capabilities turn
// off this presentation branch while independent context validation remains.
func forwardedA2UI(raw json.RawMessage) []catalogTuple {
	var forwarded, ipfs map[string]json.RawMessage
	if json.Unmarshal(raw, &forwarded) != nil || json.Unmarshal(forwarded["ipfs"], &ipfs) != nil {
		return nil
	}
	raw = ipfs["a2ui"]
	if canonicalUIBindingsErr != nil || len(raw) == 0 || len(raw) > canonicalUIBindings.descriptor.Negotiation.MaxEncodedBytes || !utf8.Valid(raw) {
		return nil
	}
	if !uniqueJSONKeys(json.NewDecoder(bytes.NewReader(raw)), 0) {
		return nil
	}
	var namespace map[string]json.RawMessage
	if json.Unmarshal(raw, &namespace) != nil || len(namespace) != 1 {
		return nil
	}
	array := bytes.TrimSpace(namespace["catalogs"])
	if len(array) == 0 || array[0] != '[' {
		return nil
	}
	var entries []json.RawMessage
	if json.Unmarshal(array, &entries) != nil || len(entries) > 4 {
		return nil
	}
	offers := make([]catalogTuple, 0, len(entries))
	seen := make(map[catalogTuple]struct{}, len(entries))
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil || len(fields) != 3 || fields["catalogId"] == nil || fields["catalogVersion"] == nil || fields["protocolVersion"] == nil {
			return nil
		}
		var offer catalogTuple
		if json.Unmarshal(entry, &offer) != nil {
			return nil
		}
		if utf8.RuneCountInString(offer.CatalogID) < 1 || utf8.RuneCountInString(offer.CatalogID) > 512 || len(offer.CatalogVersion) > 32 || !a2uiCatalogVersionPattern.MatchString(offer.CatalogVersion) || len(offer.ProtocolVersion) > 16 || !a2uiWireVersionPattern.MatchString(offer.ProtocolVersion) {
			return nil
		}
		if _, duplicate := seen[offer]; duplicate {
			return nil
		}
		seen[offer] = struct{}{}
		offers = append(offers, offer)
	}
	return offers
}

// Duplicate object keys are invalid too; decoding into a Go struct alone would
// silently give the final occurrence precedence. The byte cap also bounds work.
func uniqueJSONKeys(decoder *json.Decoder, depth int) bool {
	if depth > 32 {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, container := token.(json.Delim)
	if !container {
		return true
	}
	keys := map[string]struct{}{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return false
			}
			name, ok := key.(string)
			if !ok {
				return false
			}
			if _, duplicate := keys[name]; duplicate {
				return false
			}
			keys[name] = struct{}{}
		}
		if !uniqueJSONKeys(decoder, depth+1) {
			return false
		}
	}
	end, err := decoder.Token()
	return err == nil && (delim == '{' && end == json.Delim('}') || delim == '[' && end == json.Delim(']'))
}

func (e *a2uiEmitter) negotiate(offers []catalogTuple) *a2uiEmitter {
	if e == nil {
		return nil
	}
	for _, offer := range offers {
		if offer == e.tuple {
			return e
		}
	}
	return nil
}
