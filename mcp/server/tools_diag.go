package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

func init() { registerGroup(diagTools) }

func diagTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "swarm_peers", Call: s.swarmPeers},
		{Name: "stats_bw", Call: s.statsBw},
		{Name: "repo_stat", Call: s.repoStat},
		{Name: "repo_gc", Call: s.repoGc},
	}
}

// Connection directions of the daemon's swarm/peers (libp2p network.Direction).
const (
	directionInbound  = 1
	directionOutbound = 2

	// gcSampleSize is the contract's maxItems of repo_gc removed_sample.
	gcSampleSize = 100
	// gcMaxErrors bounds the error strings repo_gc reports.
	gcMaxErrors = 100
)

type swarmPeersInput struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type swarmPeer struct {
	ID        string `json:"id"`
	Addr      string `json:"addr"`
	Direction string `json:"direction,omitempty"`
	Latency   string `json:"latency,omitempty"`
}

type swarmPeersOutput struct {
	Peers []swarmPeer `json:"peers"`
	Page  pageInfo    `json:"page"`
}

// swarmPeers pages over the current connections, sorted by peer and address.
// The cursor is the key of the last item returned, so a page stays correct
// while connections come and go.
func (s *Server) swarmPeers(ctx context.Context, raw json.RawMessage) (any, error) {
	var in swarmPeersInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	limit, err := pageLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	cur, err := decodeCursor(in.Cursor)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Peers []struct {
			Addr      string
			Peer      string
			Latency   string
			Direction int
		}
	}
	if err := s.daemon.Call(ctx, "swarm/peers", nil, map[string]any{"latency": true, "direction": true}, &resp); err != nil {
		return nil, err
	}
	peers := make([]swarmPeer, 0, len(resp.Peers))
	for _, p := range resp.Peers {
		peers = append(peers, swarmPeer{ID: p.Peer, Addr: p.Addr, Direction: directionName(p.Direction), Latency: p.Latency})
	}
	slices.SortFunc(peers, func(a, b swarmPeer) int { return strings.Compare(peerKey(a), peerKey(b)) })
	page, info := pageByKey(peers, peerKey, cur, limit)
	if page == nil {
		page = []swarmPeer{}
	}
	return swarmPeersOutput{Peers: page, Page: info}, nil
}

// peerKey is unique per connection: a peer can hold several.
func peerKey(p swarmPeer) string { return p.ID + " " + p.Addr }

func directionName(d int) string {
	switch d {
	case directionInbound:
		return "inbound"
	case directionOutbound:
		return "outbound"
	}
	return "unknown"
}

type statsBwInput struct {
	PeerID   string `json:"peer_id"`
	Protocol string `json:"protocol"`
}

type statsBwOutput struct {
	TotalIn  int64   `json:"total_in"`
	TotalOut int64   `json:"total_out"`
	RateIn   float64 `json:"rate_in"`
	RateOut  float64 `json:"rate_out"`
}

func (s *Server) statsBw(ctx context.Context, raw json.RawMessage) (any, error) {
	var in statsBwInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	opts := map[string]any{"poll": false}
	if in.PeerID != "" {
		opts["peer"] = in.PeerID
	}
	if in.Protocol != "" {
		opts["proto"] = in.Protocol
	}
	var resp struct {
		TotalIn  float64
		TotalOut float64
		RateIn   float64
		RateOut  float64
	}
	if err := s.daemon.Call(ctx, "stats/bw", nil, opts, &resp); err != nil {
		return nil, err
	}
	return statsBwOutput{
		TotalIn:  int64(resp.TotalIn),
		TotalOut: int64(resp.TotalOut),
		RateIn:   max(resp.RateIn, 0),
		RateOut:  max(resp.RateOut, 0),
	}, nil
}

type repoStatOutput struct {
	RepoSize   uint64 `json:"repo_size"`
	StorageMax uint64 `json:"storage_max"`
	NumObjects uint64 `json:"num_objects"`
	Version    string `json:"version,omitempty"`
}

// repoStat reports sizes and counts. The daemon's RepoPath is a local
// filesystem path and is deliberately not returned.
func (s *Server) repoStat(ctx context.Context, _ json.RawMessage) (any, error) {
	var resp struct {
		RepoSize   uint64
		StorageMax uint64
		NumObjects uint64
		Version    string
	}
	if err := s.daemon.Call(ctx, "repo/stat", nil, map[string]any{"size-only": false, "human": false}, &resp); err != nil {
		return nil, err
	}
	return repoStatOutput{RepoSize: resp.RepoSize, StorageMax: resp.StorageMax, NumObjects: resp.NumObjects, Version: resp.Version}, nil
}

type repoGcOutput struct {
	RemovedCount  int      `json:"removed_count"`
	RemovedSample []string `json:"removed_sample"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// repoGc runs garbage collection after destructive confirmation and reports
// the count of removed blocks with a bounded sample of their CIDs.
func (s *Server) repoGc(ctx context.Context, _ json.RawMessage) (any, error) {
	out := repoGcOutput{RemovedSample: []string{}}
	watch, warnings := s.newGCWatch(ctx)
	out.Warnings = warnings
	opts := map[string]any{"stream-errors": true}
	err := s.daemon.Stream(ctx, "repo/gc", nil, opts, func(dec *json.Decoder) error {
		for {
			var line struct {
				Key   json.RawMessage
				Error string
			}
			if err := dec.Decode(&line); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("decode repo/gc stream: %w", err)
			}
			if line.Error != "" {
				if len(out.Errors) < gcMaxErrors {
					out.Errors = append(out.Errors, line.Error)
				}
				continue
			}
			c := cidString(line.Key)
			if c == "" {
				continue
			}
			out.RemovedCount++
			watch.see(c)
			if len(out.RemovedSample) < gcSampleSize {
				out.RemovedSample = append(out.RemovedSample, c)
			}
		}
	})
	// Blocks that were removed before an error stay removed, so their
	// vectors go either way.
	out.Warnings = append(out.Warnings, s.afterGC(ctx, watch)...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
