//go:build !noagent

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// a2aConfig marks the A2A endpoint as enabled. It has no settings: the
// endpoint is on or off, and every grant comes from the principals.
type a2aConfig struct{}

// EnableA2A turns the A2A endpoint (POST /a2a and the agent card) on. It needs
// the agent loop: without a model backend it fails rather than mount an
// endpoint that could not answer a text message. Call it after EnableAgent and
// before NewHTTPServer; the routes are mounted by NewHTTPServer and are never
// served over stdio.
func (s *Server) EnableA2A() error {
	if s.Agent() == nil {
		return errors.New("the A2A endpoint needs an LLM backend (--llm-base-url and --llm-model)")
	}
	s.a2aExt = &a2aConfig{}
	return nil
}

func init() { RegisterRouteProvider(mountA2A) }

// mountA2A mounts /a2a and the agent card when EnableA2A was called. A server
// without it mounts nothing, so both paths are a plain 404.
func mountA2A(s *Server, m RouteMounter) error {
	if _, on := s.a2aExt.(*a2aConfig); !on || s.Agent() == nil {
		return nil
	}
	ctx, stop := context.WithCancel(context.Background())
	store := newA2AStore(time.Now)
	store.parent = ctx
	// The handler keeps no agent: each task resolves the current one, so a
	// backend swap needs no remount.
	h := &a2aHandler{srv: s, store: store, log: s.log, stopTasks: stop}
	s.onShutdown(h.shutdown)
	// The route's default principal kind is a2a: a token of any other kind is
	// refused before the handler runs.
	if err := m.Mount(RouteA2A, h, MountOptions{}); err != nil {
		return err
	}
	return m.Mount(RouteAgentCard, &cardHandler{srv: s, peers: a2aPeers(m)}, MountOptions{Public: true})
}

// a2aPeers returns the configured identities of kind a2a, which decide what the
// public card advertises.
func a2aPeers(m RouteMounter) []Identity {
	hs, ok := m.(*HTTPServer)
	if !ok {
		return nil
	}
	v, ok := hs.verify.(identitied)
	if !ok {
		return nil
	}
	var peers []Identity
	for _, id := range v.Identities() {
		if id.Kind == KindA2A {
			peers = append(peers, id)
		}
	}
	return peers
}

// a2aHandler serves POST /a2a, the A2A 1.0 JSON-RPC binding.
type a2aHandler struct {
	srv   *Server
	store *a2aStore
	log   *slog.Logger

	// stopTasks cancels the context every task derives from; mu guards
	// stopped, and work counts the drive goroutines still running.
	stopTasks context.CancelFunc
	mu        sync.Mutex
	stopped   bool
	work      sync.WaitGroup
}

// start runs fn on a goroutine that shutdown waits for. It reports false, and
// runs nothing, once the endpoint is stopping.
func (h *a2aHandler) start(fn func()) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return false
	}
	h.work.Add(1)
	go func() {
		defer h.work.Done()
		fn()
	}()
	return true
}

// shutdown cancels every task and waits for its goroutine, so none outlives the
// listener that served it.
func (h *a2aHandler) shutdown() {
	h.mu.Lock()
	h.stopped = true
	h.mu.Unlock()
	h.stopTasks()
	h.work.Wait()
}

// approvalNotice answers a message sent to a task that waits for approval. The
// message changes nothing: only a presence resolver on the node's machine
// decides.
const approvalNotice = "Approval cannot be given over A2A. Still waiting for approval on the node's machine."

// ServeHTTP implements http.Handler. The route's protection (Host, Origin,
// bearer, principal kind, body limit) already ran.
func (h *a2aHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RouteA2A {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST a JSON-RPC request", http.StatusMethodNotAllowed)
		return
	}
	var id Identity
	if info := auth.TokenInfoFromContext(r.Context()); info != nil {
		id, _ = info.Extra[identityExtraKey].(Identity)
	}
	if id.ID == "" || id.Kind != KindA2A {
		http.Error(w, "this endpoint serves A2A principals only", http.StatusForbidden)
		return
	}
	if h.srv.Agent() == nil {
		// A settings write moved the backend to a UAR agent (or the loop was
		// never installed): A2A runs the agent loop and has nothing to run.
		http.Error(w, "the A2A endpoint needs the OpenAI-compatible agent loop; the configured backend does not provide it", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		writeRPC(w, nil, nil, a2aErr(rpcParseError, "PARSE_ERROR", CodeInvalidArgument, false, "Invalid JSON payload"))
		return
	}
	req, rerr := parseRPC(body)
	if rerr != nil {
		writeRPC(w, nil, nil, rerr)
		return
	}
	if v := r.Header.Get("A2A-Version"); v != "" && v != "1" && !strings.HasPrefix(v, "1.") {
		writeRPC(w, req.ID, nil, a2aErr(a2aVersionNotSupported, "VERSION_NOT_SUPPORTED", CodeInvalidArgument, false,
			"A2A version "+v+" is not supported; this server speaks 1.0"))
		return
	}
	switch req.Method {
	case "SendMessage":
		h.send(w, r, id, req, false)
	case "SendStreamingMessage":
		h.send(w, r, id, req, true)
	case "GetTask":
		h.getTask(w, id, req)
	case "CancelTask":
		h.cancelTask(w, id, req)
	case "ListTasks", "SubscribeToTask":
		writeRPC(w, req.ID, nil, a2aErr(a2aUnsupportedOperation, "UNSUPPORTED_OPERATION", CodeToolUnavailable, false,
			req.Method+" is not supported by this server"))
	case "CreateTaskPushNotificationConfig", "GetTaskPushNotificationConfig", "ListTaskPushNotificationConfigs", "DeleteTaskPushNotificationConfig":
		writeRPC(w, req.ID, nil, a2aErr(a2aPushNotSupported, "PUSH_NOTIFICATION_NOT_SUPPORTED", CodeToolUnavailable, false,
			"Push notifications are not supported"))
	case "GetExtendedAgentCard":
		writeRPC(w, req.ID, nil, a2aErr(a2aExtendedCardMissing, "EXTENDED_AGENT_CARD_NOT_CONFIGURED", CodeToolUnavailable, false,
			"This agent has no extended agent card"))
	default:
		writeRPC(w, req.ID, nil, a2aErr(rpcMethodNotFound, "METHOD_NOT_FOUND", CodeInvalidArgument, false, "Method not found"))
	}
}

// parseRPC decodes one JSON-RPC request. Batches are not supported.
func parseRPC(body []byte) (*rpcRequest, *rpcError) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return nil, a2aErr(rpcInvalidRequest, "INVALID_REQUEST", CodeInvalidArgument, false, "Batch requests are not supported")
	}
	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		return nil, a2aErr(rpcParseError, "PARSE_ERROR", CodeInvalidArgument, false, "Invalid JSON payload")
	}
	if req.JSONRPC != "2.0" || req.Method == "" || len(req.ID) == 0 {
		return nil, a2aErr(rpcInvalidRequest, "INVALID_REQUEST", CodeInvalidArgument, false, "Request payload validation error")
	}
	return &req, nil
}

// writeRPC answers a request with a result or an error.
func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, rerr *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: rerr})
}

// sendParams are the parameters of SendMessage and SendStreamingMessage.
type sendParams struct {
	Message       *a2aMessage `json:"message"`
	Configuration *struct {
		ReturnImmediately bool `json:"returnImmediately"`
	} `json:"configuration"`
}

// send handles SendMessage and SendStreamingMessage.
func (h *a2aHandler) send(w http.ResponseWriter, r *http.Request, id Identity, req *rpcRequest, stream bool) {
	var p sendParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Message == nil {
		writeRPC(w, req.ID, nil, errInvalidParams("params must hold a message"))
		return
	}
	t, notice, rerr := h.begin(r.Context(), id, p.Message)
	if rerr != nil {
		writeRPC(w, req.ID, nil, rerr)
		return
	}
	if stream {
		h.stream(w, r, req.ID, t, notice)
		return
	}
	if notice == "" && !(p.Configuration != nil && p.Configuration.ReturnImmediately) {
		if err := t.waitSettled(r.Context()); err != nil {
			return // the caller went away; the task goes on
		}
	}
	writeRPC(w, req.ID, a2aSendResponse{Task: t.view(notice)}, nil)
}

// a2aWork is what a message asks for.
type a2aWork struct {
	// call is true for a data part naming a contract tool.
	call bool
	text string
	tool string
	args json.RawMessage
	// continueOnly is set for a text message to a context that already ran a
	// text task: it must continue that conversation or fail, never start a
	// new loop session under the same contextId (A2A 1.0.0 section 3.4).
	continueOnly bool
}

// classify reads the parts of a message: one {tool, arguments} data part calls
// a contract tool directly, text parts go to the agent loop.
func classify(parts []a2aPart) (a2aWork, *rpcError) {
	var texts []string
	var data []a2aPart
	for _, p := range parts {
		switch {
		case p.Raw != nil || p.URL != nil:
			return a2aWork{}, a2aErr(a2aContentTypeNotSupport, "CONTENT_TYPE_NOT_SUPPORTED", CodeInvalidArgument, false,
				"Only text and JSON data parts are supported")
		case p.Text != nil:
			texts = append(texts, *p.Text)
		case len(p.Data) > 0:
			data = append(data, p)
		default:
			return a2aWork{}, errInvalidParams("a part needs text or data")
		}
	}
	switch {
	case len(data) > 1 || (len(data) == 1 && len(texts) > 0):
		return a2aWork{}, errInvalidParams("send text parts, or one data part naming a tool, not both")
	case len(data) == 1:
		return classifyCall(data[0])
	}
	text := strings.Join(texts, "\n")
	if strings.TrimSpace(text) == "" {
		return a2aWork{}, errInvalidParams("message has no text")
	}
	return a2aWork{text: text}, nil
}

func classifyCall(p a2aPart) (a2aWork, *rpcError) {
	if p.MediaType != "" && p.MediaType != "application/json" {
		return a2aWork{}, a2aErr(a2aContentTypeNotSupport, "CONTENT_TYPE_NOT_SUPPORTED", CodeInvalidArgument, false,
			"A data part must be application/json")
	}
	var call struct {
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	dec := json.NewDecoder(bytes.NewReader(p.Data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&call); err != nil || call.Tool == "" {
		return a2aWork{}, errInvalidParams(`a data part must be {"tool": name, "arguments": object}`)
	}
	args, err := stripConfirmToken(call.Arguments)
	if err != nil {
		return a2aWork{}, errInvalidParams("arguments must be a JSON object")
	}
	return a2aWork{call: true, tool: call.Tool, args: args}, nil
}

// begin validates a message and starts, or answers, the task it belongs to. For
// a message to a task that waits for approval it starts nothing and returns the
// notice to put in the answer.
func (h *a2aHandler) begin(ctx context.Context, id Identity, m *a2aMessage) (*a2aTask, string, *rpcError) {
	switch {
	case m.MessageID == "":
		return nil, "", errInvalidParams("message.messageId is required")
	case m.Role != a2aRoleUser:
		return nil, "", errInvalidParams("message.role must be ROLE_USER")
	case len(m.Parts) == 0 || len(m.Parts) > 16:
		return nil, "", errInvalidParams("message.parts needs between 1 and 16 parts")
	}
	work, rerr := classify(m.Parts)
	if rerr != nil {
		return nil, "", rerr
	}
	if m.TaskID != "" {
		return h.continueTask(id, m)
	}
	work.continueOnly = !work.call && m.ContextID != "" && h.store.ranText(id.ID, m.ContextID)
	t, fresh, err := h.store.newTask(id.ID, m.ContextID)
	if err != nil {
		return nil, "", rpcErrorOf(err)
	}
	if !h.start(func() { h.drive(t, id, work) }) {
		h.store.drop(t, fresh)
		return nil, "", rpcErrorOf(errServerStopping)
	}
	select {
	case <-t.started:
	case <-ctx.Done():
		return nil, "", rpcErrorOf(ctx.Err())
	}
	if t.preErr != nil {
		var te *ToolError
		if errors.As(t.preErr, &te) && te.Code == CodePermissionDenied {
			t.setStatus(a2aAuthRequired, te.Message, nil, nil)
			return t, "", nil
		}
		h.store.drop(t, fresh)
		if work.continueOnly && errors.Is(t.preErr, ErrSessionNotFound) {
			// The conversation behind the context is gone: the context goes
			// with it, and the peer starts a new one by omitting contextId.
			h.store.dropContext(id.ID, t.contextID)
		}
		return nil, "", rpcErrorOf(t.preErr)
	}
	if !work.call {
		h.store.markRanText(id.ID, t.contextID)
	}
	return t, "", nil
}

// continueTask handles a message that names a task. Only a task that waits for
// approval accepts one, and the message does nothing to it.
func (h *a2aHandler) continueTask(id Identity, m *a2aMessage) (*a2aTask, string, *rpcError) {
	t := h.store.task(id.ID, m.TaskID)
	if t == nil {
		return nil, "", errTaskNotFound()
	}
	if m.ContextID != "" && m.ContextID != t.contextID {
		return nil, "", errInvalidParams("contextId does not match the task")
	}
	v := t.view("")
	switch {
	case v.Status.State == a2aInputRequired:
		return t, approvalNotice, nil
	case a2aTerminal(v.Status.State):
		return nil, "", a2aErr(a2aUnsupportedOperation, "TASK_FINAL", CodeFailedPrecondition, false,
			"The task has ended and cannot take messages; send a new message without taskId")
	}
	return nil, "", a2aErr(a2aUnsupportedOperation, "TASK_NOT_WAITING", CodeFailedPrecondition, false,
		"The task is not waiting for input")
}

// getTask handles GetTask.
func (h *a2aHandler) getTask(w http.ResponseWriter, id Identity, req *rpcRequest) {
	taskID, rerr := taskIDParam(req)
	if rerr != nil {
		writeRPC(w, req.ID, nil, rerr)
		return
	}
	t := h.store.task(id.ID, taskID)
	if t == nil {
		writeRPC(w, req.ID, nil, errTaskNotFound())
		return
	}
	writeRPC(w, req.ID, t.view(""), nil)
}

// cancelTask handles CancelTask. For a task that waits for approval it records
// a denial, which is always safe; nothing here ever approves.
func (h *a2aHandler) cancelTask(w http.ResponseWriter, id Identity, req *rpcRequest) {
	taskID, rerr := taskIDParam(req)
	if rerr != nil {
		writeRPC(w, req.ID, nil, rerr)
		return
	}
	t := h.store.task(id.ID, taskID)
	if t == nil {
		writeRPC(w, req.ID, nil, errTaskNotFound())
		return
	}
	pending := t.pendingCopy()
	// The state changes first, so a decision that lands now finds the task
	// cancelled and does not run the call.
	if !t.setStatus(a2aCanceled, "Cancelled by the caller.", nil, nil) {
		writeRPC(w, req.ID, nil, a2aErr(a2aTaskNotCancelable, "TASK_NOT_CANCELABLE", CodeFailedPrecondition, false,
			"The task has already ended"))
		return
	}
	t.cancel()
	if pending != nil {
		// A confirmation that is already decided keeps its state.
		_ = h.srv.ResolveConfirmation(pending.confirmationID, "", false)
	}
	writeRPC(w, req.ID, t.view(""), nil)
}

func taskIDParam(req *rpcRequest) (string, *rpcError) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil || p.ID == "" {
		return "", errInvalidParams("params.id is required")
	}
	return p.ID, nil
}

// stream answers SendStreamingMessage: the task, then its updates until it
// ends. A task that waits for approval keeps the stream open, so the peer sees
// the outcome of the out-of-band decision.
func (h *a2aHandler) stream(w http.ResponseWriter, r *http.Request, rpcID json.RawMessage, t *a2aTask, notice string) {
	rc := http.NewResponseController(w)
	hd := w.Header()
	hd.Set("Content-Type", "text/event-stream")
	hd.Set("Cache-Control", "no-cache")
	hd.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(sr a2aStreamResponse) bool {
		b, err := json.Marshal(rpcResponse{JSONRPC: "2.0", ID: rpcID, Result: sr})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	view, idx := t.initialView(), 0
	if notice != "" {
		view, idx = t.snapshot(notice)
	}
	if !send(a2aStreamResponse{Task: view}) {
		return
	}
	for {
		evs, next, wake, ended := t.eventsFrom(idx)
		idx = next
		for _, ev := range evs {
			if !send(a2aStreamResponse{StatusUpdate: ev.status, ArtifactUpdate: ev.artifact}) {
				return
			}
		}
		if ended {
			return
		}
		keepalive := time.NewTimer(aguiKeepalive)
		select {
		case <-r.Context().Done():
			keepalive.Stop()
			return
		case <-wake:
			keepalive.Stop()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		}
	}
}
