package embeddings

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Stage names of Service.Stages.
const (
	StageStore     = "store"
	StageEmbedder  = "embedder"
	StageExtractor = "extractor"
)

// stageProbeTimeout bounds the reachability probe of one stage.
const stageProbeTimeout = 3 * time.Second

// Reasons a stage cannot be used. They are fixed codes: a reason never carries
// a dial address, a path or process output, which only the server log gets.
const (
	// ReasonUnreachable: nothing answered at the stage's address.
	ReasonUnreachable = "unreachable"
	// ReasonNotStarted: the embedded process could not be started.
	ReasonNotStarted = "not_started"
	// ReasonStarting: the stage is still starting; ask again shortly.
	ReasonStarting = "starting"
	// ReasonExited: the embedded process ended after it had started.
	ReasonExited = "exited"
	// ReasonAuthFailed: the service rejected the credentials.
	ReasonAuthFailed = "auth_failed"
	// ReasonTimeout: the stage did not answer in time.
	ReasonTimeout = "timeout"
	// ReasonUnavailable: any other cause.
	ReasonUnavailable = "unavailable"
)

// errNotStarted marks the failure to start an embedded process.
var errNotStarted = errors.New("the embedded process could not be started")

// errExtractorExited: the embedded extractor ended after it had started.
var errExtractorExited = errors.New("the embedded xberg process has exited")

// reasonOf maps the cause of an unavailable stage to a Reason code.
func reasonOf(err error) string {
	var ne net.Error
	var op *net.OpError
	var dns *net.DNSError
	switch {
	case errors.Is(err, ErrStoreExited), errors.Is(err, errExtractorExited):
		return ReasonExited
	case errors.Is(err, ErrAuth):
		return ReasonAuthFailed
	case errors.Is(err, errNotStarted):
		return ReasonNotStarted
	case errors.Is(err, errOpening):
		return ReasonStarting
	case errors.As(err, &ne) && ne.Timeout():
		return ReasonTimeout
	case errors.As(err, &op), errors.As(err, &dns):
		return ReasonUnreachable
	}
	return ReasonUnavailable
}

// StageStatus is whether one stage of the pipeline can be used now. Reason is
// set when it cannot, and is one of the Reason codes.
type StageStatus struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// probeBase asks a service whether anything answers at its base URL. It sends
// no credential and no content: a GET of the URL the operator configured. Any
// HTTP answer, whatever its status, means the service is up; only a failure to
// connect or to get an answer is an error.
func probeBase(ctx context.Context, hc *http.Client, base string) error {
	ctx, cancel := context.WithTimeout(ctx, stageProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return hostOnly(err)
	}
	_ = resp.Body.Close()
	return nil
}

// Probe reports whether the embedder's server answers. It sends no text and
// no key.
func (e *Embedder) Probe(ctx context.Context) error {
	return probeBase(ctx, e.hc, strings.TrimSuffix(e.endpoint, "/embeddings"))
}

// Probe reports whether the extractor can be used: a remote one must answer,
// an embedded one must still be running. It sends no file.
func (e *Extractor) Probe(ctx context.Context) error {
	if e.sup != nil {
		select {
		case <-e.sup.Done():
			return errExtractorExited
		default:
			return nil
		}
	}
	return probeBase(ctx, e.hc, strings.TrimSuffix(e.endpoint, "/extract"))
}

// Ready reports whether the store is open: reached, with its schema defined.
// It tries to open it when the retry schedule allows. The error wraps
// ErrStoreUnavailable. A service built around a ready-made store is always
// ready.
func (s *Service) Ready(ctx context.Context) error {
	if s.lazy == nil {
		return nil
	}
	return s.lazy.ready(ctx)
}

// Stages says which stages cannot or can be used now: the store, the embedder
// and, when one is configured, the extractor. The store is the operator's own
// and is always checked. The embedder and a remote extractor are probed only
// when covers acknowledges their host, the same rule that guards sending them
// text: a caller whose egress_ack does not cover a host gets no entry for that
// stage, and the host is not contacted. A nil covers acknowledges nothing.
func (s *Service) Stages(ctx context.Context, covers func(host string) bool) map[string]StageStatus {
	out := map[string]StageStatus{StageStore: s.stageOf(StageStore, s.Ready(ctx))}
	if covers != nil && covers(s.embedder.Host()) {
		out[StageEmbedder] = s.stageOf(StageEmbedder, s.embedder.Probe(ctx))
	}
	switch x := s.extractor; {
	case x != nil && (!x.remote || (covers != nil && covers(x.host))):
		out[StageExtractor] = s.stageOf(StageExtractor, x.Probe(ctx))
	case x == nil && s.settings.Extractor != nil:
		out[StageExtractor] = s.stageOf(StageExtractor, cmp.Or(s.extractorErr, error(errNotStarted)))
	}
	return out
}

// stageOf turns the cause of a stage into its status. The detail goes to the
// server log at Warn, once each time it changes, so polling does not flood it.
func (s *Service) stageOf(name string, err error) StageStatus {
	s.stageLogMu.Lock()
	defer s.stageLogMu.Unlock()
	if err == nil {
		delete(s.stageLogged, name)
		return StageStatus{Available: true}
	}
	code, detail := reasonOf(err), clip(err.Error())
	if s.stageLogged[name] != detail {
		if s.stageLogged == nil {
			s.stageLogged = map[string]string{}
		}
		s.stageLogged[name] = detail
		log := s.log
		if log == nil {
			log = slog.New(slog.DiscardHandler)
		}
		log.Warn("embeddings stage is unavailable", "stage", name, "reason", code, "error", detail)
	}
	return StageStatus{Reason: code}
}
