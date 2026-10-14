// Package embeddings is the vector index of `ipfs mcp`: a SurrealDB store of
// document embeddings keyed by CID, and the client of the operator's
// OpenAI-compatible embeddings endpoint.
//
// Nothing here runs unless the operator configures an embeddings section: there
// is no default store, no default embedder, and no default port. Embedded mode
// supervises a SurrealDB process from a binary the operator names by absolute
// path; remote mode connects to a URL the operator names. Both talk to
// SurrealDB over its HTTP API with the standard library; no SurrealDB module is
// linked into Kubo. SOURCES.md cites the source of every query this package
// sends.
//
// An optional extractor turns documents, code and data into Markdown through
// xberg, a separate service: embedded mode supervises an operator-supplied
// xberg binary on a loopback ephemeral port, remote mode connects to a URL the
// operator names. Only POST /extract is used, over the standard library, and no
// xberg code is linked. Without an extractor section no process starts and no
// request is made.
//
// The package has no build tag, so it is present in builds with the noagent
// tag. It imports nothing from the parent package: egress decisions arrive as
// a func(host string) bool built from the caller's egress_ack.
package embeddings
