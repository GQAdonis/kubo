package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// This file is built with and without the noagent tag: the embeddings feature
// is independent of the agent loop. It shares only HostFromURL, AckCovers and
// SecretRef with the rest of the package.

// Settings resolves the embeddings section into settings for the embeddings
// package: it reads each credential reference now. It does no other I/O.
func (c *EmbeddingsConfig) Settings() (embeddings.Settings, error) {
	if c == nil {
		return embeddings.Settings{}, errors.New("no embeddings section")
	}
	s := embeddings.Settings{Mode: embeddings.Mode(c.Mode)}
	var errs []error
	if c.Embedded != nil {
		s.Embedded = &embeddings.EmbeddedSettings{
			Binary:     c.Embedded.SurrealBinary,
			DataDir:    c.Embedded.DataDir,
			MinVersion: c.Embedded.MinVersion,
			AvoidPorts: ReservedPorts,
		}
	}
	if c.Remote != nil {
		r := &embeddings.RemoteSettings{Endpoint: c.Remote.Endpoint, Namespace: c.Remote.Namespace, Database: c.Remote.Database}
		if c.Remote.Credential != nil {
			v, err := c.Remote.Credential.Read()
			if err != nil {
				errs = append(errs, fmt.Errorf("embeddings.remote.credential: %w", err))
			}
			r.Credential = embeddings.Secret(v)
		}
		s.Remote = r
	}
	if c.Embedder != nil {
		e := embeddings.EmbedderSettings{BaseURL: c.Embedder.BaseURL, Model: c.Embedder.Model, Dimensions: c.Embedder.Dimensions}
		if c.Embedder.APIKey != nil {
			v, err := c.Embedder.APIKey.Read()
			if err != nil {
				errs = append(errs, fmt.Errorf("embeddings.embedder.api_key: %w", err))
			}
			e.APIKey = embeddings.Secret(v)
		}
		s.Embedder = e
	}
	if c.Policy != nil {
		s.Policy = embeddings.Policy{
			Auto:          c.Policy.Auto,
			MIMEAllowlist: c.Policy.MIMEAllowlist,
			MaxBytes:      c.Policy.MaxBytes,
			ChunkChars:    c.Policy.ChunkChars,
		}
	}
	if err := errors.Join(errs...); err != nil {
		return embeddings.Settings{}, err
	}
	return s, nil
}

// EnableEmbeddings opens the embeddings feature for the configured section: it
// starts the embedded store or connects to the remote one and creates the
// schema. With a nil section it does nothing, so nothing is started and nothing
// is contacted. The embedder is not contacted until content is indexed.
func (s *Server) EnableEmbeddings(ctx context.Context, cfg *EmbeddingsConfig, log *slog.Logger) error {
	if cfg == nil {
		return nil
	}
	if s.embeddingsExt != nil {
		return errors.New("embeddings are already enabled")
	}
	set, err := cfg.Settings()
	if err != nil {
		return err
	}
	svc, err := embeddings.Open(ctx, set, embeddings.Options{Logger: log})
	if err != nil {
		return err
	}
	s.attachEmbeddings(svc)
	return nil
}

// Embeddings returns the embeddings service, or nil when no embeddings section
// is configured.
func (s *Server) Embeddings() *embeddings.Service { return s.embeddingsExt }

// CloseEmbeddings stops the embedded store process, if one was started. It is
// safe to call when embeddings are off.
func (s *Server) CloseEmbeddings() error {
	if s.embeddingsExt == nil {
		return nil
	}
	return s.embeddingsExt.Close()
}

// EgressCovers returns the host check for an identity's egress_ack, for the
// embeddings service. The operator identity (stdio, --auth-token-*) carries no
// egress_ack and so covers nothing: its content is never sent to an embedder,
// which is the safe direction. This is the agent loop's rule, which applies to
// every identity whether or not it is a configured principal.
func EgressCovers(id Identity) func(host string) bool {
	ack := id.EgressAck
	return func(host string) bool { return AckCovers(ack, host) }
}
