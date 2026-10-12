# Spec Delta: mcp-peer-supplied-data

## Purpose

Marks data that remote peers supplied, such as the addresses in peer and routing results, so agents treat it as untrusted unless the operator trusts that peer by ID.

## ADDED Requirements

### Requirement: Peer-supplied entries carry an untrusted marker
For every tool that lists `peerSuppliedFields` in the catalog, each result object holding such a field SHALL carry a boolean `untrusted`. It SHALL be `false` only when the entry's peer ID is listed in the `ipfs mcp` config key `trusted_peers`, and `true` otherwise. A consumer that finds the field absent SHALL treat the entry as untrusted.

#### Scenario: Default config
- **WHEN** `trusted_peers` is unset and an agent calls `swarm_peers`
- **THEN** every entry has `untrusted: true` and the result has `_meta["ai.ipfs/untrusted-content"] = true`

#### Scenario: A trusted peer
- **WHEN** `trusted_peers` lists peer P and `routing_findprovs` returns providers P and Q
- **THEN** P's entry has `untrusted: false`, Q's entry has `untrusted: true`, and the result carries the untrusted-content meta flag because Q is untrusted

#### Scenario: Only trusted peers
- **WHEN** every entry of a `swarm_peers` result belongs to a listed peer
- **THEN** every entry has `untrusted: false` and the result carries no untrusted-content meta flag

### Requirement: Trust is by authenticated peer ID only
`trusted_peers` SHALL accept only libp2p peer IDs, and no other peer property (agent version, protocols, address) SHALL make an entry trusted.

#### Scenario: Agent version in the list
- **WHEN** a config sets `trusted_peers` to `["go-ipfs/0.4.23"]`
- **THEN** `ipfs mcp` refuses the config with a validation error naming `trusted_peers`

### Requirement: The node's own data is not peer-supplied
Results that describe this node (for example `ipfs_node_info` addresses) SHALL NOT carry the marker and SHALL NOT be flagged because of this rule.

#### Scenario: Node info
- **WHEN** an agent calls `ipfs_node_info`
- **THEN** the result has no `untrusted` field and is not flagged as untrusted content

### Requirement: The marker does not change content egress
Marking peer-supplied data SHALL NOT change any tool's `contentEgress` class or its `egress_ack` handling.

#### Scenario: Principal without egress_ack
- **WHEN** a principal with the diag group and no `egress_ack` calls `swarm_peers`
- **THEN** the call succeeds as before, and the entries are marked untrusted

### Requirement: WebMCP marks every peer-supplied entry untrusted
`mcp/webmcp` SHALL set `untrusted: true` on every peer-supplied entry, SHALL register these tools with `untrustedContentHint: true`, and SHALL place their results under `data` with `untrusted_content: true`. It has no trusted list this phase.

#### Scenario: Browser call
- **WHEN** a WebMCP host calls `routing_findpeer`
- **THEN** the result's `data` has `untrusted: true` and the result has `untrusted_content: true`
