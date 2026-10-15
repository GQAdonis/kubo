package server

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// This file is built with and without the noagent tag, like index.go. It holds
// the three bulk indexing tools; the run engine is in index_bulk.go.

// bulkTools are the tools of the bulk indexing run (embeddings.bulk.enabled).
var bulkTools = []string{"ipfs_index_bulk_start", "ipfs_index_bulk_status", "ipfs_index_bulk_cancel"}

// BulkTools returns the names of the bulk indexing tools, which
// embeddings.bulk.enabled: false removes from the tool set and from the RPC
// paths ipfs mcp --print-rpc-allowed-paths prints.
func BulkTools() []string { return slices.Clone(bulkTools) }

// IndexTools returns the names of every index-group tool, which are offered only
// with an embeddings section.
func IndexTools() []string {
	return append([]string{"ipfs_index_search", "ipfs_index_cid", "ipfs_index_status", "ipfs_index_reconcile"}, bulkTools...)
}

func isBulkTool(name string) bool { return slices.Contains(bulkTools, name) }

func bulkImpls(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "ipfs_index_bulk_start", Call: s.indexBulkStart},
		{Name: "ipfs_index_bulk_status", Call: s.indexBulkStatus},
		{Name: "ipfs_index_bulk_cancel", Call: s.indexBulkCancel},
	}
}

// bulkOffered reports whether the bulk tools are listed once embeddings are
// configured: embeddings.bulk.enabled is not false.
func (s *Server) bulkOffered() bool { return s.bulk.cfg.IsEnabled() }

// runStore returns the store of the run state, or a tool_unavailable error.
func (s *Server) runStore() (*embeddings.Service, embeddings.RunStore, error) {
	svc := s.embeddingsExt
	if svc == nil {
		return nil, nil, notConfigured()
	}
	rs := svc.Runs()
	if rs == nil {
		return nil, nil, &ToolError{Code: CodeToolUnavailable, Message: "the vector store keeps no bulk run state"}
	}
	return svc, rs, nil
}

// --- shapes ---------------------------------------------------------------

type bulkCapsView struct {
	MaxFiles       int   `json:"max_files"`
	MaxBytes       int64 `json:"max_bytes"`
	FilesPerMinute int   `json:"files_per_minute"`
}

func capsView(c embeddings.RunCaps) bulkCapsView {
	return bulkCapsView{MaxFiles: c.MaxFiles, MaxBytes: c.MaxBytes, FilesPerMinute: c.FilesPerMinute}
}

// bulkRunView is the run object of ipfs_index_bulk_status.
type bulkRunView struct {
	RunID           string         `json:"run_id"`
	State           string         `json:"state"`
	StartedBy       string         `json:"started_by"`
	Sources         []string       `json:"sources"`
	MFSRoot         string         `json:"mfs_root,omitempty"`
	Caps            bulkCapsView   `json:"caps"`
	FilesSeen       int            `json:"files_seen"`
	FilesIndexed    int            `json:"files_indexed"`
	FilesSkipped    int            `json:"files_skipped"`
	ChunksIndexed   int            `json:"chunks_indexed"`
	BytesRead       int64          `json:"bytes_read"`
	SkippedByReason map[string]int `json:"skipped_by_reason,omitempty"`
	LastError       string         `json:"last_error,omitempty"`
}

func runView(r embeddings.Run) bulkRunView {
	return bulkRunView{
		RunID: r.ID, State: r.State, StartedBy: r.StartedBy, Sources: nonNil(r.Sources), MFSRoot: r.MFSRoot,
		Caps: capsView(r.Caps), FilesSeen: r.FilesSeen, FilesIndexed: r.FilesIndexed, FilesSkipped: r.FilesSkipped,
		ChunksIndexed: r.ChunksIndexed, BytesRead: r.BytesRead, SkippedByReason: r.SkippedByReason, LastError: r.LastError,
	}
}

// --- recovery ---------------------------------------------------------------

// projectRun is the run as a reader should see it: a run stored as running
// while live is false (this process has no runner for it) is paused, because
// the process that ran it stopped and nothing resumes a run without a confirmed
// request (ADR-0011 V4). It changes nothing in the store.
func projectRun(run *embeddings.Run, live bool) *embeddings.Run {
	if run == nil || run.State != embeddings.RunRunning || live {
		return run
	}
	p := *run
	p.State = embeddings.RunPaused
	return &p
}

// snapshotRun loads the stored run and projects it, holding the bulk lock so a
// start that is just installing its runner is not seen half done. A read never
// writes: correcting the stored state is the work of recoverBulkRun and of a
// start, which own the lock.
func (s *Server) snapshotRun(ctx context.Context, rs embeddings.RunStore) (*embeddings.Run, error) {
	s.bulk.mu.Lock()
	defer s.bulk.mu.Unlock()
	run, err := rs.LoadRun(ctx)
	if err != nil {
		return nil, indexErr(err)
	}
	return projectRun(run, s.bulk.runner != nil), nil
}

// pauseStaleRun loads the stored run and, when it says running but no runner
// is live, stores it as paused. The caller holds s.bulk.mu.
func (s *Server) pauseStaleRun(ctx context.Context, rs embeddings.RunStore) (*embeddings.Run, error) {
	run, err := rs.LoadRun(ctx)
	if err != nil {
		return nil, indexErr(err)
	}
	if p := projectRun(run, s.bulk.runner != nil); p != run {
		if err := rs.SaveRun(ctx, *p); err != nil {
			return nil, indexErr(err)
		}
		return p, nil
	}
	return run, nil
}

// recoverBulkRun pauses a run that the previous ipfs mcp left running, at
// start, so the stored state is true before anyone asks.
func (s *Server) recoverBulkRun(ctx context.Context) {
	if !s.bulkOffered() || s.embeddingsExt == nil || s.embeddingsExt.Runs() == nil {
		return
	}
	s.bulk.mu.Lock()
	run, err := s.pauseStaleRun(ctx, s.embeddingsExt.Runs())
	s.bulk.mu.Unlock()
	switch {
	case err != nil:
		s.log.Warn("bulk index: cannot check the stored run", "error", clipMessage(err.Error()))
	case run != nil && run.State == embeddings.RunPaused:
		s.log.Info("bulk index: a run is paused; resume it with ipfs_index_bulk_start and resume true", "run", run.ID)
	}
}

// --- ipfs_index_bulk_start ---------------------------------------------------

type bulkStartInput struct {
	Sources        []string `json:"sources"`
	MFSRoot        string   `json:"mfs_root"`
	MaxFiles       *int     `json:"max_files"`
	MaxBytes       *int64   `json:"max_bytes"`
	FilesPerMinute *int     `json:"files_per_minute"`
	Resume         bool     `json:"resume"`
}

type bulkStartOutput struct {
	RunID       string       `json:"run_id"`
	State       string       `json:"state"`
	Resumed     bool         `json:"resumed"`
	Sources     []string     `json:"sources"`
	MFSRoot     string       `json:"mfs_root,omitempty"`
	Caps        bulkCapsView `json:"caps"`
	EgressHosts []string     `json:"egress_hosts,omitempty"`
}

// plan is the effective run for an input: the sources, MFS root and caps in
// effect after clamping to the configured embeddings.bulk values.
type bulkPlan struct {
	sources []string
	mfsRoot string
	caps    embeddings.RunCaps
}

// planFor resolves the input against the configuration. A run never exceeds a
// configured cap, never walks a source the configuration leaves out and never
// leaves the configured MFS root.
func (s *Server) planFor(in bulkStartInput) (bulkPlan, error) {
	cfg := s.bulk.cfg
	allowed := cfg.EffectiveSources()
	p := bulkPlan{sources: in.Sources, caps: cfg.Caps()}
	if len(p.sources) == 0 {
		p.sources = allowed
	}
	for _, src := range p.sources {
		if !slices.Contains(allowed, src) {
			return p, invalidArgument(fmt.Sprintf("source %q is not enabled in embeddings.bulk.sources", src))
		}
	}
	if len(p.sources) == 0 {
		return p, &ToolError{Code: CodeFailedPrecondition, Message: "embeddings.bulk.sources is empty, so a run would index nothing"}
	}
	slices.SortFunc(p.sources, func(a, b string) int {
		return slices.Index(embeddings.RunSources, a) - slices.Index(embeddings.RunSources, b)
	})
	root := cfg.EffectiveMFSRoot()
	if in.MFSRoot != "" {
		if !withinRoot(root, in.MFSRoot) {
			return p, invalidArgument(fmt.Sprintf("mfs_root %q is outside embeddings.bulk.mfs_root %q", in.MFSRoot, root))
		}
		root = in.MFSRoot
	}
	if slices.Contains(p.sources, embeddings.SourceMFS) {
		p.mfsRoot = root
	}
	if in.MaxFiles != nil {
		p.caps.MaxFiles = min(p.caps.MaxFiles, *in.MaxFiles)
	}
	if in.MaxBytes != nil {
		p.caps.MaxBytes = min(p.caps.MaxBytes, *in.MaxBytes)
	}
	if in.FilesPerMinute != nil {
		p.caps.FilesPerMinute = min(p.caps.FilesPerMinute, *in.FilesPerMinute)
	}
	return p, nil
}

// withinRoot reports whether p is root or a path under it.
func withinRoot(root, p string) bool {
	root = strings.TrimSuffix(root, "/")
	return root == "" || p == root || strings.HasPrefix(p, root+"/")
}

// egressHosts lists the non-loopback hosts the run may contact.
func (s *Server) egressHosts(svc *embeddings.Service) []string {
	var hosts []string
	for _, h := range svc.StageHosts() {
		if !IsLocalHost(h) {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// bulkPreflight is the checks of a start that cost nothing and change nothing,
// run before the confirmation is asked: a start the calling principal cannot
// make is refused up front, not after a person has approved it.
func (s *Server) bulkPreflight(ctx context.Context, raw json.RawMessage) error {
	var in bulkStartInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return invalidArgument(err.Error())
	}
	svc, _, err := s.runStore()
	if err != nil {
		return err
	}
	if err := svc.CheckStageEgress(callerCovers(ctx)); err != nil {
		return indexErr(err)
	}
	if in.Resume {
		// Refuse a resume that cannot continue before anyone is asked to confirm
		// it: nothing paused, or a paused run the configuration no longer allows.
		stored, err := s.snapshotRun(ctx, svc.Runs())
		if err != nil {
			return err
		}
		_, err = s.resumedRun(in, stored, "")
		return err
	}
	_, err = s.planFor(in)
	return err
}

func (s *Server) indexBulkStart(ctx context.Context, raw json.RawMessage) (any, error) {
	var in bulkStartInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	svc, rs, err := s.runStore()
	if err != nil {
		return nil, err
	}
	covers := callerCovers(ctx)
	if err := svc.CheckStageEgress(covers); err != nil {
		return nil, indexErr(err)
	}
	s.bulk.mu.Lock()
	defer s.bulk.mu.Unlock()
	if s.bulk.runner != nil {
		return nil, &ToolError{Code: CodeFailedPrecondition, Message: "a bulk run is already running; cancel it with ipfs_index_bulk_cancel or wait for it to end"}
	}
	stored, err := s.pauseStaleRun(ctx, rs) // no runner: checked above under the lock
	if err != nil {
		return nil, err
	}
	who := PrincipalFromContext(ctx).ID
	var run embeddings.Run
	switch {
	case in.Resume:
		run, err = s.resumedRun(in, stored, who)
	default:
		run, err = s.newRun(in, stored, who)
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.startBulkRun(svc, rs, covers, run); err != nil {
		return nil, err
	}
	return bulkStartOutput{
		RunID: run.ID, State: embeddings.RunRunning, Resumed: in.Resume, Sources: run.Sources, MFSRoot: run.MFSRoot,
		Caps: capsView(run.Caps), EgressHosts: s.egressHosts(svc),
	}, nil
}

// newRun makes the state of a new run, refused while another is active.
func (s *Server) newRun(in bulkStartInput, stored *embeddings.Run, who string) (embeddings.Run, error) {
	if stored != nil && stored.Active() {
		return embeddings.Run{}, &ToolError{
			Code:    CodeFailedPrecondition,
			Message: fmt.Sprintf("a bulk run (%s) is %s; resume it with resume true or cancel it with ipfs_index_bulk_cancel first", stored.ID, stored.State),
		}
	}
	p, err := s.planFor(in)
	if err != nil {
		return embeddings.Run{}, err
	}
	return embeddings.Run{
		ID: uuid.NewString(), State: embeddings.RunRunning, StartedBy: who,
		Sources: p.sources, MFSRoot: p.mfsRoot, Caps: p.caps,
	}, nil
}

// resumedRun makes the state of a paused run, continuing from its cursor. It
// keeps what the run started with only as far as the configuration still allows:
// its sources are narrowed to embeddings.bulk.sources, a stored MFS root outside
// embeddings.bulk.mfs_root is refused, and its caps are clamped again.
func (s *Server) resumedRun(in bulkStartInput, stored *embeddings.Run, who string) (embeddings.Run, error) {
	if len(in.Sources) > 0 || in.MFSRoot != "" || in.MaxFiles != nil || in.MaxBytes != nil || in.FilesPerMinute != nil {
		return embeddings.Run{}, invalidArgument("resume continues the paused run with the sources and caps it started with; pass only resume")
	}
	if stored == nil || stored.State != embeddings.RunPaused {
		return embeddings.Run{}, &ToolError{Code: CodeFailedPrecondition, Message: "there is no paused bulk run to resume; start a new one without resume"}
	}
	cfg := s.bulk.cfg
	run := *stored
	allowed := cfg.EffectiveSources()
	run.Sources = slices.DeleteFunc(slices.Clone(stored.Sources), func(src string) bool { return !slices.Contains(allowed, src) })
	if len(run.Sources) == 0 {
		return embeddings.Run{}, &ToolError{
			Code:    CodeFailedPrecondition,
			Message: "none of the paused run's sources is enabled in embeddings.bulk.sources any more; cancel it with ipfs_index_bulk_cancel and start a new run",
		}
	}
	if slices.Contains(run.Sources, embeddings.SourceMFS) {
		if root := cfg.EffectiveMFSRoot(); !withinRoot(root, stored.MFSRoot) {
			return embeddings.Run{}, &ToolError{
				Code:    CodeFailedPrecondition,
				Message: fmt.Sprintf("the paused run walks MFS from %q, which is outside embeddings.bulk.mfs_root %q; cancel it with ipfs_index_bulk_cancel and start a new run", stored.MFSRoot, root),
			}
		}
	} else {
		run.MFSRoot = ""
	}
	if !slices.Contains(run.Sources, run.Cursor.Source) {
		run.Cursor = embeddings.RunCursor{}
	}
	run.Caps = clampCaps(run.Caps, cfg.Caps())
	run.State, run.StartedBy, run.LastError = embeddings.RunRunning, who, ""
	return run, nil
}

// clampCaps lowers a run's stored caps to the configured ones.
func clampCaps(run, cfg embeddings.RunCaps) embeddings.RunCaps {
	return embeddings.RunCaps{
		MaxFiles: min(run.MaxFiles, cfg.MaxFiles), MaxBytes: min(run.MaxBytes, cfg.MaxBytes),
		FilesPerMinute: min(run.FilesPerMinute, cfg.FilesPerMinute),
	}
}

// --- ipfs_index_bulk_status ----------------------------------------------------

type bulkStatusOutput struct {
	Enabled bool         `json:"enabled"`
	Run     *bulkRunView `json:"run,omitempty"`
}

func (s *Server) indexBulkStatus(ctx context.Context, _ json.RawMessage) (any, error) {
	_, rs, err := s.runStore()
	if err != nil {
		return nil, err
	}
	out := bulkStatusOutput{Enabled: s.bulkOffered()}
	run, err := s.snapshotRun(ctx, rs)
	if err != nil {
		return nil, err
	}
	if run != nil {
		v := runView(*run)
		out.Run = &v
	}
	return out, nil
}

// --- ipfs_index_bulk_cancel ----------------------------------------------------

type bulkCancelOutput struct {
	RunID         string `json:"run_id"`
	State         string `json:"state"`
	FilesIndexed  int    `json:"files_indexed"`
	FilesSkipped  int    `json:"files_skipped"`
	ChunksIndexed int    `json:"chunks_indexed"`
}

func (s *Server) indexBulkCancel(ctx context.Context, _ json.RawMessage) (any, error) {
	_, rs, err := s.runStore()
	if err != nil {
		return nil, err
	}
	// The run records how it ended when it stops, so the stop is awaited without
	// the lock (the run takes the lock to leave). The check and the write below
	// then run under it, so a start or resume cannot slip in between.
	if r := s.bulk.active(); r != nil {
		r.stop(errBulkCancelled)
	}
	s.bulk.mu.Lock()
	defer s.bulk.mu.Unlock()
	if s.bulk.runner != nil {
		return nil, &ToolError{Code: CodeFailedPrecondition, Message: "another bulk run started while this one was being cancelled; check ipfs_index_bulk_status"}
	}
	stored, err := s.pauseStaleRun(ctx, rs)
	if err != nil {
		return nil, err
	}
	if stored == nil || !(stored.Active() || stored.State == embeddings.RunCancelled) {
		return nil, &ToolError{Code: CodeFailedPrecondition, Message: "there is no active or paused bulk run to cancel"}
	}
	if stored.State != embeddings.RunCancelled {
		stored.State = embeddings.RunCancelled
		if err := rs.SaveRun(ctx, *stored); err != nil {
			return nil, indexErr(err)
		}
	}
	return bulkCancelOutput{
		RunID: stored.ID, State: embeddings.RunCancelled, FilesIndexed: stored.FilesIndexed,
		FilesSkipped: stored.FilesSkipped, ChunksIndexed: stored.ChunksIndexed,
	}, nil
}

// --- confirmation text ------------------------------------------------------------

// summaryArgs gives the confirmation text of a bulk start the values the run
// will use, not the words "default", and appends every host it may contact. It
// leaves other tools alone. The call's argument hash, which the confirmation
// binds to, is computed from the original arguments.
func (s *Server) summaryArgs(tool string, args map[string]any) (map[string]any, string) {
	if tool != "ipfs_index_bulk_start" || s.embeddingsExt == nil {
		return args, ""
	}
	out := make(map[string]any, len(args)+6)
	for k, v := range args {
		out[k] = v
	}
	in := bulkStartInput{}
	if raw, err := json.Marshal(args); err == nil {
		_ = json.Unmarshal(raw, &in)
	}
	resume, _ := args["resume"].(bool)
	out["resume"] = resume
	var p bulkPlan
	if resume {
		// The run a resume would continue, as resumedRun would restart it:
		// narrowed to the configuration and with the caps clamped.
		run, err := s.resumedRun(bulkStartInput{Resume: true}, s.storedRunForSummary(), "")
		if err != nil {
			return out, ""
		}
		p = bulkPlan{sources: run.Sources, mfsRoot: run.MFSRoot, caps: run.Caps}
	} else if pl, err := s.planFor(in); err == nil {
		p = pl
	} else {
		return out, ""
	}
	out["sources"] = strings.Join(p.sources, ", ")
	out["mfs_root"] = cmpOr(p.mfsRoot, "(not walked)")
	out["max_files"], out["max_bytes"], out["files_per_minute"] = p.caps.MaxFiles, p.caps.MaxBytes, p.caps.FilesPerMinute
	hosts := s.egressHosts(s.embeddingsExt)
	if len(hosts) == 0 {
		return out, " Contacts only this machine."
	}
	return out, " Contacts: " + strings.Join(hosts, ", ") + "."
}

// storedRunForSummary returns the paused run a resume would continue, so the
// confirmation shows its real sources and caps; nil when there is none.
func (s *Server) storedRunForSummary() *embeddings.Run {
	if s.embeddingsExt == nil || s.embeddingsExt.Runs() == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run, err := s.snapshotRun(ctx, s.embeddingsExt.Runs())
	if err != nil {
		return nil
	}
	return run
}
