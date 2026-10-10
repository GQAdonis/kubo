package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClock is a settable clock for the gate.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeResolver records the requests it gets and lets the test decide later.
type fakeResolver struct {
	name string
	err  error

	mu       sync.Mutex
	requests []Pending
	resolve  map[string]ResolveFunc
}

func newFakeResolver(name string) *fakeResolver {
	return &fakeResolver{name: name, resolve: map[string]ResolveFunc{}}
}

func (r *fakeResolver) Name() string { return r.name }

func (r *fakeResolver) Request(_ context.Context, p Pending, resolve ResolveFunc) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, p)
	r.resolve[p.ID] = resolve
	return nil
}

func (r *fakeResolver) last(t testing.TB) (Pending, ResolveFunc) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.NotEmpty(t, r.requests, "the resolver was never asked")
	p := r.requests[len(r.requests)-1]
	return p, r.resolve[p.ID]
}

// gateEnv is a server with the pin tools, destructive tools enabled, a fake
// clock and a fake out-of-band resolver.
type gateEnv struct {
	s        *Server
	h        *harness
	d        *fakeDaemon
	pins     *pinStore
	clock    *fakeClock
	resolver *fakeResolver
	target   string
}

func newGateEnv(t *testing.T, resolvers ...Resolver) *gateEnv {
	t.Helper()
	d := newFakeDaemon(t)
	target := testCID(t, 1)
	pins := installPins(d, map[string]fakePin{target: {Type: "recursive"}, testCID(t, 2): {Type: "recursive"}})
	clock := newFakeClock()
	var res *fakeResolver
	if len(resolvers) == 0 {
		res = newFakeResolver(ResolverDesktop)
		resolvers = []Resolver{res}
	}
	s := mustNew(t, d, Options{Tools: []string{"pin", "destructive"}, Resolvers: resolvers, now: clock.Now})
	return &gateEnv{s: s, h: newHarness(s), d: d, pins: pins, clock: clock, resolver: res, target: target}
}

func (e *gateEnv) rm(t *testing.T, cs *mcp.ClientSession, extra map[string]any) *mcp.CallToolResult {
	t.Helper()
	args := map[string]any{"path": "/ipfs/" + e.target}
	for k, v := range extra {
		args[k] = v
	}
	return toolCall(t, cs, "pin_rm", args)
}

// confirmationOf decodes the tool error of a result that must be an error.
func confirmationOf(t *testing.T, res *mcp.CallToolResult, code string) ToolError {
	t.Helper()
	require.True(t, res.IsError, "want %s, got success: %+v", code, res.StructuredContent)
	te := decodeToolError(t, res)
	require.Equal(t, code, te.Code, "message: %s", te.Message)
	validateAgainstCommon(t, "ToolError", res.StructuredContent)
	return te
}

func validateConfirmationDef(t *testing.T, def string, v any) {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "confirmation/confirmation.schema.json")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	wrapper := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$ref":    "#/$defs/" + def,
		"$defs":   doc["$defs"],
	}
	validateJSON(t, asJSON(t, wrapper), v)
}

func TestNonElicitingClientWithoutResolverIsRefused(t *testing.T) {
	d := newFakeDaemon(t)
	store := installPins(d, map[string]fakePin{testCID(t, 1): {Type: "recursive"}})
	s := mustNew(t, d, Options{Tools: []string{"pin", "destructive"}})
	cs := connect(t, s)

	res := toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + testCID(t, 1)})
	te := confirmationOf(t, res, CodeConfirmUnavailable)
	assert.Nil(t, te.Confirmation, "no token and no pending confirmation without a resolver")
	assert.True(t, store.has(testCID(t, 1)), "the pin remains")
	assert.Zero(t, d.count("pin/rm"))

	// Writes are refused the same way.
	res = toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 5)})
	confirmationOf(t, res, CodeConfirmUnavailable)
	assert.Zero(t, d.count("pin/add"))
}

func TestElicitationAcceptRunsOnce(t *testing.T) {
	e := newGateEnv(t)
	var asked []*mcp.ElicitParams
	cs := e.h.connect(t, elicitOptions(func(r *mcp.ElicitRequest) *mcp.ElicitResult {
		asked = append(asked, r.Params)
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}
	}), nil)

	res := e.rm(t, cs, nil)
	require.False(t, res.IsError, "%+v", res.Content)
	var out pinRmOutput
	require.NoError(t, json.Unmarshal(asJSON(t, res.StructuredContent), &out))
	assert.Equal(t, []string{e.target}, out.Unpinned)
	validateOutput(t, "pin_rm", out)
	assert.False(t, e.pins.has(e.target))
	assert.Equal(t, 1, e.d.count("pin/rm"))

	require.Len(t, asked, 1)
	assert.Equal(t, "form", asked[0].Mode)
	assert.Contains(t, asked[0].Message, e.target, "the prompt names the CID")
	assert.Contains(t, asked[0].Message, "Remove the pin")
	assert.Empty(t, e.s.PendingConfirmations())
}

func TestElicitationDeclineCancelAndFalseLeaveThePin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *mcp.ElicitResult
	}{
		{"decline", &mcp.ElicitResult{Action: "decline"}},
		{"cancel", &mcp.ElicitResult{Action: "cancel"}},
		{"accept without consent", &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": false}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, elicitOptions(func(*mcp.ElicitRequest) *mcp.ElicitResult { return tc.result }), nil)
			res := e.rm(t, cs, nil)
			te := confirmationOf(t, res, CodeConfirmDeclined)
			assert.Contains(t, te.Message, "Denied.")
			assert.Equal(t, "declined", te.Details["status"])
			validateConfirmationDef(t, "Outcome", te.Details)
			assert.True(t, e.pins.has(e.target))
			assert.Zero(t, e.d.count("pin/rm"))

			// Deny is final: the next call asks again.
			res = e.rm(t, cs, nil)
			confirmationOf(t, res, CodeConfirmDeclined)
		})
	}
}

func TestElicitationFailureFailsClosed(t *testing.T) {
	e := newGateEnv(t)
	co := &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return nil, errors.New("the dialog crashed")
	}}
	cs := e.h.connect(t, co, nil)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_rm", Arguments: map[string]any{"path": "/ipfs/" + e.target}})
	if err == nil {
		assert.True(t, res.IsError, "a failed prompt never counts as consent")
	}
	assert.True(t, e.pins.has(e.target))
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestWritesAreConfirmedToo(t *testing.T) {
	e := newGateEnv(t)
	declines := 0
	cs := e.h.connect(t, elicitOptions(func(r *mcp.ElicitRequest) *mcp.ElicitResult {
		declines++
		assert.Contains(t, r.Params.Message, "/ipfs/")
		return &mcp.ElicitResult{Action: "decline"}
	}), nil)
	confirmationOf(t, toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + testCID(t, 9)}), CodeConfirmDeclined)
	confirmationOf(t, toolCall(t, cs, "pin_update", map[string]any{"from": "/ipfs/" + e.target, "to": "/ipfs/" + testCID(t, 9)}), CodeConfirmDeclined)
	assert.Equal(t, 2, declines)
	assert.Zero(t, e.d.count("pin/add")+e.d.count("pin/update"))
}

func TestTokenFlowRequiresApproval(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)

	// First call: nothing happens, a token and a pending confirmation come back.
	first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	require.NotNil(t, first.Confirmation)
	conf := first.Confirmation
	validateConfirmationDef(t, "ConfirmationRequired", conf)
	assert.Equal(t, StatePending, conf.State)
	assert.Equal(t, []string{ResolverDesktop}, conf.Resolvers)
	assert.Equal(t, KindHumanMCP, conf.PrincipalKind)
	assert.NotEmpty(t, conf.ConfirmToken)
	assert.Equal(t, "pin_rm", conf.Summary.Tool)
	assert.Equal(t, contract.ClassDestructive, conf.Summary.Class)
	assert.Equal(t, []Target{{Kind: "cid", Value: e.target}}, conf.Summary.Targets, "summary names the CID")
	assert.Contains(t, conf.Summary.Text, e.target)
	assert.Contains(t, conf.Summary.Text, "recursive=true", "defaults are rendered")
	assert.True(t, e.pins.has(e.target))
	assert.Zero(t, e.d.count("pin/rm"))
	req, resolve := e.resolver.last(t)
	assert.Equal(t, conf.ConfirmationID, req.ID, "the resolver was asked about this confirmation")

	// The agent presents its own token while the confirmation is pending: no.
	pending := confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmPending)
	assert.True(t, pending.Retryable)
	assert.True(t, e.pins.has(e.target), "a token alone never confirms")
	assert.Zero(t, e.d.count("pin/rm"))

	// A repeat without a token does not approve either.
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmPending)
	assert.Zero(t, e.d.count("pin/rm"))

	// A person approves out of band.
	require.NoError(t, resolve(true))
	res := e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken})
	require.False(t, res.IsError, "%+v", res.Content)
	assert.False(t, e.pins.has(e.target))
	assert.Equal(t, 1, e.d.count("pin/rm"))

	// Replay of the used token is rejected and runs nothing.
	e.pins.mu.Lock()
	e.pins.pins[e.target] = fakePin{Type: "recursive"}
	e.pins.mu.Unlock()
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": conf.ConfirmToken}), CodeConfirmInvalid)
	assert.True(t, e.pins.has(e.target))
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestTokenRejections(t *testing.T) {
	approvedToken := func(t *testing.T, e *gateEnv, cs *mcp.ClientSession) string {
		t.Helper()
		first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
		_, resolve := e.resolver.last(t)
		require.NoError(t, resolve(true))
		return first.Confirmation.ConfirmToken
	}
	cases := []struct {
		name string
		// attack runs with an approved confirmation and its token.
		attack func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult
		code   string
	}{
		{
			name: "tampered signature",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				return e.rm(t, cs, map[string]any{"confirm_token": flipLast(tok)})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "tampered claims",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				return e.rm(t, cs, map[string]any{"confirm_token": flipFirst(tok)})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "garbage",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, _ string) *mcp.CallToolResult {
				return e.rm(t, cs, map[string]any{"confirm_token": "0123456789012345678901234567890123456789"})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "changed arguments",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				other := testCID(t, 2)
				return toolCall(t, cs, "pin_rm", map[string]any{"path": "/ipfs/" + other, "confirm_token": tok})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "different optional argument",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				return e.rm(t, cs, map[string]any{"recursive": false, "confirm_token": tok})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "another tool",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				return toolCall(t, cs, "pin_add", map[string]any{"path": "/ipfs/" + e.target, "confirm_token": tok})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "another session",
			attack: func(t *testing.T, e *gateEnv, _ *mcp.ClientSession, tok string) *mcp.CallToolResult {
				other := e.h.connect(t, nil, nil)
				return e.rm(t, other, map[string]any{"confirm_token": tok})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "another principal",
			attack: func(t *testing.T, e *gateEnv, _ *mcp.ClientSession, tok string) *mcp.CallToolResult {
				other := e.h.connect(t, nil, &Principal{ID: "someone-else", Kind: KindHumanMCP})
				return e.rm(t, other, map[string]any{"confirm_token": tok})
			},
			code: CodeConfirmInvalid,
		},
		{
			name: "expired",
			attack: func(t *testing.T, e *gateEnv, cs *mcp.ClientSession, tok string) *mcp.CallToolResult {
				e.clock.Advance(confirmTTL + time.Second)
				return e.rm(t, cs, map[string]any{"confirm_token": tok})
			},
			code: CodeConfirmExpired,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, nil, nil)
			tok := approvedToken(t, e, cs)
			confirmationOf(t, tc.attack(t, e, cs, tok), tc.code)
			assert.True(t, e.pins.has(e.target), "a rejected token leaves the pin")
			assert.Zero(t, e.d.count("pin/rm"))
			if tc.name != "expired" {
				// The approved confirmation is still usable by its owner.
				require.False(t, e.rm(t, cs, map[string]any{"confirm_token": tok}).IsError)
				assert.Equal(t, 1, e.d.count("pin/rm"))
			}
		})
	}
}

func flipLast(s string) string {
	b := []byte(s)
	if b[len(b)-1] == 'A' {
		b[len(b)-1] = 'B'
	} else {
		b[len(b)-1] = 'A'
	}
	return string(b)
}

func flipFirst(s string) string {
	b := []byte(s)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

func TestTokenStatesFollowTheContractTable(t *testing.T) {
	// TokenAcceptance: confirmation state -> accepted, error code.
	cases := []struct {
		state    string
		prepare  func(t *testing.T, e *gateEnv, resolve ResolveFunc)
		accepted bool
		code     string
		errCode  string
	}{
		{StatePending, func(*testing.T, *gateEnv, ResolveFunc) {}, false, "confirmation_pending", CodeConfirmPending},
		{StateApproved, func(t *testing.T, _ *gateEnv, r ResolveFunc) { require.NoError(t, r(true)) }, true, "none", ""},
		{StateDenied, func(t *testing.T, _ *gateEnv, r ResolveFunc) { require.NoError(t, r(false)) }, false, "confirmation_declined", CodeConfirmDeclined},
		{StateExpired, func(t *testing.T, e *gateEnv, _ ResolveFunc) { e.clock.Advance(confirmTTL + time.Second) }, false, "confirmation_expired", CodeConfirmExpired},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			e := newGateEnv(t)
			cs := e.h.connect(t, nil, nil)
			first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
			_, resolve := e.resolver.last(t)
			tc.prepare(t, e, resolve)
			res := e.rm(t, cs, map[string]any{"confirm_token": first.Confirmation.ConfirmToken})
			validateConfirmationDef(t, "TokenAcceptance", map[string]any{
				"confirmation_state": tc.state, "accepted": !res.IsError, "error_code": codeOrNone(t, res),
			})
			assert.Equal(t, tc.accepted, !res.IsError)
			if !tc.accepted {
				assert.Equal(t, tc.errCode, decodeToolError(t, res).Code)
				assert.Zero(t, e.d.count("pin/rm"))
			}
		})
	}
	t.Run("executed", func(t *testing.T) {
		e := newGateEnv(t)
		cs := e.h.connect(t, nil, nil)
		first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
		_, resolve := e.resolver.last(t)
		require.NoError(t, resolve(true))
		require.False(t, e.rm(t, cs, map[string]any{"confirm_token": first.Confirmation.ConfirmToken}).IsError)
		res := e.rm(t, cs, map[string]any{"confirm_token": first.Confirmation.ConfirmToken})
		confirmationOf(t, res, CodeConfirmInvalid)
		validateConfirmationDef(t, "TokenAcceptance", map[string]any{
			"confirmation_state": StateExecuted, "accepted": false, "error_code": "confirmation_invalid",
		})
		assert.Equal(t, 1, e.d.count("pin/rm"))
	})
}

func codeOrNone(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		return "none"
	}
	return decodeToolError(t, res).Code
}

func TestExternalApprovalRunsExactlyOnce(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	assert.ErrorIs(t, resolve(true), ErrConfirmationResolved, "a decision is final")
	assert.ErrorIs(t, resolve(false), ErrConfirmationResolved)

	// Many identical repeats race: exactly one runs.
	var wg sync.WaitGroup
	var mu sync.Mutex
	ran := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "pin_rm", Arguments: map[string]any{"path": "/ipfs/" + e.target}})
			if err == nil && !res.IsError {
				mu.Lock()
				ran++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, ran)
	assert.Equal(t, 1, e.d.count("pin/rm"))
}

func TestDeniedConfirmationIsFinal(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(false))

	denied := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmDeclined)
	assert.Contains(t, denied.Message, "Denied.")
	assert.Equal(t, "denied", denied.Details["status"])
	validateConfirmationDef(t, "Outcome", denied.Details)
	assert.Zero(t, e.d.count("pin/rm"))

	// The agent must start a new call, which needs a new confirmation.
	again := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	assert.NotEqual(t, denied.Details["confirmation_id"], again.Confirmation.ConfirmationID)
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestExpiredConfirmationIsDenial(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	e.clock.Advance(confirmTTL + time.Second)

	assert.ErrorIs(t, resolve(true), ErrConfirmationExpired, "approval after expiry is refused")
	expired := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmExpired)
	validateConfirmationDef(t, "Outcome", expired.Details)
	assert.Zero(t, e.d.count("pin/rm"))
	confirmationOf(t, e.rm(t, cs, map[string]any{"confirm_token": first.Confirmation.ConfirmToken}), CodeConfirmExpired)
	assert.Empty(t, e.s.PendingConfirmations())
}

func TestApprovedButExpiredDoesNotRun(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	_, resolve := e.resolver.last(t)
	require.NoError(t, resolve(true))
	e.clock.Advance(confirmTTL + time.Second)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmExpired)
	assert.Zero(t, e.d.count("pin/rm"))
}

func TestResolverFailureFailsClosed(t *testing.T) {
	r := newFakeResolver(ResolverDesktop)
	r.err = errors.New("no prompt available")
	e := newGateEnv(t, r)
	cs := e.h.connect(t, nil, nil)
	confirmationOf(t, e.rm(t, cs, nil), CodeConfirmUnavailable)
	assert.Zero(t, e.d.count("pin/rm"))
	assert.Empty(t, e.s.PendingConfirmations())
}

func TestUnknownResolverNameIsRejectedAtStartup(t *testing.T) {
	_, err := New(t.Context(), newFakeDaemon(t), Options{Resolvers: []Resolver{newFakeResolver("telepathy")}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telepathy")
	// The token is not a resolver either.
	_, err = New(t.Context(), newFakeDaemon(t), Options{Resolvers: []Resolver{newFakeResolver(ResolverElicitation)}})
	require.Error(t, err, "elicitation is built in, not a pluggable resolver")
}

func TestResolveConfirmationOnServer(t *testing.T) {
	e := newGateEnv(t)
	cs := e.h.connect(t, nil, nil)
	first := confirmationOf(t, e.rm(t, cs, nil), CodeConfirmRequired)
	id := first.Confirmation.ConfirmationID

	pending := e.s.PendingConfirmations()
	require.Len(t, pending, 1)
	assert.Equal(t, id, pending[0].ID)
	assert.ErrorIs(t, e.s.ResolveConfirmation("cf_unknown0000000000", ResolverDesktop, true), ErrUnknownConfirmation)
	require.NoError(t, e.s.ResolveConfirmation(id, ResolverDesktop, true))
	assert.Empty(t, e.s.PendingConfirmations())
	require.False(t, e.rm(t, cs, nil).IsError)
}
