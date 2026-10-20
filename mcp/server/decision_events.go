//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

const (
	decisionChangedEvent       = "ipfs.decision.changed"
	decisionSnapshotEvent      = "ipfs.decisions.snapshot_required"
	decisionStreamKeepalive    = 25 * time.Second
	decisionStreamWriteLimit   = 5 * time.Second
	decisionStreamLifetime     = 15 * time.Minute
	decisionStreamRetrySeconds = 5
)

func encodeDecisionNotice(notice decisionNotice) ([]byte, error) {
	name := ""
	var value any
	switch {
	case notice.change != nil && notice.snapshot == nil:
		name, value = decisionChangedEvent, notice.change
	case notice.snapshot != nil && notice.change == nil:
		name, value = decisionSnapshotEvent, notice.snapshot
	default:
		return nil, errors.New("invalid decision notice")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode decision notice: %w", err)
	}
	frame := []byte("event: " + name + "\ndata: " + string(data) + "\n\n")
	if len(frame) > maxDecisionEventBytes {
		return nil, errors.New("decision notice exceeds event bound")
	}
	return frame, nil
}

func decisionStreamEnd(now time.Time, info *auth.TokenInfo, id Identity) time.Time {
	if info == nil || info.Expiration.IsZero() {
		return time.Time{}
	}
	end := now.Add(decisionStreamLifetime)
	if info.Expiration.Before(end) {
		end = info.Expiration
	}
	if id.Session != nil && id.Session.Expiry.Before(end) {
		end = id.Session.Expiry
	}
	return end
}

func (h *decisionReadsHandler) events(w http.ResponseWriter, r *http.Request, id Identity, info *auth.TokenInfo) {
	end := decisionStreamEnd(time.Now(), info, id)
	if !time.Now().Before(end) {
		http.Error(w, "Unauthorized: authentication must still be valid", http.StatusUnauthorized)
		return
	}
	rc := http.NewResponseController(w)
	// Probe before allocating a queue or starting SSE. An unsupported writer
	// cannot satisfy the bounded slow-consumer contract, so fail closed.
	if err := rc.SetWriteDeadline(minDeadline(time.Now().Add(decisionStreamWriteLimit), end)); err != nil {
		decisionReadError(w, 500, "internal", "Decision streaming requires bounded writes.", nil)
		return
	}
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		decisionReadError(w, 500, "internal", "Decision streaming requires bounded writes.", nil)
		return
	}
	// Hydrate existing safe UAR source markers without any backend I/O, then
	// atomically subscribe before capturing the initial owner revision.
	h.srv.decisionSnapshotFor(id.BindingKey())
	sub, err := h.srv.gate.broker.subscribeDecisions(id.Principal)
	if err != nil {
		if errors.Is(err, errDecisionSubscriptionsFull) {
			w.Header().Set("Retry-After", strconv.Itoa(decisionStreamRetrySeconds))
			decisionReadError(w, 429, "resource_exhausted", "Too many decision streams are open.", nil)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	defer sub.close()
	ctx, cancel := context.WithDeadline(r.Context(), end)
	defer cancel()
	alive := func() bool {
		if ctx.Err() != nil || h.runtime.ctx.Err() != nil || !time.Now().Before(end) {
			return false
		}
		select {
		case <-sub.done:
			return false
		default:
			return true
		}
	}
	if !alive() {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// At most one cancellation watcher per already-quota-bound stream. It
	// interrupts blocked writes on known invalidation, shutdown or deadline;
	// it never runs under broker.mu and always joins before the handler exits.
	stopWatch, watched := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watched)
		select {
		case <-stopWatch:
			return
		case <-ctx.Done():
		case <-h.runtime.ctx.Done():
		case <-sub.done:
		}
		cancel()
		_ = rc.SetWriteDeadline(time.Now())
	}()
	defer func() {
		close(stopWatch)
		<-watched
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(frame []byte) bool {
		if !alive() {
			return false
		}
		if err := rc.SetWriteDeadline(minDeadline(time.Now().Add(decisionStreamWriteLimit), end)); err != nil {
			return false
		}
		if !alive() {
			return false
		}
		if n, err := w.Write(frame); err != nil || n != len(frame) || !alive() {
			return false
		}
		if err := rc.Flush(); err != nil || !alive() {
			return false
		}
		// An expired write deadline cannot be extended. Clear a successful
		// short frame's deadline before entering the longer idle interval.
		return rc.SetWriteDeadline(time.Time{}) == nil
	}
	send := func(notice decisionNotice) bool {
		frame, err := encodeDecisionNotice(notice)
		return err == nil && write(frame)
	}
	if !send(decisionNotice{snapshot: &sub.initial}) {
		return
	}
	keepalive := time.NewTicker(decisionStreamKeepalive)
	defer keepalive.Stop()
	// Last-Event-ID is intentionally ignored: every connection gets the new
	// initial marker, never an SSE id or durable replay promise.
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.runtime.ctx.Done():
			return
		case <-sub.done:
			return
		case notice, open := <-sub.events:
			if !open || !send(notice) {
				return
			}
		case <-keepalive.C:
			if !write([]byte(": keepalive\n\n")) {
				return
			}
		}
	}
}

func minDeadline(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
