package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// runTable holds the state of the bulk indexing run. There is one record, the
// current or last run: a new run replaces it.
const (
	runTable    = "ipfs_bulk_run"
	currentRun  = "current"
	runDefinesQ = "DEFINE TABLE IF NOT EXISTS " + runTable + " SCHEMALESS;"
)

// Run states, the contract's values of ipfs_index_bulk_status run.state.
const (
	RunRunning   = "running"
	RunPaused    = "paused"
	RunCompleted = "completed"
	RunCancelled = "cancelled"
	RunFailed    = "failed"
)

// Bulk sources, the contract's embeddings.bulk.sources values.
const (
	SourcePins = "pins"
	SourceMFS  = "mfs"
)

// RunSources lists the bulk sources in the order a run walks them.
var RunSources = []string{SourcePins, SourceMFS}

// RunCaps are the effective limits of one run, already clamped to the
// configured embeddings.bulk caps.
type RunCaps struct {
	MaxFiles       int   `json:"max_files"`
	MaxBytes       int64 `json:"max_bytes"`
	FilesPerMinute int   `json:"files_per_minute"`
}

// RunCursor says where a run stands in its walk: the source it is in and the
// last item it finished there (a pin CID, or an MFS path). A resumed run starts
// after it; files already indexed are skipped anyway, so the cursor only saves
// the walk. Inside a pinned directory, Pin is the pin being walked and Inside the
// slash-separated entry names from its root down to the last finished entry, so
// a resume continues in the middle of the pin instead of restarting it; After is
// then the last pin finished before it.
type RunCursor struct {
	Source string `json:"source"`
	After  string `json:"after"`
	Pin    string `json:"pin,omitempty"`
	Inside string `json:"inside,omitempty"`
}

// Run is the persisted state of the bulk indexing run (ADR-0011 V4).
type Run struct {
	ID        string    `json:"run_id"`
	State     string    `json:"state"`
	StartedBy string    `json:"started_by"`
	Sources   []string  `json:"sources"`
	MFSRoot   string    `json:"mfs_root,omitempty"`
	Caps      RunCaps   `json:"caps"`
	Cursor    RunCursor `json:"cursor"`

	FilesSeen     int   `json:"files_seen"`
	FilesIndexed  int   `json:"files_indexed"`
	FilesSkipped  int   `json:"files_skipped"`
	ChunksIndexed int   `json:"chunks_indexed"`
	BytesRead     int64 `json:"bytes_read"`
	// SkippedByReason counts the skipped files per skipped_reason.
	SkippedByReason map[string]int `json:"skipped_by_reason,omitempty"`
	LastError       string         `json:"last_error,omitempty"`
}

// Attempted is how many files the run has tried: the files seen, less the ones
// that were already indexed and the entries that are not UnixFS files at all. It
// is what max_files_per_run bounds, so a second run over the same node reaches
// new files instead of re-counting the done ones.
func (r Run) Attempted() int {
	return r.FilesSeen - r.SkippedByReason[SkipAlreadyIndexed] - r.SkippedByReason[SkipNotAFile]
}

// Active reports whether the run blocks a new one: it is running or paused.
func (r Run) Active() bool { return r.State == RunRunning || r.State == RunPaused }

// RunStore keeps the state of the bulk run in the vector store, so it survives
// a restart of ipfs mcp.
type RunStore interface {
	// SaveRun replaces the stored run.
	SaveRun(ctx context.Context, run Run) error
	// LoadRun returns the stored run, or nil when none was ever started.
	LoadRun(ctx context.Context) (*Run, error)
}

// SaveRun implements RunStore.
func (s *surrealStore) SaveRun(ctx context.Context, run Run) error {
	if run.ID == "" {
		return errors.New("save run: empty run ID")
	}
	_, err := s.c.query(ctx, "UPSERT type::record('"+runTable+"', '"+currentRun+"') CONTENT $run;", map[string]any{"run": run})
	if err != nil {
		return fmt.Errorf("save bulk run: %w", err)
	}
	return nil
}

// LoadRun implements RunStore.
func (s *surrealStore) LoadRun(ctx context.Context) (*Run, error) {
	res, err := s.c.query(ctx, "SELECT * FROM type::record('"+runTable+"', '"+currentRun+"');", nil)
	if err != nil {
		return nil, fmt.Errorf("load bulk run: %w", err)
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("load bulk run: %d results, want 1", len(res))
	}
	var rows []Run
	if err := json.Unmarshal(res[0], &rows); err != nil {
		return nil, fmt.Errorf("load bulk run: %w", err)
	}
	if len(rows) == 0 || rows[0].ID == "" {
		return nil, nil
	}
	return &rows[0], nil
}

// Runs returns the store's run state, or nil when the store keeps none (a
// test store that does not implement RunStore).
func (s *Service) Runs() RunStore {
	rs, _ := s.store.(RunStore)
	return rs
}
