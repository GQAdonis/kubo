package embeddings

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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

// StageStatus is whether one stage of the pipeline can be used now. Reason is
// set when it cannot.
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
			return errors.New("the embedded xberg process has exited")
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
	out := map[string]StageStatus{StageStore: stageOf(trimUnavailable(s.Ready(ctx)))}
	if covers != nil && covers(s.embedder.Host()) {
		out[StageEmbedder] = stageOf(s.embedder.Probe(ctx))
	}
	switch x := s.extractor; {
	case x != nil && (!x.remote || (covers != nil && covers(x.host))):
		out[StageExtractor] = stageOf(x.Probe(ctx))
	case x == nil && s.settings.Extractor != nil:
		out[StageExtractor] = stageOf(cmp.Or(s.extractorErr, errors.New("the extractor is not running")))
	}
	return out
}

func stageOf(err error) StageStatus {
	if err == nil {
		return StageStatus{Available: true}
	}
	return StageStatus{Reason: clip(err.Error())}
}

// trimUnavailable drops the generic prefix so a stage reason is the cause.
func trimUnavailable(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.TrimPrefix(err.Error(), ErrStoreUnavailable.Error()+": "))
}
