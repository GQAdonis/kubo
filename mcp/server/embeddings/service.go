package embeddings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
)

// embedBatch is how many chunks go to the embedder in one request.
const embedBatch = 16

// Service is the embeddings feature of one ipfs mcp run: the store, the
// embedder and, in embedded mode, the supervised process. It exists only when
// the operator configured an embeddings section.
type Service struct {
	settings Settings
	store    Store
	// lazy is the store when Open built it, nil for a store passed to
	// NewService. It defines the schema once the store is reachable.
	lazy     *lazyStore
	embedder *Embedder
	// mu guards sup, which a lazy store start sets after Open returns.
	mu  sync.Mutex
	sup *Supervisor
	// storeHost is the host:port of the store when it is not on this machine.
	storeHost string
	policy    Policy
	// extractor is the xberg client, nil when no extractor is configured.
	extractor *Extractor
	// describer is the chat-model client, nil when no describer is configured.
	describer *Describer
	// extractorErr is why a configured extractor could not be started.
	extractorErr error
	// log receives the detail that stage reasons leave out. Nil discards.
	log *slog.Logger
	// stageLogMu guards stageLogged, the detail last logged per stage, so a
	// stage that stays down is logged once and not on every status call.
	stageLogMu  sync.Mutex
	stageLogged map[string]string
}

// Options are test seams of Open. The zero value is the production behavior.
type Options struct {
	Logger *slog.Logger
	// HTTPClient, if set, is the base for the store and embedder clients.
	HTTPClient *http.Client
}

// Open validates the settings and builds the service. Configuration errors
// (invalid settings, rejected credentials, an index of another dimension) fail
// here with nothing left running. A store that cannot be reached, and an
// embedded store or extractor that cannot start, do not: the service opens, the
// stage reports unavailable (Stages) and the store is tried again on use, at
// most once per backoff, until its schema is defined. Without settings there is
// no Open: callers do not construct a Service for an absent embeddings section.
func Open(ctx context.Context, s Settings, opts Options) (*Service, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	// A binary path that cannot run is a configuration error. A binary that
	// exists but fails to start or become healthy only degrades the stage.
	if s.Mode == ModeEmbedded {
		if err := checkBinaryPath("embeddings.embedded.surreal_binary", s.Embedded.Binary); err != nil {
			return nil, err
		}
	}
	if x := s.Extractor; x != nil && x.Mode == ModeEmbedded {
		if err := checkBinaryPath("embeddings.extractor.embedded.xberg_binary", x.Embedded.Binary); err != nil {
			return nil, err
		}
	}
	emb, err := NewEmbedder(s.Embedder, opts.HTTPClient)
	if err != nil {
		return nil, err
	}
	log := opts.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	svc := &Service{settings: s, embedder: emb, policy: s.Policy.Effective(), log: log}
	switch s.Mode {
	case ModeEmbedded:
		svc.lazy = newLazyStore(func(ctx context.Context) (Store, error) { return svc.openEmbeddedStore(ctx, opts, log) }, embeddedOpenTimeout)
	case ModeRemote:
		r := s.Remote
		c := newSurrealClient(r.Endpoint, r.Namespace, r.Database, parseCredential(r.Credential), opts.HTTPClient)
		if u, err := url.Parse(r.Endpoint); err == nil {
			svc.storeHost = u.Host
		}
		st := newSurrealStore(c, s.Embedder.Dimensions)
		svc.lazy = newLazyStore(func(ctx context.Context) (Store, error) {
			if err := st.Bootstrap(ctx); err != nil {
				return nil, err
			}
			return st, nil
		}, startupStoreTimeout)
	}
	svc.store = svc.lazy
	// The open has its own deadline; this waits for it under ctx.
	err = svc.lazy.ready(ctx)
	switch {
	case errors.Is(err, ErrAuth), errors.Is(err, ErrDimensionMismatch):
		_ = svc.Close()
		return nil, fmt.Errorf("embeddings store: %w", err)
	case err != nil:
		log.Warn("embeddings store is unavailable; searching and indexing fail until it is reached, and it is tried again on use", "error", clip(err.Error()))
	}
	if s.Extractor != nil {
		x, err := OpenExtractor(ctx, *s.Extractor, opts.HTTPClient, opts)
		if err != nil {
			svc.extractorErr = fmt.Errorf("%w: %w", errNotStarted, err)
			log.Warn("embeddings extractor is unavailable; files that need it are not extracted until ipfs mcp restarts", "error", clip(err.Error()))
		}
		svc.extractor = x
	}
	if s.Describer != nil {
		d, err := NewDescriber(*s.Describer, opts.HTTPClient)
		if err != nil {
			_ = svc.Close()
			return nil, fmt.Errorf("embeddings describer: %w", err)
		}
		svc.describer = d
	}
	return svc, nil
}

// openEmbeddedStore starts the supervised store if it is not running, then
// defines the schema. A supervisor that has exited is replaced.
func (s *Service) openEmbeddedStore(ctx context.Context, opts Options, log *slog.Logger) (Store, error) {
	s.mu.Lock()
	sup := s.sup
	if sup != nil {
		select {
		case <-sup.Done():
			_ = sup.Close()
			sup = nil
		default:
		}
	}
	if sup == nil {
		var err error
		if sup, err = StartSupervisor(ctx, *s.settings.Embedded, log); err != nil {
			s.sup = nil
			s.mu.Unlock()
			return nil, fmt.Errorf("%w: %w", errNotStarted, err)
		}
	}
	s.sup = sup
	s.mu.Unlock()
	c := newSurrealClient(sup.Endpoint(), embeddedNamespace, embeddedDatabase, parseCredential(sup.Credential()), opts.HTTPClient)
	c.gone = sup.Done()
	st := newSurrealStore(c, s.settings.Embedder.Dimensions)
	if err := st.Bootstrap(ctx); err != nil {
		return nil, err
	}
	return st, nil
}

// NewService assembles a service from parts that are already open: the store
// seam of tests and of callers that bring their own. The settings are not
// validated here; Open validates.
func NewService(s Settings, st Store, e *Embedder) *Service {
	svc := &Service{settings: s, store: st, embedder: e, policy: s.Policy.Effective()}
	if s.Mode == ModeRemote && s.Remote != nil {
		if u, err := url.Parse(s.Remote.Endpoint); err == nil {
			svc.storeHost = u.Host
		}
	}
	return svc
}

// Policy is the effective indexing policy, with defaults filled in.
func (s *Service) Policy() Policy { return s.policy }

// Close stops the embedded store and extractor processes, if any.
func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	if s.lazy != nil {
		s.lazy.close()
	}
	var errs []error
	if s.extractor != nil {
		errs = append(errs, s.extractor.Close())
	}
	if sup := s.Supervisor(); sup != nil {
		errs = append(errs, sup.Close())
	}
	return errors.Join(errs...)
}

// Extractor is the xberg client, or nil when no extractor is configured.
func (s *Service) Extractor() *Extractor { return s.extractor }

// WithExtractor returns the service with an extractor attached: the seam of
// callers and tests that build the extractor themselves. The service takes
// ownership and stops it on Close.
func (s *Service) WithExtractor(x *Extractor) *Service {
	s.extractor = x
	return s
}

// Describer is the chat-model client, or nil when no describer is configured.
func (s *Service) Describer() *Describer { return s.describer }

// WithDescriber returns the service with a describer attached: the seam of
// callers and tests that build the describer themselves.
func (s *Service) WithDescriber(d *Describer) *Service {
	s.describer = d
	return s
}

// Mode is the configured mode.
func (s *Service) Mode() Mode { return s.settings.Mode }

// Store gives direct access to the vector store. Deletion and counts send no
// content, but a remote store still sees what they name: CIDs (the status
// lookup, reconcile and unpin cleanup) and, in bulk run state, MFS path and
// entry names and who started the run.
func (s *Service) Store() Store { return s.store }

// Embedder is the configured embedder.
func (s *Service) Embedder() *Embedder { return s.embedder }

// Supervisor is the embedded store's supervisor, nil in remote mode.
func (s *Service) Supervisor() *Supervisor {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sup
}

// Describe is a one-line, secret-free summary for the startup log.
func (s *Service) Describe() string {
	where := "remote store " + s.storeHost
	if s.settings.Mode == ModeEmbedded {
		where = "embedded store (not started)"
	}
	if sup := s.Supervisor(); sup != nil {
		where = "embedded store on " + sup.Addr()
	}
	desc := fmt.Sprintf("%s, embedder %s model %s (%d dimensions)", where, s.embedder.Host(), s.embedder.Model(), s.embedder.Dimensions())
	switch x := s.extractor; {
	case x == nil:
	case x.sup != nil:
		desc += ", extractor xberg on " + x.sup.Addr()
	default:
		desc += ", extractor xberg at " + x.host
	}
	if d := s.describer; d != nil {
		desc += ", describer " + d.host + " model " + d.model
	}
	return desc
}

// Hosts lists the hosts that content or its vectors reach: the embedder and a
// store that is not on this machine.
func (s *Service) Hosts() []string {
	hosts := []string{s.embedder.Host()}
	if s.storeHost != "" && s.storeHost != s.embedder.Host() {
		hosts = append(hosts, s.storeHost)
	}
	return hosts
}

// StageHosts lists every host that the configured stages may send content to:
// the embedder, a store that is not on this machine, a remote extractor and the
// describer. A bulk run needs all of them acknowledged, because it reaches each
// of them for some file.
func (s *Service) StageHosts() []string {
	hosts := s.Hosts()
	if x := s.extractor; x != nil && x.remote && !slices.Contains(hosts, x.host) {
		hosts = append(hosts, x.host)
	}
	if d := s.describer; d.Enabled() && !slices.Contains(hosts, d.host) {
		hosts = append(hosts, d.host)
	}
	return hosts
}

// StageHost is one host a stage may send content to, with the stage's name.
type StageHost struct{ Role, Host string }

// StageRoles is StageHosts with the stage that uses each host, for texts that
// tell a person where a file may go: the embedder, a store that is not on this
// machine, a remote extractor and the describer.
func (s *Service) StageRoles() []StageHost {
	roles := []StageHost{{"embedder", s.embedder.Host()}}
	if s.storeHost != "" {
		roles = append(roles, StageHost{"store", s.storeHost})
	}
	if x := s.extractor; x != nil && x.remote {
		roles = append(roles, StageHost{"extractor", x.host})
	}
	if d := s.describer; d.Enabled() {
		roles = append(roles, StageHost{"describer", d.host})
	}
	return roles
}

// ModelDownloadHost is the host the embedded extractor may download models
// from when embeddings.extractor.embedded.allow_model_downloads is on: the
// operator's HF_ENDPOINT mirror if one is set, else the Hugging Face Hub. It is
// empty when downloads are off or there is no embedded extractor.
func (s *Service) ModelDownloadHost() string {
	e := s.settings.Extractor
	if e == nil || e.Mode != ModeEmbedded || e.Embedded == nil || !e.Embedded.AllowModelDownloads {
		return ""
	}
	if v := os.Getenv("HF_ENDPOINT"); v != "" {
		if u, err := url.Parse(v); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return "huggingface.co"
}

// CheckStageEgress returns an *EgressError naming every stage host the calling
// identity's egress_ack does not cover. A nil covers covers nothing.
func (s *Service) CheckStageEgress(covers func(host string) bool) error {
	var missing []string
	for _, h := range s.StageHosts() {
		if covers == nil || !covers(h) {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return &EgressError{Hosts: missing}
	}
	return nil
}

// CheckEgress returns an *EgressError naming every host the calling identity's
// egress_ack does not cover. A nil covers covers nothing.
func (s *Service) CheckEgress(covers func(host string) bool) error {
	var missing []string
	for _, h := range s.Hosts() {
		if covers == nil || !covers(h) {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return &EgressError{Hosts: missing}
	}
	return nil
}

// Document is the text of one CID, already split into chunks by the caller.
type Document struct {
	CID    string
	Path   string
	MIME   string
	Chunks []string
	// Kind and Source record how the chunks were produced. Both empty means a
	// natively indexed text file. A Source other than "text" stores the chunk
	// text with the vector.
	Kind   string
	Source string
	// DescribedBy is the describer model when Source is "description".
	DescribedBy string
	// Segments, if set, replaces Chunks, Kind, Source and DescribedBy: a file
	// whose stages each produced text (OCR and a description of one image) is
	// stored in one replacement, so a later stage never erases an earlier one.
	// Chunk indexes continue across the segments in order.
	Segments []Segment
}

// Segment is the chunks one stage produced for a document.
type Segment struct {
	Chunks      []string
	Kind        string
	Source      string
	DescribedBy string
}

// segments returns the document's segments: Segments, or the single one made
// of its own fields.
func (d Document) segments() []Segment {
	if len(d.Segments) > 0 {
		return d.Segments
	}
	return []Segment{{Chunks: d.Chunks, Kind: d.Kind, Source: d.Source, DescribedBy: d.DescribedBy}}
}

// Index embeds the chunks and replaces everything stored for the CID. Nothing
// is sent anywhere unless covers acknowledges the embedder host and, for a
// remote store, its host; indexing is refused with an *EgressError otherwise.
// It returns the number of chunks stored.
func (s *Service) Index(ctx context.Context, covers func(host string) bool, doc Document) (int, error) {
	if doc.CID == "" {
		return 0, errors.New("index: empty CID")
	}
	segs := doc.segments()
	total := 0
	for _, seg := range segs {
		total += len(seg.Chunks)
	}
	if total == 0 {
		return 0, errors.New("index: no chunks")
	}
	if err := s.CheckEgress(covers); err != nil {
		return 0, err
	}
	// Fail before any text goes to the embedder when it could not be stored.
	if err := s.Ready(ctx); err != nil {
		return 0, fmt.Errorf("index %s: %w", doc.CID, err)
	}
	rows := make([]Row, 0, total)
	for _, seg := range segs {
		for start := 0; start < len(seg.Chunks); start += embedBatch {
			end := min(start+embedBatch, len(seg.Chunks))
			vecs, err := s.embedder.Embed(ctx, covers, seg.Chunks[start:end])
			if err != nil {
				return 0, fmt.Errorf("index %s: %w", doc.CID, err)
			}
			for i, v := range vecs {
				row := Row{CID: doc.CID, Path: doc.Path, MIME: doc.MIME, ChunkIndex: len(rows), Model: s.embedder.Model(), Vector: v, Kind: seg.Kind, Source: seg.Source}
				if seg.Source != "" && seg.Source != SourceText {
					row.Text = seg.Chunks[start+i]
				}
				if seg.Source == SourceDescription {
					row.DescribedBy = seg.DescribedBy
				}
				rows = append(rows, row)
			}
		}
	}
	if err := s.store.Replace(ctx, doc.CID, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

// Search embeds the query and returns the k nearest chunks. The query text
// goes to the embedder, so the same egress rule as Index applies.
func (s *Service) Search(ctx context.Context, covers func(host string) bool, query string, k int) ([]Hit, error) {
	return s.SearchFiltered(ctx, covers, query, "", k, Filter{})
}

// SearchCID is Search restricted to the chunks of one CID.
func (s *Service) SearchCID(ctx context.Context, covers func(host string) bool, query, cid string, k int) ([]Hit, error) {
	return s.SearchFiltered(ctx, covers, query, cid, k, Filter{})
}

// Filter limits hits to content kinds and chunk sources. An empty list matches
// every value. A row written before derived content existed has no kind and no
// source and counts as natively indexed text.
type Filter struct {
	Kinds   []string
	Sources []string
}

// Empty reports whether the filter matches every hit.
func (f Filter) Empty() bool { return len(f.Kinds) == 0 && len(f.Sources) == 0 }

// Match reports whether the hit passes the filter.
func (f Filter) Match(h Hit) bool {
	kind, source := h.Kind, h.Source
	if kind == "" {
		kind = KindText
	}
	if source == "" {
		source = SourceText
	}
	return (len(f.Kinds) == 0 || slices.Contains(f.Kinds, kind)) &&
		(len(f.Sources) == 0 || slices.Contains(f.Sources, source))
}

// maxFilterFetch bounds how many nearest rows a filtered search reads from the
// store while it looks for k that pass the filter.
const maxFilterFetch = 1000

// SearchFiltered embeds the query once and returns up to k nearest chunks that
// pass the filter, restricted to one CID when cid is not empty. The store ranks
// by distance only, so a filter reads more rows than k, widening until k pass,
// the store has no more, or maxFilterFetch rows were read; the result keeps the
// nearest-first order. A search with no filter reads exactly k.
func (s *Service) SearchFiltered(ctx context.Context, covers func(host string) bool, query, cid string, k int, f Filter) ([]Hit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("search: empty query")
	}
	if err := s.CheckEgress(covers); err != nil {
		return nil, err
	}
	// Fail closed before the query goes to the embedder.
	if err := s.Ready(ctx); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	vecs, err := s.embedder.Embed(ctx, covers, []string{query})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	query1 := func(n int) ([]Hit, error) {
		if cid != "" {
			return s.store.SearchCID(ctx, vecs[0], cid, n)
		}
		return s.store.Search(ctx, vecs[0], n)
	}
	if f.Empty() {
		return query1(k)
	}
	fetch := min(max(4*k, 40), maxFilterFetch)
	for {
		hits, err := query1(fetch)
		if err != nil {
			return nil, err
		}
		matched := slices.DeleteFunc(slices.Clone(hits), func(h Hit) bool { return !f.Match(h) })
		if len(matched) >= k {
			return matched[:k], nil
		}
		if len(hits) < fetch || fetch >= maxFilterFetch {
			return matched, nil
		}
		fetch = min(4*fetch, maxFilterFetch)
	}
}
