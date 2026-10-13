//go:build !noagent

package server

import (
	"context"
	"errors"
	"time"
)

// aguiKeepalive is how often an idle {wait: true} stream gets an SSE comment.
const aguiKeepalive = 15 * time.Second

// resume answers the thread's one open interrupt (design.md, "Resume").
//
// Everything that resolves a confirmation passes the owner check first:
// Agent.OpenInterrupt returns the interrupt only to the principal that owns the
// thread, and the interrupt the client names must be that one. The confirmation
// ID comes from the server's own state, never from the request.
func (h *aguiHandler) resume(ctx context.Context, st *aguiStream, id Identity, in *aguiInput) {
	entry := in.Resume[0]
	open, err := h.srv.Agent().OpenInterrupt(id, in.ThreadID)
	if err != nil {
		h.reject(st.w, err)
		return
	}
	if entry.InterruptID != open.ID {
		h.reject(st.w, ErrUnknownInterrupt)
		return
	}
	switch {
	case entry.Status == ResumeCancelled, !entry.Wait && !entry.Approved:
		h.deny(ctx, st, id, in, open, entry)
	case entry.Wait:
		h.wait(ctx, st, id, in, open)
	default:
		h.approve(ctx, st, id, in, open)
	}
}

// approve records the chat's approval of a write and resumes the run. It is the
// one place a chat gesture approves anything: the principal is the chat (ui)
// principal, the interrupt does not need the native prompt, and the gate's
// broker refuses an approval from this resolver for a call that does.
func (h *aguiHandler) approve(ctx context.Context, st *aguiStream, id Identity, in *aguiInput, open *Interrupt) {
	if id.Kind != KindUI {
		h.reject(st.w, errAGUIInput("only the chat principal can approve in the chat"))
		return
	}
	if open.RequiresNativePrompt {
		h.reject(st.w, ErrNativePromptRequired)
		return
	}
	err := h.srv.ResolveConfirmation(open.ConfirmationID, ResolverAGUIInterrupt, true)
	switch {
	case err == nil,
		// Expiry is denial and an answered confirmation keeps its state: the
		// run reports what really happened.
		errors.Is(err, ErrConfirmationExpired),
		errors.Is(err, ErrConfirmationResolved),
		errors.Is(err, ErrUnknownConfirmation):
	default:
		h.reject(st.w, err)
		return
	}
	_, err = h.srv.Agent().Run(ctx, id, RunInput{SessionID: in.ThreadID, Resume: []ResumeEntry{
		{InterruptID: open.ID, Status: ResumeResolved, Approved: true},
	}}, st.handle)
	h.finish(st, err)
}

// deny resumes with a denial. When an observation of the same interrupt is
// open, that run owns the outcome: this request only records the denial and
// finishes empty, and the open stream reports confirmation_declined.
func (h *aguiHandler) deny(ctx context.Context, st *aguiStream, id Identity, in *aguiInput, open *Interrupt, entry aguiResume) {
	if h.watching(in.ThreadID, open.ID) {
		_ = h.srv.ResolveConfirmation(open.ConfirmationID, ResolverAGUIInterrupt, false) // already decided is fine
		st.begin()
		st.finishSuccess()
		return
	}
	_, err := h.srv.Agent().Run(ctx, id, RunInput{SessionID: in.ThreadID, Resume: []ResumeEntry{
		{InterruptID: open.ID, Status: entry.Status, Approved: false},
	}}, st.handle)
	h.finish(st, err)
}

// wait observes a confirmation that only the system prompt can decide. The
// stream starts at once; when the confirmation leaves pending the run resumes
// with Approved true, which executes the call only if a presence resolver
// approved it and otherwise reports confirmation_declined or
// confirmation_expired. Nothing is resolved here. If the client goes away
// first, nothing is called and the confirmation stays pending until it
// expires; a new {wait: true} picks it up.
func (h *aguiHandler) wait(ctx context.Context, st *aguiStream, id Identity, in *aguiInput, open *Interrupt) {
	if !open.RequiresNativePrompt {
		h.reject(st.w, errAGUIInput("{wait: true} is for an interrupt that needs the system prompt"))
		return
	}
	wctx, cancel := context.WithCancel(ctx)
	w := &aguiWatch{interruptID: open.ID, cancel: cancel}
	h.mu.Lock()
	if old := h.watches[in.ThreadID]; old != nil {
		old.cancel() // one watch per thread: the newest takes over
	}
	h.watches[in.ThreadID] = w
	h.mu.Unlock()
	defer func() {
		cancel()
		h.drop(in.ThreadID, w)
	}()

	st.begin()
	if err := h.awaitDecision(wctx, st, open.ConfirmationID); err != nil {
		return // the client left, or a newer watch took over
	}
	if !h.drop(in.ThreadID, w) {
		return
	}
	_, err := h.srv.Agent().Run(ctx, id, RunInput{SessionID: in.ThreadID, Resume: []ResumeEntry{
		{InterruptID: open.ID, Status: ResumeResolved, Approved: true},
	}}, st.handle)
	h.finish(st, err)
}

// awaitDecision blocks until the confirmation leaves pending. A confirmation
// the broker no longer knows is expired, which the run reports. It returns the
// context's error when ctx ends first.
func (h *aguiHandler) awaitDecision(ctx context.Context, st *aguiStream, confirmationID string) error {
	for {
		actx, cancel := context.WithTimeout(ctx, aguiKeepalive)
		_, err := h.srv.AwaitConfirmation(actx, confirmationID)
		cancel()
		switch {
		case err == nil, errors.Is(err, ErrUnknownConfirmation):
			return nil
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, context.DeadlineExceeded):
			st.keepalive()
		default:
			return err
		}
	}
}

// watching reports whether an observation of this interrupt is open.
func (h *aguiHandler) watching(thread, interruptID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	w := h.watches[thread]
	return w != nil && w.interruptID == interruptID
}

// drop unregisters the watch and reports whether it was still the registered
// one.
func (h *aguiHandler) drop(thread string, w *aguiWatch) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watches[thread] != w {
		return false
	}
	delete(h.watches, thread)
	return true
}
