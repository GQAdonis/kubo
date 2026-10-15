//go:build !noagent

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// uarBackend is the UAR agent behind /agui (ADR-0010): kubo provisions the
// agent on the instance the operator named, proxies each run to its AG-UI
// endpoint and translates the interrupts. It holds no model key and runs no
// tool; the UAR agent calls this server's MCP endpoint, where the confirmation
// gate decides every write. It is built without any network call: the agent is
// provisioned on the first run, and again after a backend change builds a new
// uarBackend.
type uarBackend struct {
	srv *Server
	set *AgentSettings
	// prompt is the system prompt UAR holds: the fixed safety prefix, then the
	// operator's custom part.
	prompt  string
	cli     *uarClient
	log     *slog.Logger
	now     func() time.Time
	limiter *settingsLimiter

	// provMu serializes provisioning; provisioned is true once it succeeded.
	provMu      sync.Mutex
	provisioned bool

	mu      sync.Mutex
	threads map[string]*uarThread
}

// newUARBackend validates the settings and builds the backend. It contacts
// nothing.
func newUARBackend(s *Server, set *AgentSettings) (*uarBackend, error) {
	if !set.IsUAR() || set.UAR == nil {
		return nil, errors.New("not a UAR backend")
	}
	if err := validateBackendURL(set.BaseURL); err != nil {
		return nil, err
	}
	return &uarBackend{
		srv:     s,
		set:     set,
		prompt:  composeSystemPrompt(set.SystemPrompt),
		cli:     newUARClient(set.BaseURL, set.APIKey, set.HTTPClient),
		log:     s.log,
		now:     time.Now,
		limiter: newSettingsLimiter(defaultRunBurst, defaultRunsPerMinute, 0),
		threads: map[string]*uarThread{},
	}, nil
}

// installUAR makes the UAR backend the one that serves /agui. A backend change
// builds a new uarBackend: threads and open interrupts of the old one end, as
// a swap of the model backend revokes the interrupts of the old endpoint.
func (s *Server) installUAR(set *AgentSettings) error {
	b, err := newUARBackend(s, set)
	if err != nil {
		return err
	}
	s.agentMu.Lock()
	s.uarExt = b
	s.agentMu.Unlock()
	return nil
}

// UARBackend returns the UAR backend, or nil when the backend is not a UAR
// agent.
func (s *Server) UARBackend() *uarBackend {
	s.agentMu.RLock()
	defer s.agentMu.RUnlock()
	b, _ := s.uarExt.(*uarBackend)
	return b
}

// BackendHost is the host:port of the UAR instance.
func (b *uarBackend) BackendHost() string { return HostFromURL(b.set.BaseURL) }

// ensureProvisioned provisions the agent once. A failure is not cached: the
// next run tries again, so an instance that was down at the first run is
// picked up without a restart.
func (b *uarBackend) ensureProvisioned(ctx context.Context) error {
	b.provMu.Lock()
	defer b.provMu.Unlock()
	if b.provisioned {
		return nil
	}
	if b.set.UAR.MCPCredential == nil {
		b.log.Warn("llm.uar.mcp_principal is not set: the UAR agent is provisioned without the kubo MCP binding and has no kubo tools")
	}
	if err := b.provisionUAR(ctx); err != nil {
		return fmt.Errorf("provision UAR agent: %w", err)
	}
	b.provisioned = true
	b.log.Info("UAR agent provisioned", "agent", b.set.UAR.AgentID, "host", b.BackendHost())
	return nil
}

// forgetProvisioned makes the next run provision again (the instance lost the
// agent).
func (b *uarBackend) forgetProvisioned() {
	b.provMu.Lock()
	b.provisioned = false
	b.provMu.Unlock()
}

// uarThreadID is the thread id UAR sees: a digest of the principal and the
// client's thread id. UAR keeps one thread space for the credential kubo
// presents, so two chat principals that pick the same thread id must not meet
// there.
func uarThreadID(principal, thread string) string {
	sum := sha256.Sum256([]byte(principal + "\x00" + thread))
	return "kubo-" + hex.EncodeToString(sum[:16])
}
