package server

import (
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Execution states are separate from approval's StateExecuted, which consumes
// the authorization before the handler starts.
const (
	ExecutionNotStarted = "not_started"
	ExecutionRunning    = "running"
	ExecutionSucceeded  = "succeeded"
	ExecutionFailed     = "failed"
	ExecutionUnknown    = "unknown"

	// One server-wide bound covers every resolved source observation.
	decisionHistoryLimit = 200
	decisionHistoryTTL   = 7 * 24 * time.Hour
)

// SafeExecutionError contains bounded public facts, never an RPC response body.
type SafeExecutionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ReceiptReference is an actual identifier returned by the successful handler.
type ReceiptReference struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// SuccessfulReceipt binds an actual successful invocation to its reviewed effect.
type SuccessfulReceipt struct {
	ID           string             `json:"id"`
	DecisionID   string             `json:"decision_id"`
	Tool         string             `json:"tool"`
	EffectDigest string             `json:"effect_digest"`
	CompletedAt  time.Time          `json:"completed_at"`
	Outcome      string             `json:"outcome"`
	Effect       string             `json:"effect"`
	Provenance   string             `json:"provenance"`
	ResultRefs   []ReceiptReference `json:"result_refs"`
}

// ExecutionRecord describes real handler progress, never approval authority.
type ExecutionRecord struct {
	State       string              `json:"state"`
	StartedAt   *time.Time          `json:"started_at,omitempty"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	Receipt     *SuccessfulReceipt  `json:"receipt,omitempty"`
	Error       *SafeExecutionError `json:"error,omitempty"`
}

// ToolOutcomeMetadata is the contract's current-call result._meta["ipfs-mcp"]
// envelope. It contains no authorization handle or raw tool data.
type ToolOutcomeMetadata struct {
	ConfirmationID string           `json:"confirmation_id"`
	EffectDigest   string           `json:"effect_digest"`
	Execution      *ExecutionRecord `json:"execution"`
}

// retainedDecision is the single broker-owned history entry. It retains no
// token, session handle, argument payload or raw OIDC subject. The full private
// owner binding is used only by read-side filtering, never serialized.
type retainedDecision struct {
	pending     Pending
	owner       string
	tool        string
	execution   ExecutionRecord
	claim       *executionClaim
	closedAt    time.Time
	closedOrder uint64
	version     uint64
}

// executionClaim exists only for the invocation that won the atomic claim.
// It is unexported, cannot be supplied by a transport, and never invokes tools.
type executionClaim struct {
	b     *Broker
	id    string
	entry *retainedDecision
}

// decorateResult publishes only this winning invocation's detached snapshot.
// The claim retains its entry during delivery even if a busy history ring
// evicts that completed decision; this does not confer replay authority.
func (c *executionClaim) decorateResult(result *mcp.CallToolResult) {
	if c == nil || result == nil {
		return
	}
	c.b.mu.Lock()
	e := c.entry
	if e == nil || e.claim != c || !validEffectDigest(e.pending.EffectDigest) {
		c.b.mu.Unlock()
		return
	}
	meta := ToolOutcomeMetadata{ConfirmationID: c.id, EffectDigest: e.pending.EffectDigest, Execution: cloneExecution(&e.execution)}
	c.b.mu.Unlock()
	if result.Meta == nil {
		result.Meta = mcp.Meta{}
	}
	result.Meta["ipfs-mcp"] = meta
}

func cloneExecution(e *ExecutionRecord) *ExecutionRecord {
	if e == nil {
		return nil
	}
	out := *e
	if e.StartedAt != nil {
		t := *e.StartedAt
		out.StartedAt = &t
	}
	if e.CompletedAt != nil {
		t := *e.CompletedAt
		out.CompletedAt = &t
	}
	if e.Error != nil {
		v := *e.Error
		out.Error = &v
	}
	if e.Receipt != nil {
		v := *e.Receipt
		v.ResultRefs = slices.Clone(e.Receipt.ResultRefs)
		out.Receipt = &v
	}
	return &out
}

// start is called immediately before invoking the implementation. A lost
// context or an abnormal handler exit later records unknown, never a retry.
func (c *executionClaim) start() bool {
	if c == nil {
		return true // reads have no approval claim or decision execution record
	}
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	e := c.b.retained[c.id]
	if e == nil || e.claim != c || e.execution.State != ExecutionNotStarted {
		return false
	}
	now := c.b.now().UTC()
	e.execution = ExecutionRecord{State: ExecutionRunning, StartedAt: &now}
	e.version = c.b.nextVersionLocked()
	c.b.publishDecisionLocked(e.owner, c.id, ExecutionRunning)
	return true
}

// finish stores the actual outcome before result decoration or serialization.
// The first terminal report wins; later delivery failures cannot undo success.
func (c *executionClaim) finish(state string, publicError *SafeExecutionError, effect string, refs []ReceiptReference) {
	if c == nil {
		return
	}
	c.b.mu.Lock()
	defer c.b.mu.Unlock()
	e := c.b.retained[c.id]
	if e == nil || e.claim != c || e.execution.State != ExecutionRunning {
		return
	}
	now := c.b.now().UTC()
	if now.Before(*e.execution.StartedAt) {
		now = *e.execution.StartedAt
	}
	if state == ExecutionSucceeded && (!validEffectDigest(e.pending.EffectDigest) || e.tool == "") {
		state = ExecutionUnknown
		publicError = &SafeExecutionError{Code: CodeInternal, Message: "The operation returned success, but its review binding could not be established."}
	}
	e.execution.State = state
	e.execution.CompletedAt = &now
	if publicError != nil {
		v := *publicError
		e.execution.Error = &v
	}
	if state == ExecutionSucceeded {
		if refs == nil {
			refs = []ReceiptReference{}
		}
		e.execution.Receipt = &SuccessfulReceipt{
			ID: "rc_" + randomID(16), DecisionID: c.id, Tool: e.tool,
			EffectDigest: e.pending.EffectDigest, CompletedAt: now,
			Outcome: ExecutionSucceeded, Effect: effect, Provenance: "node_handler", ResultRefs: slices.Clone(refs),
		}
	}
	c.b.closeRetainedLocked(e, now)
	if !c.b.historyEnabled {
		c.b.removeRetainedLocked(c.id)
	}
	c.b.pruneRetainedLocked()
	c.b.publishDecisionLocked(e.owner, c.id, state)
}

func (c *executionClaim) unknownIfUnfinished() {
	c.finish(ExecutionUnknown, &SafeExecutionError{Code: CodeInternal, Message: "The operation's outcome could not be established. Do not retry the write automatically."}, "", nil)
}
