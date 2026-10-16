//go:build !noagent

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// uarProvisionWait bounds how long a chat waits for another chat's
// provisioning of the same backend before it fails with a retryable error. The
// first chat after a start or a settings change provisions (up to a dozen
// management calls); the others must not queue behind it for as long as it
// takes.
const uarProvisionWait = 5 * time.Second

// errUARProvisioning is what a chat gets while another chat is provisioning.
var errUARProvisioning = &uarBackendError{"The UAR agent is being provisioned by another chat; try again in a moment."}

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
	// gen identifies this backend instance. It is part of every UAR thread id,
	// so a rebuilt backend, which forgot its threads and their open interrupts,
	// never reaches a UAR thread that still holds them.
	gen string

	// provisioned is true once provisioning succeeded; provSem (capacity 1) is
	// the slot of the one chat that provisions, and provWait how long the
	// others wait for it. ownedSkills (guarded by provSem) are the skill ids
	// kubo bound last time.
	provisioned atomic.Bool
	provSem     chan struct{}
	provWait    time.Duration
	ownedSkills []string

	mu        sync.Mutex
	threads   map[string]*uarThread
	threadSeq uint64
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
		srv:      s,
		set:      set,
		prompt:   composeSystemPrompt(set.SystemPrompt),
		cli:      newUARClient(set.BaseURL, set.APIKey, set.HTTPClient),
		log:      s.log,
		now:      time.Now,
		limiter:  newSettingsLimiter(defaultRunBurst, defaultRunsPerMinute, 0),
		gen:      randomID(9),
		provSem:  make(chan struct{}, 1),
		provWait: uarProvisionWait,
		threads:  map[string]*uarThread{},
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
// picked up without a restart. One chat provisions at a time; a chat that
// arrives meanwhile waits at most provWait (and never past its own deadline),
// then fails with a retryable error, so a slow instance cannot hold every chat.
func (b *uarBackend) ensureProvisioned(ctx context.Context) error {
	if b.provisioned.Load() {
		return nil
	}
	wait := time.NewTimer(b.provWait)
	defer wait.Stop()
	select {
	case b.provSem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-wait.C:
		return errUARProvisioning
	}
	defer func() { <-b.provSem }()
	if b.provisioned.Load() {
		return nil
	}
	if err := b.provisionUAR(ctx); err != nil {
		var conflict *uarSkillConflictError
		if errors.As(err, &conflict) {
			b.log.Warn("UAR provisioning stopped", "skill", conflict.Name, "reason", "a skill with this name and different content exists on the instance")
			return conflict
		}
		return fmt.Errorf("provision UAR agent: %w", err)
	}
	b.provisioned.Store(true)
	b.log.Info("UAR agent provisioned", "agent", b.set.UAR.AgentID, "host", b.BackendHost())
	return nil
}

// forgetProvisioned makes the next run provision again (the instance lost the
// agent).
func (b *uarBackend) forgetProvisioned() { b.provisioned.Store(false) }

// uarThreadID is the thread id UAR sees: a digest of the backend generation,
// the principal, the client's thread id and a per-backend sequence number. UAR
// keeps one thread space for the credential kubo presents, so two chat
// principals that pick the same thread id must not meet there, and a thread
// kubo forgot (evicted, abandoned after a failed resume, or lost with a
// rebuilt backend) must not map back to a UAR thread that still holds its open
// interrupts.
func uarThreadID(gen string, seq uint64, principal, thread string) string {
	sum := sha256.Sum256([]byte(gen + "\x00" + strconv.FormatUint(seq, 10) + "\x00" + principal + "\x00" + thread))
	return "kubo-" + hex.EncodeToString(sum[:16])
}

// nextThreadSeqLocked returns a sequence number no thread of this backend has
// used. Callers hold b.mu.
func (b *uarBackend) nextThreadSeqLocked() uint64 {
	b.threadSeq++
	return b.threadSeq
}
