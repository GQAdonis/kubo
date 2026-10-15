package embeddings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// embedBatch is how many chunks go to the embedder in one request.
const embedBatch = 16

// Service is the embeddings feature of one ipfs mcp run: the store, the
// embedder and, in embedded mode, the supervised process. It exists only when
// the operator configured an embeddings section.
type Service struct {
	settings Settings
	store    Store
	embedder *Embedder
	sup      *Supervisor
	// storeHost is the host:port of the store when it is not on this machine.
	storeHost string
	policy    Policy
	// extractor is the xberg client, nil when no extractor is configured.
	extractor *Extractor
	// describer is the chat-model client, nil when no describer is configured.
	describer *Describer
}

// Options are test seams of Open. The zero value is the production behavior.
type Options struct {
	Logger *slog.Logger
	// HTTPClient, if set, is the base for the store and embedder clients.
	HTTPClient *http.Client
}

// Open validates the settings, starts the embedded store if configured,
// connects to the store and creates the schema. A failure leaves nothing
// running. Without settings there is no Open: callers do not construct a
// Service for an absent embeddings section.
func Open(ctx context.Context, s Settings, opts Options) (*Service, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	emb, err := NewEmbedder(s.Embedder, opts.HTTPClient)
	if err != nil {
		return nil, err
	}
	svc := &Service{settings: s, embedder: emb, policy: s.Policy.Effective()}
	var c *surrealClient
	switch s.Mode {
	case ModeEmbedded:
		sup, err := StartSupervisor(ctx, *s.Embedded, opts.Logger)
		if err != nil {
			return nil, err
		}
		svc.sup = sup
		c = newSurrealClient(sup.Endpoint(), embeddedNamespace, embeddedDatabase, parseCredential(sup.Credential()), opts.HTTPClient)
	case ModeRemote:
		r := s.Remote
		c = newSurrealClient(r.Endpoint, r.Namespace, r.Database, parseCredential(r.Credential), opts.HTTPClient)
		if u, err := url.Parse(r.Endpoint); err == nil {
			svc.storeHost = u.Host
		}
	}
	svc.store = newSurrealStore(c, s.Embedder.Dimensions)
	if err := svc.store.Bootstrap(ctx); err != nil {
		_ = svc.Close()
		return nil, fmt.Errorf("embeddings store: %w", err)
	}
	if s.Extractor != nil {
		x, err := OpenExtractor(ctx, *s.Extractor, opts.HTTPClient, opts)
		if err != nil {
			_ = svc.Close()
			return nil, fmt.Errorf("embeddings extractor: %w", err)
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
	var errs []error
	if s.extractor != nil {
		errs = append(errs, s.extractor.Close())
	}
	if s.sup != nil {
		errs = append(errs, s.sup.Close())
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

// Store gives direct access to the vector store (deletion and counts do not
// send content anywhere).
func (s *Service) Store() Store { return s.store }

// Embedder is the configured embedder.
func (s *Service) Embedder() *Embedder { return s.embedder }

// Supervisor is the embedded store's supervisor, nil in remote mode.
func (s *Service) Supervisor() *Supervisor { return s.sup }

// Describe is a one-line, secret-free summary for the startup log.
func (s *Service) Describe() string {
	where := "remote store " + s.storeHost
	if s.sup != nil {
		where = "embedded store on " + s.sup.Addr()
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
