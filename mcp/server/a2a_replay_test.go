//go:build !noagent

package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"testing"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type a2aFixture struct {
	Card      map[string]any `json:"card"`
	Exchanges []struct {
		Request  map[string]any   `json:"request"`
		Response map[string]any   `json:"response"`
		Stream   []map[string]any `json:"stream"`
	} `json:"exchanges"`
}

func loadA2AFixture(t *testing.T, name string) a2aFixture {
	t.Helper()
	raw, err := fs.ReadFile(contract.FS(), "fixtures/a2a/"+name)
	require.NoError(t, err)
	var f a2aFixture
	require.NoError(t, json.Unmarshal(raw, &f))
	return f
}

// fixtureTask returns the Task of a fixture response or stream event.
func fixtureTask(r map[string]any) map[string]any {
	res, _ := r["result"].(map[string]any)
	if task, ok := res["task"].(map[string]any); ok {
		return task
	}
	if _, ok := res["status"]; ok {
		return res
	}
	return nil
}

// replayIDs rewrites the fixture's task and context IDs into the ones the
// server issued.
func replayIDs(v any, ids map[string]string) any {
	b, _ := json.Marshal(v)
	s := string(b)
	for from, to := range ids {
		s = strings.ReplaceAll(s, `"`+from+`"`, `"`+to+`"`)
	}
	var out any
	_ = json.Unmarshal([]byte(s), &out)
	return out
}

// TestA2AFixturesReplay sends the requests of the contract fixtures to the real
// handler and checks the states, the artifact shape and the schema.
func TestA2AFixturesReplay(t *testing.T) {
	t.Run("send-message-read", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		f := loadA2AFixture(t, "send-message-read.json")
		ex := f.Exchanges[0]
		validateA2A(t, "SendMessageRequest", ex.Request["params"])
		_, got := e.call(a2aTokPeer, "SendMessage", ex.Request["params"])
		want, have := fixtureTask(ex.Response), taskOf(t, got)
		assert.Equal(t, stateOf(want), stateOf(have))
		assert.Equal(t, want["artifacts"].([]any)[0].(map[string]any)["name"], have["artifacts"].([]any)[0].(map[string]any)["name"])
		validateA2A(t, "Task", have)
	})
	t.Run("write-input-required", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		f := loadA2AFixture(t, "write-input-required.json")
		ids := map[string]string{}
		var resolve ResolveFunc
		for i, ex := range f.Exchanges {
			if m := ex.Request["params"].(map[string]any)["message"]; m != nil {
				validateA2A(t, "SendMessageRequest", map[string]any{"message": m})
			}
			if i == 2 { // GetTask: the person approves first, no peer message
				_, resolve = e.presence.last(t)
				require.NoError(t, resolve(true))
				e.eventuallyState(a2aTokPeer, ids["task-2"], a2aCompleted)
			}
			params := replayIDs(ex.Request["params"], ids)
			_, got := e.call(a2aTokPeer, ex.Request["method"].(string), params)
			want := fixtureTask(ex.Response)
			have := rpcResult(t, got)
			if task, ok := have["task"].(map[string]any); ok {
				have = task
			}
			assert.Equal(t, stateOf(want), stateOf(have), "exchange %d", i+1)
			ids[want["id"].(string)], ids[want["contextId"].(string)] = have["id"].(string), have["contextId"].(string)
			validateA2A(t, "Task", have)
			if i == 0 {
				fm, hm := want["metadata"].(map[string]any)[a2aMetaKey].(map[string]any), have["metadata"].(map[string]any)[a2aMetaKey].(map[string]any)
				assert.Equal(t, fm["tool"], hm["tool"])
				assert.Equal(t, fm["class"], hm["class"])
			}
			if i == 1 {
				msg := have["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
				assert.Equal(t, want["status"].(map[string]any)["message"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"], msg)
			}
		}
		assert.True(t, e.pins.has("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku"))
	})
	t.Run("auth-required", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		f := loadA2AFixture(t, "auth-required.json")
		_, got := e.call(a2aTokReader, "SendMessage", f.Exchanges[0].Request["params"])
		have := taskOf(t, got)
		assert.Equal(t, stateOf(fixtureTask(f.Exchanges[0].Response)), stateOf(have))
		assert.Equal(t, fixtureTask(f.Exchanges[0].Response)["status"].(map[string]any)["message"].(map[string]any)["parts"], have["status"].(map[string]any)["message"].(map[string]any)["parts"])
		validateA2A(t, "Task", have)
	})
	t.Run("stream-read", func(t *testing.T) {
		e := newA2AEnv(t, a2aEnvOpts{})
		f := loadA2AFixture(t, "stream-read.json")
		resp := e.open(a2aTokPeer, f.Exchanges[0].Request["method"].(string), f.Exchanges[0].Request["params"], nil)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		evs := readStream(t, resp)
		require.NotEmpty(t, evs)
		// A2A 1.0.0 section 3.1.2: the stream begins with the Task, in
		// submitted, and no Task follows.
		first, ok := rpcResult(t, evs[0])["task"].(map[string]any)
		require.True(t, ok, "event 0 must be a task: %v", evs[0])
		assert.Equal(t, "TASK_STATE_SUBMITTED", stateOf(first))
		for i, ev := range evs[1:] {
			assert.NotContains(t, rpcResult(t, ev), "task", "event %d is a task after the first event", i+1)
		}
		// The terminal state is the one the fixture ends with.
		fixtureStream := f.Exchanges[0].Stream
		wantFinal := fixtureStream[len(fixtureStream)-1]["result"].(map[string]any)["statusUpdate"].(map[string]any)["status"].(map[string]any)["state"]
		last := rpcResult(t, evs[len(evs)-1])["statusUpdate"].(map[string]any)
		assert.Equal(t, wantFinal, last["status"].(map[string]any)["state"])
		for _, ev := range evs {
			validateA2A(t, "StreamResponse", rpcResult(t, ev))
		}
	})
	t.Run("agent-card", func(t *testing.T) {
		f := loadA2AFixture(t, "agent-card.json")
		validateA2A(t, "AgentCard", f.Card)
	})
}
