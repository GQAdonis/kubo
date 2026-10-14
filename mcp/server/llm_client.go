//go:build !noagent

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Limits on what a model backend can make this process hold in memory.
const (
	// maxSSELine bounds one SSE line. A chunk carries a few tokens; a backend
	// that sends more is broken or hostile.
	maxSSELine = 1 << 20
	// maxLLMResponse bounds one whole response.
	maxLLMResponse = 16 << 20
	// maxErrorSnippet is how much of a backend error message is kept.
	maxErrorSnippet = 200
)

// ChatMessage is one message of an OpenAI-compatible chat.
type ChatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []ChatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

// ChatToolCall is a tool call the model requested. Arguments is the JSON text
// the model produced; it is not yet validated.
type ChatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ChatFunctionCall `json:"function"`
}

// ChatFunctionCall is the function part of a tool call.
type ChatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatTool is a tool definition offered to the model.
type ChatTool struct {
	Type     string           `json:"type"`
	Function ChatToolFunction `json:"function"`
}

// ChatToolFunction describes one function.
type ChatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatUsage is the token accounting a backend reports.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResult is one assembled model reply.
type ChatResult struct {
	Content      string
	ToolCalls    []ChatToolCall
	FinishReason string
	Usage        ChatUsage
	// UsageReported is false when the backend sent no usage, so callers estimate.
	UsageReported bool
}

// LLMError is a failure of the model backend. It carries the status and a short
// excerpt of the backend's error message, never the request.
type LLMError struct {
	Status  int
	Message string
}

// Error implements error.
func (e *LLMError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("model backend answered HTTP %d", e.Status)
	}
	return fmt.Sprintf("model backend answered HTTP %d: %s", e.Status, e.Message)
}

// LLMClient is an OpenAI-compatible chat-completions client built on the
// standard library. It is created only for a configured backend; nothing in
// this package dials a model on its own.
type LLMClient struct {
	endpoint string
	model    string
	key      secret
	timeout  time.Duration
	hc       *http.Client
}

// NewLLMClient builds a client for the settings. The key, if any, is read now.
func NewLLMClient(set AgentSettings) (*LLMClient, error) {
	if set.BaseURL == "" || set.Model == "" {
		return nil, errors.New("model backend is not configured")
	}
	if err := validateBackendURL(set.BaseURL); err != nil {
		return nil, err
	}
	c := &LLMClient{
		endpoint: strings.TrimRight(set.BaseURL, "/") + "/chat/completions",
		model:    set.Model,
		timeout:  set.Timeout,
		hc:       set.HTTPClient,
	}
	if c.timeout <= 0 {
		c.timeout = DefaultLLMTimeout
	}
	if set.APIKey != nil {
		k, err := set.APIKey.Read()
		if err != nil {
			return nil, fmt.Errorf("llm api key: %w", err)
		}
		c.key = secret(k)
	}
	if c.hc == nil {
		c.hc = &http.Client{}
	}
	// A redirect would carry the Authorization header to another host. The
	// operator named one endpoint; a redirect is an answer, not a detour.
	hc := *c.hc
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.hc = &hc
	return c, nil
}

// dropKey discards the key material of a swapped-out client. It runs only when
// the last snapshot reference to the client has left, so no Chat is running or
// can start on it; the client is never used again afterwards.
func (c *LLMClient) dropKey() { c.key = "" }

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []ChatMessage  `json:"messages"`
	Tools         []ChatTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions map[string]any `json:"stream_options,omitempty"`
}

// Chat sends one chat-completions request with streaming on and assembles the
// reply. onText, if not nil, receives each content delta as it arrives. The
// whole call, including the stream, is bounded by the configured timeout.
func (c *LLMClient) Chat(ctx context.Context, msgs []ChatMessage, tools []ChatTool, onText func(string)) (*ChatResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := json.Marshal(chatRequest{
		Model:         c.model,
		Messages:      msgs,
		Tools:         tools,
		Stream:        true,
		StreamOptions: map[string]any{"include_usage": true},
	})
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key.reveal())
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		// The error text of net/http can include the URL; the URL carries no
		// credential (validateBackendURL), so it is safe to keep.
		return nil, fmt.Errorf("model backend request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, backendError(resp)
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mt == "application/json" {
		return decodeCompletion(io.LimitReader(resp.Body, maxLLMResponse), onText)
	}
	return assembleStream(io.LimitReader(resp.Body, maxLLMResponse), onText)
}

func backendError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var env struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := ""
	if json.Unmarshal(raw, &env) == nil {
		msg = env.Error.Message
	}
	return &LLMError{Status: resp.StatusCode, Message: clipRunes(strings.TrimSpace(msg), maxErrorSnippet)}
}

// chunk is one streamed chat.completion.chunk.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    *int   `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *ChatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callBuilder assembles one tool call from its deltas.
type callBuilder struct {
	id, name string
	args     strings.Builder
}

// assembler folds chunks into a ChatResult. Tool-call deltas are keyed by their
// index; a backend that sends no index starts a new call at each delta that
// carries an id and otherwise continues the last one.
type assembler struct {
	content strings.Builder
	calls   map[int]*callBuilder
	last    int
	next    int
	res     ChatResult
	onText  func(string)
}

func newAssembler(onText func(string)) *assembler {
	return &assembler{calls: map[int]*callBuilder{}, last: -1, onText: onText}
}

func (a *assembler) add(ch chunk) error {
	if ch.Error != nil {
		return &LLMError{Status: http.StatusOK, Message: clipRunes(ch.Error.Message, maxErrorSnippet)}
	}
	if ch.Usage != nil {
		a.res.Usage, a.res.UsageReported = *ch.Usage, true
		if a.res.Usage.TotalTokens == 0 {
			a.res.Usage.TotalTokens = a.res.Usage.PromptTokens + a.res.Usage.CompletionTokens
		}
	}
	for _, choice := range ch.Choices {
		if t := choice.Delta.Content; t != "" {
			a.content.WriteString(t)
			if a.onText != nil {
				a.onText(t)
			}
		}
		for _, tc := range choice.Delta.ToolCalls {
			idx := a.indexFor(tc.Index, tc.ID)
			b := a.calls[idx]
			if b == nil {
				b = &callBuilder{}
				a.calls[idx] = b
			}
			if tc.ID != "" && b.id == "" {
				b.id = tc.ID
			}
			if n := tc.Function.Name; n != "" && n != b.name {
				b.name += n
			}
			b.args.WriteString(tc.Function.Arguments)
			a.last = idx
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			a.res.FinishReason = *choice.FinishReason
		}
	}
	return nil
}

func (a *assembler) indexFor(index *int, id string) int {
	switch {
	case index != nil:
		if *index >= a.next {
			a.next = *index + 1
		}
		return *index
	case id != "" || a.last < 0:
		idx := a.next
		a.next++
		return idx
	default:
		return a.last
	}
}

func (a *assembler) result() *ChatResult {
	a.res.Content = a.content.String()
	idxs := make([]int, 0, len(a.calls))
	for i := range a.calls {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for n, i := range idxs {
		b := a.calls[i]
		id := b.id
		if id == "" {
			id = fmt.Sprintf("call_%d", n)
		}
		args := b.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		a.res.ToolCalls = append(a.res.ToolCalls, ChatToolCall{
			ID: id, Type: "function", Function: ChatFunctionCall{Name: b.name, Arguments: args},
		})
	}
	return &a.res
}

// assembleStream reads an SSE chat-completions stream.
func assembleStream(r io.Reader, onText func(string)) (*ChatResult, error) {
	a := newAssembler(onText)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxSSELine)
	var data []string
	flush := func() (done bool, err error) {
		if len(data) == 0 {
			return false, nil
		}
		payload := strings.Join(data, "\n")
		data = data[:0]
		if strings.TrimSpace(payload) == "[DONE]" {
			return true, nil
		}
		var ch chunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			return false, fmt.Errorf("model backend sent a malformed stream event: %w", err)
		}
		return false, a.add(ch)
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "":
			done, err := flush()
			if err != nil {
				return nil, err
			}
			if done {
				return a.result(), nil
			}
		case strings.HasPrefix(line, ":"):
			// comment / keep-alive
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read model stream: %w", err)
	}
	// A stream that ends without a blank line still delivers its last event.
	if _, err := flush(); err != nil {
		return nil, err
	}
	return a.result(), nil
}

// decodeCompletion reads a non-streaming reply, for backends that ignore
// stream:true.
func decodeCompletion(r io.Reader, onText func(string)) (*ChatResult, error) {
	var cc struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *ChatUsage `json:"usage"`
	}
	if err := json.NewDecoder(r).Decode(&cc); err != nil {
		return nil, fmt.Errorf("decode model reply: %w", err)
	}
	res := &ChatResult{}
	if cc.Usage != nil {
		res.Usage, res.UsageReported = *cc.Usage, true
		if res.Usage.TotalTokens == 0 {
			res.Usage.TotalTokens = res.Usage.PromptTokens + res.Usage.CompletionTokens
		}
	}
	if len(cc.Choices) == 0 {
		return res, nil
	}
	m := cc.Choices[0]
	res.Content, res.FinishReason = m.Message.Content, m.FinishReason
	if res.Content != "" && onText != nil {
		onText(res.Content)
	}
	for i, tc := range m.Message.ToolCalls {
		id := tc.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		args := tc.Function.Arguments
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		res.ToolCalls = append(res.ToolCalls, ChatToolCall{ID: id, Type: "function", Function: ChatFunctionCall{Name: tc.Function.Name, Arguments: args}})
	}
	return res, nil
}
