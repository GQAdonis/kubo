//go:build !noagent

package server

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Limits of the proxied stream. UAR is the operator's own instance, but a
// broken or hostile peer must not make this process hold unbounded memory.
const (
	// maxUAREventBytes bounds one SSE line.
	maxUAREventBytes = 4 << 20
	// maxUAREvents bounds the events read from UAR for one island request,
	// across all its hops.
	maxUAREvents = 100000
	// maxUARRequestBytes bounds the bytes read from UAR for one island request,
	// across all its hops (a single hop is also bounded by maxUAREventBytes per
	// line).
	maxUARRequestBytes = 32 << 20
	// maxUARHops bounds the UAR requests one island request makes: interrupts
	// that need no person (a URL elicitation, a form kubo cannot render) are
	// answered and the run continues; a peer that keeps raising them is cut off.
	maxUARHops = 6
	// maxUARToolNames bounds the tool names remembered per thread.
	maxUARToolNames = 256
	// maxUARInterrupts bounds the interrupts of one segment.
	maxUARInterrupts = 16
	// maxUARRunError bounds the message of a relayed RUN_ERROR.
	maxUARRunError = 500
	// maxUARCardText bounds the text of an approval card taken from UAR.
	maxUARCardText = 500
	// maxUARLabelLen bounds a tool or server name taken from UAR.
	maxUARLabelLen = 64
)

// uarApprovalLabel opens the message of every approval card that comes from
// UAR. The card is answered by the person in the chat, but it is UAR's approval
// of UAR's tool call: it is not a confirmation of this node, which the gate
// decides separately when the tool runs.
const uarApprovalLabel = "UAR approval, not a node confirmation: "

// uarEventFields is, per AG-UI event type, the fields of an event relayed from
// UAR. These are the types the island renders from a run (agui.schema.json
// Event) minus RUN_*: the proxy writes RUN_STARTED, RUN_FINISHED and RUN_ERROR
// itself, with the island's thread and run ids. Every other type (CUSTOM,
// including any A2UI surface UAR sends, STATE_*, MESSAGES_SNAPSHOT, RAW, STEP_*
// and the rest) is dropped, and so is any field not listed.
var uarEventFields = map[string][]string{
	"TEXT_MESSAGE_START":   {"messageId", "role"},
	"TEXT_MESSAGE_CONTENT": {"messageId", "delta"},
	"TEXT_MESSAGE_END":     {"messageId"},
	"TOOL_CALL_START":      {"toolCallId", "toolCallName", "parentMessageId"},
	"TOOL_CALL_ARGS":       {"toolCallId", "delta"},
	"TOOL_CALL_END":        {"toolCallId"},
	"TOOL_CALL_RESULT":     {"messageId", "toolCallId", "content", "role"},
}

// UAR interrupt reasons and elicitation actions (pinned contract, "Complete
// AG-UI interrupt contract").
const (
	uarReasonToolApproval = "tool_approval"
	uarReasonElicitation  = "mcp_elicitation"
)

// uarBackendError is a failure of the UAR instance or the path to it. Its text
// is fixed per cause: an HTTP reply or a transport error from the peer can
// quote the conversation, so it is never forwarded. The one peer-written text
// that is relayed is the message of a RUN_ERROR event the agent raised itself:
// it is the only explanation of why a run failed, so it is shown after the UAR
// key and the run grant are removed from it and cut to maxUARRunError.
type uarBackendError struct{ msg string }

func (e *uarBackendError) Error() string { return e.msg }

// uarFailure turns an error of a UAR call into the error clients see, and logs
// its type and status.
func (b *uarBackend) uarFailure(err error) error {
	var (
		he       *uarHTTPError
		be       *uarBackendError
		conflict *uarSkillConflictError
	)
	switch {
	case errors.As(err, &be):
		return be
	case errors.As(err, &conflict):
		return &uarBackendError{fmt.Sprintf("The UAR instance holds a different version of the skill %q; remove it there or rename it.", conflict.Name)}
	case errors.Is(err, context.DeadlineExceeded):
		return &uarBackendError{"The run reached the time limit."}
	case errors.As(err, &he):
		b.log.Warn("UAR request failed", "op", he.Op, "status", he.Status)
		if he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden {
			return &uarBackendError{"The UAR instance refused the credential kubo presents (HTTP " + fmt.Sprint(he.Status) + ")."}
		}
		return &uarBackendError{fmt.Sprintf("The UAR instance answered HTTP %d.", he.Status)}
	}
	b.log.Warn("UAR request failed", "error_type", fmt.Sprintf("%T", err))
	return &uarBackendError{"The UAR instance could not be reached or failed the run."}
}

// uarAnswer is the answer recorded for one UAR interrupt.
type uarAnswer struct {
	Status  string
	Payload map[string]any
}

// uarPending is one interrupt UAR raised and kubo has not answered yet.
type uarPending struct {
	// ID is UAR's interrupt id, sent back on resume. ShownID is what the island
	// sees: the same id unless it is too long for the island's contract.
	ID, ShownID string
	Reason      string
	Message     string
	ToolCallID  string
	Tool        string
	ExpiresAt   string
	// ApproveKey is, for a form elicitation, the one boolean property an
	// approval sets to true.
	ApproveKey string
	// NeedsUser is true for what the island shows; false for an interrupt kubo
	// answers by itself (see classifyUARInterrupt).
	NeedsUser bool
	Answer    *uarAnswer
}

// uarThread is the proxy's record of one chat thread on the UAR instance.
type uarThread struct {
	owner string
	// client is the thread id the island chose; uarID is the id UAR sees. uarID
	// changes when the thread is abandoned (see abandonThread).
	client string
	uarID  string
	// lastRun is the run id of the UAR segment that raised the open interrupts;
	// the resume names it as its parent.
	lastRun string
	pending []*uarPending
	// shown is the ShownID of the interrupt the island was asked to answer.
	shown   string
	tools   map[string]string
	running bool
	touched time.Time
}

// shownPending returns the interrupt the island is answering, or nil.
func (t *uarThread) shownPending() *uarPending {
	for _, p := range t.pending {
		if p.ShownID == t.shown && p.Answer == nil {
			return p
		}
	}
	return nil
}

// nextToAsk returns the first unanswered interrupt that needs a person.
func (t *uarThread) nextToAsk() *uarPending {
	for _, p := range t.pending {
		if p.Answer == nil && p.NeedsUser {
			return p
		}
	}
	return nil
}

// rememberTool records the name of a tool call, so a tool_approval (which names
// only the call id) can tell the person which tool it is about.
func (t *uarThread) rememberTool(callID, name string) {
	if callID == "" || name == "" {
		return
	}
	if t.tools == nil {
		t.tools = map[string]string{}
	}
	if len(t.tools) >= maxUARToolNames {
		clear(t.tools)
	}
	t.tools[callID] = name
}

func uarThreadKey(principal, thread string) string { return principal + "\x00" + thread }

// acquireThread finds or creates the principal's thread and marks it running.
// A thread belongs to the principal that created it; another principal's
// thread id is not revealed. create=false never creates (a resume).
func (b *uarBackend) acquireThread(principal, thread string, create bool) (*uarThread, func(), error) {
	if len(thread) > maxSessionIDLen {
		return nil, nil, ErrSessionNotFound
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	for k, t := range b.threads {
		if !t.running && now.Sub(t.touched) > sessionIdleTTL {
			delete(b.threads, k)
		}
	}
	key := uarThreadKey(principal, thread)
	t, ok := b.threads[key]
	switch {
	case ok && t.running:
		return nil, nil, ErrSessionBusy
	case !ok && !create:
		return nil, nil, ErrSessionNotFound
	}
	if !ok {
		if err := b.makeRoomLocked(principal); err != nil {
			return nil, nil, err
		}
		t = &uarThread{owner: principal, client: thread, uarID: uarThreadID(b.gen, b.nextThreadSeqLocked(), principal, thread)}
		b.threads[key] = t
	}
	t.running, t.touched = true, now
	return t, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		t.running, t.touched = false, b.now()
	}, nil
}

// makeRoomLocked frees a slot for a new thread of the principal by evicting its
// own least recently used idle thread, never another principal's. Callers hold
// b.mu.
func (b *uarBackend) makeRoomLocked(principal string) error {
	own := 0
	oldest := ""
	for k, t := range b.threads {
		if t.owner != principal {
			continue
		}
		own++
		if !t.running && (oldest == "" || t.touched.Before(b.threads[oldest].touched)) {
			oldest = k
		}
	}
	if own < maxSessionsPerPrincipal && len(b.threads) < maxSessions {
		return nil
	}
	if oldest == "" {
		return ErrTooManySessions
	}
	delete(b.threads, oldest)
	return nil
}

// serve handles one POST /agui while the backend is a UAR agent. The route's
// protection and the chat-principal check already ran.
func (b *uarBackend) serve(h *aguiHandler, w http.ResponseWriter, r *http.Request, id Identity) {
	in, err := readAGUIInput(r.Body)
	if err != nil {
		h.reject(w, err)
		return
	}
	eff, refusal := b.srv.applyPolicy(r.Context(), id, false)
	if refusal != nil {
		h.reject(w, refusal)
		return
	}
	if _, retry := b.limiter.take(eff.ID); retry > 0 {
		h.reject(w, &RateLimitedError{RetryAfter: retry})
		return
	}
	// One wall-clock deadline for the whole island request: provisioning, every
	// UAR hop and the relay share it.
	ctx, cancel := context.WithTimeout(r.Context(), b.set.Limits.WallClock)
	defer cancel()
	st := newAGUIStream(w, in, nil, eff.EgressAck)
	if in.Resume != nil {
		b.resume(ctx, h, st, eff, in)
		return
	}
	b.message(ctx, h, st, eff, in)
}

// message starts a run from the thread's last user message.
func (b *uarBackend) message(ctx context.Context, h *aguiHandler, st *aguiStream, id Identity, in *aguiInput) {
	text, err := in.lastUserText()
	if err != nil {
		h.reject(st.w, err)
		return
	}
	if len(text) > maxUserMessageBytes {
		h.reject(st.w, ErrMessageTooLarge)
		return
	}
	th, release, err := b.acquireThread(id.ID, in.ThreadID, true)
	if err != nil {
		h.reject(st.w, err)
		return
	}
	defer release()
	if len(th.pending) > 0 {
		h.reject(st.w, ErrInterruptOpen)
		return
	}
	if err := b.ensureProvisioned(ctx); err != nil {
		h.reject(st.w, b.uarFailure(err))
		return
	}
	body := b.runBody(th, in.RunID, "", []any{map[string]any{"id": newMessageID(), "role": "user", "content": text}}, nil)
	b.execute(ctx, h, st, th, in, body)
}

// resume answers the interrupt the island was shown. Every answer is recorded
// here; the UAR run continues only when every interrupt of its segment has one,
// because UAR rejects a resume that does not cover all of them.
func (b *uarBackend) resume(ctx context.Context, h *aguiHandler, st *aguiStream, id Identity, in *aguiInput) {
	th, release, err := b.acquireThread(id.ID, in.ThreadID, false)
	if err != nil {
		h.reject(st.w, err)
		return
	}
	defer release()
	entry := in.Resume[0]
	cur := th.shownPending()
	switch {
	case len(th.pending) == 0:
		h.reject(st.w, ErrNoOpenInterrupt)
		return
	case cur == nil || cur.ShownID != entry.InterruptID:
		h.reject(st.w, ErrUnknownInterrupt)
		return
	case entry.Wait:
		h.reject(st.w, errAGUIInput("{wait: true} is for an interrupt that needs the system prompt; this one is answered in the chat"))
		return
	}
	approve := entry.Status == ResumeResolved && entry.Approved
	cancelled := entry.Status == ResumeCancelled
	cur.Answer = cur.answerFor(approve, cancelled)
	if !approve {
		// A denial ends the run with nothing executed: every other interrupt of
		// the segment is denied with it.
		for _, p := range th.pending {
			if p.Answer == nil {
				p.Answer = p.answerFor(false, false)
			}
		}
	}
	if next := th.nextToAsk(); next != nil {
		// More to ask: a local segment, no UAR request.
		th.shown = next.ShownID
		b.askNext(st, next)
		return
	}
	body := b.resumeBody(th, in.RunID)
	b.execute(ctx, h, st, th, in, body)
}

// askNext emits a segment that carries only the next interrupt to ask.
func (b *uarBackend) askNext(st *aguiStream, p *uarPending) {
	st.startOnce()
	st.send(evRunFinished(st.threadID, st.runID, map[string]any{
		"type":       "interrupt",
		"interrupts": []any{p.aguiInterrupt()},
	}))
}

// startOnce sends RUN_STARTED once.
func (s *aguiStream) startOnce() {
	if s.runStarted {
		return
	}
	s.runStarted = true
	s.send(evRunStarted(s.threadID, s.runID))
}

// runBody builds a RunAgentInput for the UAR path, in the shape the acceptance
// posted (tests/fixtures/uar_1_1_mcp_contract.py:95-110 initial_request and
// continuation; docs/AGUI_V1.md at the tag): protocolVersion "1.0", empty tools
// and context, no state (UAR appends state to the prompt as untrusted text, and
// kubo has none), and messages empty on a resume. The run grant (the credential
// UAR presents to this server's MCP endpoint) goes in forwardedProps.uar and is
// supplied per run, so it is never stored on the instance (contract,
// "Trusted-host MCP setup"); the fixture sends it on the resume as well.
func (b *uarBackend) runBody(th *uarThread, runID, parent string, messages []any, resume []any) map[string]any {
	body := map[string]any{
		"threadId":        th.uarID,
		"runId":           runID,
		"protocolVersion": "1.0",
		"messages":        messages,
		"tools":           []any{},
		"context":         []any{},
	}
	if parent != "" {
		body["parentRunId"] = parent
	}
	if resume != nil {
		body["resume"] = resume
	}
	if grant := b.runGrant(); grant != nil {
		body["forwardedProps"] = map[string]any{"uar": map[string]any{"mcp_servers": []any{grant}}}
	}
	return body
}

// runGrant is the explicit run grant for the kubo MCP destination, or nil when
// no principal is configured or its credential does not resolve (the agent then
// runs without kubo tools; the startup and provisioning warnings say so). The
// shape is the one the acceptance sent (acc. 597-599; docs/releases/
// v1.1.0-kubo.md "Restricted destinations ..."): name, and grant with
// credential_revision, scopes, expires_at_unix and headers.Authorization. The
// expiry is advisory on UAR's side, and the credential is the principal's
// static bearer; a leaked grant is usable until that credential is rotated.
func (b *uarBackend) runGrant() map[string]any {
	u := b.set.UAR
	if u.MCPCredential == nil {
		return nil
	}
	tok, err := u.MCPCredential.Read()
	if err != nil {
		b.log.Warn("the credential of llm.uar.mcp_principal does not resolve; the run carries no kubo MCP grant", "principal", u.MCPPrincipal)
		return nil
	}
	return map[string]any{
		"name": u.MCPName,
		"grant": map[string]any{
			"credential_revision": u.CredentialRevision,
			"scopes":              u.Scopes,
			"expires_at_unix":     b.now().Add(u.GrantTTL).Unix(),
			"headers":             map[string]string{"Authorization": "Bearer " + tok},
		},
	}
}

// resumeBody builds the resume request: the same thread, a new run id, the
// preceding run as parent, empty messages, and one entry per pending interrupt
// in the order UAR raised them (fixture continuation).
func (b *uarBackend) resumeBody(th *uarThread, runID string) map[string]any {
	entries := make([]any, 0, len(th.pending))
	for _, p := range th.pending {
		e := map[string]any{"interruptId": p.ID, "status": p.Answer.Status}
		if p.Answer.Payload != nil {
			e["payload"] = p.Answer.Payload
		}
		entries = append(entries, e)
	}
	return b.runBody(th, runID, th.lastRun, []any{}, entries)
}

// answerFor records the person's decision for the interrupt. Deny and cancel
// never carry an approval.
func (p *uarPending) answerFor(approve, cancelled bool) *uarAnswer {
	if cancelled {
		return &uarAnswer{Status: ResumeCancelled}
	}
	if p.Reason == uarReasonElicitation {
		if approve {
			return &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"action": "accept", "content": map[string]any{p.ApproveKey: true}}}
		}
		return &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"action": "decline"}}
	}
	return &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"approved": approve}}
}

// relayResult is how one UAR request ended.
type relayResult struct {
	err error
	// responded is true once UAR answered the request with a 200 event stream:
	// the interrupts it was resuming are settled on its side. Any other outcome
	// leaves them open there (see settle).
	responded bool
	// refused is true when UAR refused the request with a status that says the
	// thread cannot be continued (400, 404, 409, 410, 422): its interrupts are
	// gone or never will match.
	refused bool
	// interrupts are the raw interrupts of a segment that ended with an interrupt
	// outcome (not yet relayed to the island).
	interrupts []any
	// done is true when the run reached its end (RUN_FINISHED success or
	// RUN_ERROR) and the island has been told.
	done bool
}

// uarBudget is what one island request may read from UAR across all its hops.
type uarBudget struct{ events, bytes int }

// execute sends body to UAR and relays the run, looping over the interrupts
// that need no person. It ends the island's stream or HTTP request. The
// context carries the request's one wall-clock deadline.
func (b *uarBackend) execute(ctx context.Context, h *aguiHandler, st *aguiStream, th *uarThread, in *aguiInput, body map[string]any) {
	bud := &uarBudget{}
	for hop := 0; hop < maxUARHops; hop++ {
		res := b.relay(ctx, st, th, body, bud)
		b.settle(th, res)
		if res.err != nil {
			b.fail(h, st, res.err)
			return
		}
		if res.done {
			return
		}
		pending, err := b.parseInterrupts(th, res.interrupts)
		if err != nil {
			b.fail(h, st, err)
			return
		}
		th.pending = pending
		if next := th.nextToAsk(); next != nil {
			th.shown = next.ShownID
			b.askNext(st, next)
			return
		}
		body = b.resumeBody(th, fmt.Sprintf("%s-h%d", in.RunID, hop+1))
	}
	b.fail(h, st, &uarBackendError{"The UAR run kept raising interrupts that need no person."})
}

// settle updates the thread's record of its open interrupts after one UAR
// request. The interrupts are forgotten only when UAR answered with a 200 event
// stream (it consumed the resume) or refused the thread outright. A transport
// error, a 401 or 403, a 5xx or a rate limit leaves them open on UAR, so the
// thread keeps them: the answers the person gave are withdrawn and the same
// interrupt is asked again, and a new message is refused until it is answered.
// Forgetting them would let the next message reach a UAR thread that still
// holds them.
func (b *uarBackend) settle(th *uarThread, res relayResult) {
	switch {
	case res.responded:
		th.pending, th.shown = nil, ""
	case res.refused && len(th.pending) > 0:
		b.abandonThread(th)
	case res.err != nil && len(th.pending) > 0:
		b.reopen(th)
	}
}

// reopen withdraws the answers of the interrupts a person gave, so the failed
// resume can be tried again. Interrupts kubo answered by itself keep their
// answers. When no interrupt needs a person, nothing could ever clear the
// thread: it is abandoned.
func (b *uarBackend) reopen(th *uarThread) {
	for _, p := range th.pending {
		if p.NeedsUser {
			p.Answer = nil
		}
	}
	if next := th.nextToAsk(); next != nil {
		th.shown = next.ShownID
		return
	}
	b.abandonThread(th)
}

// abandonThread forgets the thread's open interrupts and moves it to a fresh
// UAR thread, so a later message does not meet the old one.
func (b *uarBackend) abandonThread(th *uarThread) {
	th.pending, th.shown, th.lastRun = nil, "", ""
	b.mu.Lock()
	th.uarID = uarThreadID(b.gen, b.nextThreadSeqLocked(), th.owner, th.client)
	b.mu.Unlock()
}

// fail ends the island's request after a failure: an HTTP error before the
// stream began, RUN_ERROR after.
func (b *uarBackend) fail(h *aguiHandler, st *aguiStream, err error) {
	be, _ := b.uarFailure(err).(*uarBackendError)
	if st.started {
		st.closeText()
		st.send(evRunError(be.msg, "backend_error"))
		return
	}
	h.reject(st.w, be)
}

// refusedStatuses are the UAR statuses that end a thread's open interrupts.
var refusedStatuses = []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusGone, http.StatusUnprocessableEntity}

// relay performs one UAR request and relays its events to the island. A
// RUN_FINISHED with an interrupt outcome is not relayed: it is returned, so the
// caller can translate and, where no person is needed, continue.
func (b *uarBackend) relay(ctx context.Context, st *aguiStream, th *uarThread, body map[string]any, bud *uarBudget) relayResult {
	raw, err := json.Marshal(body)
	if err != nil {
		return relayResult{err: err}
	}
	// /api/uar/ag-ui nest: src/server.rs:1756; run body and events: docs/AGUI_V1.md.
	req, err := b.cli.newRequest(ctx, http.MethodPost, "/api/uar/ag-ui/agents/"+url.PathEscape(b.set.UAR.AgentID), raw)
	if err != nil {
		return relayResult{err: err}
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := b.cli.hc.Do(req)
	if err != nil {
		return relayResult{err: b.uarFailure(unwrapURLError(err))}
	}
	defer resp.Body.Close()
	var res relayResult
	if resp.StatusCode == http.StatusNotFound {
		// The instance lost the agent (or never had it): provision again on the
		// next run.
		b.forgetProvisioned()
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxUARReply))
		res.refused = slices.Contains(refusedStatuses, resp.StatusCode)
		res.err = b.uarFailure(&uarHTTPError{Op: "run", Status: resp.StatusCode})
		return res
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != "text/event-stream" {
		res.err = b.uarFailure(errors.New("UAR answered a run with a body that is not an event stream"))
		return res
	}
	// UAR accepted the request: what it resumed is settled, and this run is the
	// one a later resume names as its parent.
	res.responded = true
	if id, _ := body["runId"].(string); id != "" {
		th.lastRun = id
	}
	scrub := b.newScrubber()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxUAREventBytes)
	for sc.Scan() {
		line := sc.Text()
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // comments, keepalives and other SSE fields
		}
		bud.events++
		bud.bytes += len(line)
		if bud.events > maxUAREvents || bud.bytes > maxUARRequestBytes {
			res.err = &uarBackendError{"The UAR run sent more data than one request allows."}
			return res
		}
		var ev aguiEvent
		if json.Unmarshal([]byte(strings.TrimSpace(data)), &ev) != nil {
			res.err = b.uarFailure(errors.New("UAR sent an event that is not JSON"))
			return res
		}
		typ, _ := ev["type"].(string)
		switch typ {
		case "":
			res.err = b.uarFailure(errors.New("UAR sent an event without a type"))
			return res
		case "RUN_STARTED":
			st.startOnce()
		case "RUN_FINISHED":
			outcome, _ := ev["outcome"].(map[string]any)
			if t, _ := outcome["type"].(string); t == "interrupt" {
				list, _ := outcome["interrupts"].([]any)
				if len(list) == 0 || len(list) > maxUARInterrupts {
					res.err = b.uarFailure(errors.New("UAR sent an interrupt outcome with no usable interrupts"))
					return res
				}
				st.closeText()
				res.interrupts = list
				return res
			}
			st.startOnce()
			st.closeText()
			st.send(evRunFinished(st.threadID, st.runID, map[string]any{"type": "success"}))
			res.done = true
			return res
		case "RUN_ERROR":
			st.startOnce()
			st.closeText()
			msg, _ := ev["message"].(string)
			st.send(evRunError(trimSnippet(scrub.text(msg), maxUARRunError), "backend_error"))
			res.done = true
			return res
		default:
			clean, ok := relayedEvent(ev, typ, scrub)
			if !ok {
				continue
			}
			if typ == "TOOL_CALL_START" {
				id, _ := clean["toolCallId"].(string)
				name, _ := clean["toolCallName"].(string)
				th.rememberTool(id, name)
			}
			st.startOnce()
			st.send(clean)
		}
	}
	if err := sc.Err(); err != nil {
		res.err = b.uarFailure(err)
		return res
	}
	res.err = b.uarFailure(errors.New("UAR ended the stream before the run finished"))
	return res
}

// relayedEvent returns the event the island is shown for a UAR event: only the
// allowlisted types, only their listed fields, every string cleaned of the UAR
// key and the run grant. ok is false for a type that is not relayed.
func relayedEvent(ev aguiEvent, typ string, scrub *uarScrubber) (aguiEvent, bool) {
	fields, ok := uarEventFields[typ]
	if !ok {
		return nil, false
	}
	out := aguiEvent{"type": typ}
	for _, f := range fields {
		if v, present := ev[f]; present {
			out[f] = scrub.value(v)
		}
	}
	if ts, ok := ev["timestamp"].(float64); ok {
		out["timestamp"] = ts
	}
	return out, true
}

// uarScrubber removes the secrets kubo holds for UAR from text that came back
// from it: the UAR credential and the credential of the run grant. It matches
// each secret whole inside one string; a secret that UAR splits across two
// events is not recognised.
type uarScrubber struct{ secrets []string }

// minScrubbedSecret is the shortest secret the scrubber replaces: a shorter
// value would rewrite ordinary text.
const minScrubbedSecret = 8

// newScrubber reads the secrets as they are now (both can be rotated).
func (b *uarBackend) newScrubber() *uarScrubber {
	var secrets []string
	add := func(s string) {
		if len(s) >= minScrubbedSecret && !slices.Contains(secrets, s) {
			secrets = append(secrets, s)
		}
	}
	add(b.cli.token())
	if cred := b.set.UAR.MCPCredential; cred != nil {
		if tok, err := cred.Read(); err == nil {
			add(tok)
		}
	}
	return &uarScrubber{secrets: secrets}
}

func (s *uarScrubber) text(in string) string {
	for _, sec := range s.secrets {
		in = strings.ReplaceAll(in, sec, "[redacted]")
	}
	return in
}

// value cleans every string inside a decoded JSON value.
func (s *uarScrubber) value(v any) any {
	switch x := v.(type) {
	case string:
		return s.text(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = s.value(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[s.text(k)] = s.value(e)
		}
		return out
	}
	return v
}

// parseInterrupts reads the interrupts of an interrupt outcome. An interrupt
// kubo cannot show is answered by kubo, and is never approved.
func (b *uarBackend) parseInterrupts(th *uarThread, list []any) ([]*uarPending, error) {
	out := make([]*uarPending, 0, len(list))
	seen := map[string]struct{}{}
	scrub := b.newScrubber()
	for _, raw := range list {
		m, _ := raw.(map[string]any)
		p, ok := classifyUARInterrupt(m, th.tools)
		if !ok {
			return nil, &uarBackendError{"UAR sent an interrupt without an id."}
		}
		p.Message = trimSnippet(scrub.text(p.Message), maxUARCardText)
		p.Tool = scrub.text(p.Tool)
		if _, dup := seen[p.ID]; dup {
			return nil, &uarBackendError{"UAR sent the same interrupt twice."}
		}
		seen[p.ID] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

var uarShownIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// classifyUARInterrupt reads one UAR interrupt. ok is false when it has no
// usable id. Tool approvals and boolean-confirmation elicitations (kubo's own
// confirmation is one) are shown to the person. Everything else is answered
// without them and never approved:
//   - an elicitation whose connection is gone (resolvable false) is cancelled;
//   - a URL elicitation is declined: the link is untrusted and never fetched;
//   - a form that asks for anything but one yes/no is cancelled: the chat cannot
//     collect other input;
//   - an unknown reason is cancelled.
func classifyUARInterrupt(m map[string]any, tools map[string]string) (*uarPending, bool) {
	id, _ := m["id"].(string)
	if id == "" || len(id) > 2*maxAGUIIDLen {
		return nil, false
	}
	p := &uarPending{ID: id, ShownID: id}
	if !uarShownIDPattern.MatchString(id) {
		sum := sha256.Sum256([]byte(id))
		p.ShownID = "uar-" + hex.EncodeToString(sum[:16])
	}
	p.Reason, _ = m["reason"].(string)
	p.Message, _ = m["message"].(string)
	p.ToolCallID, _ = m["toolCallId"].(string)
	p.ExpiresAt, _ = m["expiresAt"].(string)
	switch p.Reason {
	case uarReasonToolApproval:
		p.NeedsUser = true
		p.Tool = uarLabel(tools[p.ToolCallID], "tool")
	case uarReasonElicitation:
		uar, _ := nested(m, "metadata", "uar").(map[string]any)
		server, _ := uar["serverName"].(string)
		p.Tool = uarLabel(server, "mcp")
		req, _ := uar["request"].(map[string]any)
		if msg, _ := req["message"].(string); msg != "" {
			p.Message = msg
		}
		switch {
		case uar["resolvable"] == false:
			p.Answer = &uarAnswer{Status: ResumeCancelled}
		case req["mode"] == "url":
			p.Answer = &uarAnswer{Status: ResumeResolved, Payload: map[string]any{"action": "decline"}}
		default:
			schema, _ := req["requestedSchema"].(map[string]any)
			if key, ok := confirmationKey(schema); ok {
				p.NeedsUser, p.ApproveKey = true, key
			} else {
				p.Answer = &uarAnswer{Status: ResumeCancelled}
			}
		}
	default:
		p.Answer = &uarAnswer{Status: ResumeCancelled}
	}
	return p, true
}

// nested reads m[k1][k2]... through objects, or nil.
func nested(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		obj, _ := cur.(map[string]any)
		if obj == nil {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

// confirmationKey reports whether the form asks for exactly one required
// boolean, which is what a yes/no confirmation is (kubo's own confirmation
// elicitation uses a "confirm" boolean), and returns its property name.
func confirmationKey(schema map[string]any) (string, bool) {
	if schema == nil || schema["type"] != "object" {
		return "", false
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) != 1 {
		return "", false
	}
	for name, v := range props {
		prop, _ := v.(map[string]any)
		if prop["type"] != "boolean" {
			return "", false
		}
		req, _ := schema["required"].([]any)
		if !slices.Contains(req, any(name)) {
			return "", false
		}
		return name, true
	}
	return "", false
}

var uarLabelDrop = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// uarLabel is the tool name the island shows for a UAR approval: "uar:" and the
// name UAR gave, reduced to letters, digits, dot, dash and underscore and cut
// to maxUARLabelLen, because the name is the peer's choice. The prefix keeps a
// UAR tool from looking like a tool of this node.
func uarLabel(name, fallback string) string {
	name = uarLabelDrop.ReplaceAllString(name, "")
	if len(name) > maxUARLabelLen {
		name = name[:maxUARLabelLen]
	}
	if name == "" {
		name = fallback
	}
	return "uar:" + name
}

// aguiInterrupt is the kubo-shaped interrupt (contract fixtures/agui.schema.json,
// Interrupt) for a UAR interrupt: reason tool_call, the standard
// {approved: boolean} response, and the ipfs-mcp metadata the island reads. The
// class is write, the more cautious of the two: UAR does not say what a tool
// does.
//
// The card is UAR's approval of UAR's tool call, not a confirmation of this
// node, and says so three ways: the message opens with uarApprovalLabel, the
// tool name carries the "uar:" prefix, and metadata.uar names the source. The
// island's schema requires ipfs-mcp.confirmation_id to match ^cf_...$, so the id
// keeps the cf_ prefix, but it is "cf_uar_" and a digest of UAR's interrupt id:
// no confirmation of the broker has that id, and the answer only goes to UAR,
// whose agent then calls this server's MCP endpoint, where the confirmation
// gate still decides every write.
func (p *uarPending) aguiInterrupt() map[string]any {
	msg := strings.TrimSpace(p.Message)
	if len(msg) < 5 {
		msg = "The assistant asks to run " + p.Tool + "."
	}
	sum := sha256.Sum256([]byte(p.ID))
	callID := p.ToolCallID
	if callID == "" {
		callID = p.ShownID
	}
	out := map[string]any{
		"id":         p.ShownID,
		"reason":     "tool_call",
		"message":    uarApprovalLabel + msg,
		"toolCallId": callID,
		"responseSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"approved": map[string]any{"type": "boolean"}},
			"required":   []string{"approved"},
		},
		"metadata": map[string]any{
			"ipfs-mcp": map[string]any{
				"confirmation_id":        "cf_uar_" + hex.EncodeToString(sum[:16]),
				"tool":                   p.Tool,
				"class":                  "write",
				"requires_native_prompt": false,
			},
			"uar": map[string]any{"reason": p.Reason, "source": "uar", "node_confirmation": false},
		},
	}
	if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
		out["expiresAt"] = t.UTC().Format(time.RFC3339)
	}
	return out
}
