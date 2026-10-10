package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

func init() { registerGroup(nodeTools) }

// idResponse is the /api/v0/id response.
type idResponse struct {
	ID           string
	PublicKey    string
	Addresses    []string
	AgentVersion string
}

// nodeInfoOutput is the ipfs_node_info output.
type nodeInfoOutput struct {
	PeerID       string   `json:"peer_id"`
	AgentVersion string   `json:"agent_version,omitempty"`
	Version      string   `json:"version"`
	Commit       string   `json:"commit,omitempty"`
	RepoVersion  string   `json:"repo_version,omitempty"`
	Addresses    []string `json:"addresses"`
}

// nodeStatusOutput is the ipfs_node_status output.
type nodeStatusOutput struct {
	DaemonReachable  bool          `json:"daemon_reachable"`
	DaemonVersion    string        `json:"daemon_version,omitempty"`
	MinKuboVersion   string        `json:"min_kubo_version,omitempty"`
	VersionSupported bool          `json:"version_supported"`
	ContractVersion  string        `json:"contract_version"`
	Transport        string        `json:"transport"`
	ReadOnly         bool          `json:"read_only"`
	EnabledGroups    []string      `json:"enabled_groups"`
	Unavailable      []Unavailable `json:"unavailable_tools"`
}

func nodeTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "ipfs_node_info", Call: s.nodeInfo},
		{Name: "ipfs_node_status", Call: s.nodeStatus},
	}
}

func (s *Server) nodeInfo(ctx context.Context, _ json.RawMessage) (any, error) {
	var id idResponse
	if err := s.daemon.Call(ctx, "id", nil, nil, &id); err != nil {
		return nil, err
	}
	var ver daemonVersion
	if err := s.daemon.Call(ctx, "version", nil, nil, &ver); err != nil {
		return nil, err
	}
	addrs := id.Addresses
	if addrs == nil {
		addrs = []string{}
	}
	return nodeInfoOutput{
		PeerID:       id.ID,
		AgentVersion: id.AgentVersion,
		Version:      ver.Version,
		Commit:       ver.Commit,
		RepoVersion:  ver.Repo,
		Addresses:    addrs,
	}, nil
}

// nodeStatus reports on the server and its daemon connection. A daemon that
// cannot be reached is reported, not raised, so the tool stays useful when it
// is needed most.
func (s *Server) nodeStatus(ctx context.Context, _ json.RawMessage) (any, error) {
	out := nodeStatusOutput{
		MinKuboVersion:  s.cat.MinKuboVersion,
		ContractVersion: s.cat.ContractVersion,
		Transport:       s.opts.Transport,
		ReadOnly:        s.opts.ReadOnly,
		EnabledGroups:   append([]string{}, s.enabled...),
		Unavailable:     append([]Unavailable{}, s.unavailable...),
	}
	var ver daemonVersion
	if err := s.daemon.Call(ctx, "version", nil, nil, &ver); err != nil {
		if errors.Is(err, ErrDaemonUnreachable) {
			return out, nil
		}
		return nil, err
	}
	out.DaemonReachable = true
	out.DaemonVersion = ver.Version
	ok, err := versionAtLeast(ver.Version, s.cat.MinKuboVersion)
	if err != nil {
		return nil, fmt.Errorf("daemon version: %w", err)
	}
	out.VersionSupported = ok
	return out, nil
}
