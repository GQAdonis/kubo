package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func init() { registerGroup(routingTools) }

func routingTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "routing_findprovs", Call: s.routingFindProvs},
		{Name: "routing_findpeer", Call: s.routingFindPeer},
		{Name: "routing_provide", Call: s.routingProvide},
	}
}

// Query event types of the daemon's routing commands (libp2p
// core/routing.QueryEventType).
const (
	queryEventFinalPeer  = 2
	queryEventQueryError = 3
	queryEventProvider   = 4

	defaultNumProviders = 20
)

// queryEvent is one line of the routing/findprovs, routing/findpeer and
// routing/provide streams.
type queryEvent struct {
	ID        string
	Type      int
	Responses []struct {
		ID    string
		Addrs []string
	}
	Extra string
}

// readQueryEvents decodes the event stream and calls fn for each event until
// fn returns stop=true or the stream ends.
func readQueryEvents(command string, fn func(queryEvent) (stop bool, err error)) func(*json.Decoder) error {
	return func(dec *json.Decoder) error {
		for {
			var ev queryEvent
			if err := dec.Decode(&ev); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("decode %s stream: %w", command, err)
			}
			stop, err := fn(ev)
			if err != nil {
				return err
			}
			if stop {
				return ErrStopStream
			}
		}
	}
}

type peerAddrs struct {
	ID    string   `json:"id"`
	Addrs []string `json:"addrs"`
}

type routingFindProvsInput struct {
	CID          string `json:"cid"`
	NumProviders int    `json:"num_providers"`
}

type routingFindProvsOutput struct {
	CID       string      `json:"cid"`
	Providers []peerAddrs `json:"providers"`
}

// routingFindProvs asks the configured routing system for providers and stops
// reading as soon as num_providers distinct ones arrived.
func (s *Server) routingFindProvs(ctx context.Context, raw json.RawMessage) (any, error) {
	var in routingFindProvsInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	n := in.NumProviders
	if n == 0 {
		n = defaultNumProviders
	}
	out := routingFindProvsOutput{CID: in.CID, Providers: []peerAddrs{}}
	seen := map[string]struct{}{}
	opts := map[string]any{"num-providers": n, "verbose": false}
	err := s.daemon.Stream(ctx, "routing/findprovs", []string{in.CID}, opts, readQueryEvents("routing/findprovs", func(ev queryEvent) (bool, error) {
		if ev.Type != queryEventProvider {
			return false, nil
		}
		for _, r := range ev.Responses {
			if _, dup := seen[r.ID]; dup || r.ID == "" {
				continue
			}
			seen[r.ID] = struct{}{}
			out.Providers = append(out.Providers, peerAddrs{ID: r.ID, Addrs: nonNil(r.Addrs)})
		}
		return len(out.Providers) >= n, nil
	}))
	if err != nil {
		return nil, err
	}
	return out, nil
}

type routingFindPeerInput struct {
	PeerID string `json:"peer_id"`
}

func (s *Server) routingFindPeer(ctx context.Context, raw json.RawMessage) (any, error) {
	var in routingFindPeerInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	var found *peerAddrs
	var queryErr string
	err := s.daemon.Stream(ctx, "routing/findpeer", []string{in.PeerID}, map[string]any{"verbose": false}, readQueryEvents("routing/findpeer", func(ev queryEvent) (bool, error) {
		switch ev.Type {
		case queryEventFinalPeer:
			for _, r := range ev.Responses {
				found = &peerAddrs{ID: r.ID, Addrs: nonNil(r.Addrs)}
				return true, nil
			}
		case queryEventQueryError:
			queryErr = ev.Extra
		}
		return false, nil
	}))
	if err != nil {
		return nil, err
	}
	if found == nil {
		msg := "routing found no addresses for peer " + in.PeerID
		if queryErr != "" {
			msg += ": " + queryErr
		}
		return nil, &ToolError{Code: CodeNotFound, Message: msg}
	}
	return *found, nil
}

type routingProvideInput struct {
	CID       string `json:"cid"`
	Recursive bool   `json:"recursive"`
}

type routingProvideOutput struct {
	CID               string `json:"cid"`
	Announced         bool   `json:"announced"`
	ProvidingDisabled bool   `json:"providing_disabled"`
}

// routingProvide announces one CID, or a DAG when recursive, after the
// caller's confirmation. It makes no announcement when the operator turned
// providing off with Provide.Enabled=false. The setting is read from the node
// first. When the API authorization forbids reading it (the contract lists
// only routing/provide for this tool), the daemon's own refusal decides:
// routing/provide checks the same setting before it announces anything. Any
// other failure to read the setting stops the call.
func (s *Server) routingProvide(ctx context.Context, raw json.RawMessage) (any, error) {
	var in routingProvideInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	disabled := routingProvideOutput{CID: in.CID, ProvidingDisabled: true}
	enabled, err := s.provideEnabled(ctx)
	switch {
	case isForbidden(err):
		// Rely on the daemon's own check below.
	case err != nil:
		return nil, fmt.Errorf("refusing to announce %s: cannot read Provide.Enabled: %w", in.CID, err)
	case !enabled:
		return disabled, nil
	}

	opts := map[string]any{"recursive": in.Recursive, "verbose": false}
	var queryErr string
	err = s.daemon.Stream(ctx, "routing/provide", []string{in.CID}, opts, readQueryEvents("routing/provide", func(ev queryEvent) (bool, error) {
		if ev.Type == queryEventQueryError {
			queryErr = ev.Extra
		}
		return false, nil
	}))
	if err != nil {
		// The daemon applies the same switch itself; honor it if the setting
		// changed between the read above and the call.
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && strings.Contains(rpcErr.Message, "Provide.Enabled is set to 'false'") {
			return disabled, nil
		}
		return nil, err
	}
	if queryErr != "" {
		return nil, fmt.Errorf("providing %s failed: %s", in.CID, queryErr)
	}
	return routingProvideOutput{CID: in.CID, Announced: true}, nil
}

// provideEnabled reads the node's Provide.Enabled setting. An unset key is
// the default, which is enabled.
func (s *Server) provideEnabled(ctx context.Context) (bool, error) {
	var resp struct {
		Value json.RawMessage
	}
	if err := s.daemon.Call(ctx, "config", []string{"Provide.Enabled"}, nil, &resp); err != nil {
		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && isUnsetConfigKey(rpcErr.Message) {
			return true, nil
		}
		return false, err
	}
	switch strings.TrimSpace(string(resp.Value)) {
	case "", "null", "true":
		return true, nil
	case "false":
		return false, nil
	}
	var str string
	if json.Unmarshal(resp.Value, &str) == nil && str == "false" {
		return false, nil
	}
	return true, nil
}

// isUnsetConfigKey recognises the daemon's answer for a key that is not in
// the config file.
func isUnsetConfigKey(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no such") || strings.Contains(msg, "has no attribute") || strings.Contains(msg, "not exist")
}
