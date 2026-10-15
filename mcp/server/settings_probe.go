//go:build !noagent

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/ipfs/kubo/mcp/server/embeddings"
)

// Routes of the settings surface's probe endpoints, pinned here by
// mcp-agent-settings-api 1.4 (the client contract in mcp/contract/settings/
// takes the paths from this change). Both live under the settings route's
// subtree and share its admin-only scoping.
const (
	RouteSettingsTestConnection = RouteSettings + "/test-connection"
	RouteSettingsModels         = RouteSettings + "/models"
)

const (
	// probeTimeout bounds one test-connection or model-list probe. A candidate
	// URL has no timeout configuration of its own, and an unbounded probe
	// would let a picked endpoint hang the settings route.
	probeTimeout = 10 * time.Second
	// maxModelListBytes bounds a backend's model-list reply.
	maxModelListBytes = 4 << 20
	// maxModelListEntries is the contract's maxItems on the models array.
	maxModelListEntries = 10000
	// maxProbeDetail bounds the detail string of a failed probe.
	maxProbeDetail = 500
)

// testConnectionBody is the POST body of the test-connection endpoint
// (contract settings/test-connection.schema.json).
type testConnectionBody struct {
	BaseURL string `json:"base_url"`
	Kind    string `json:"kind"`
	Schema  string `json:"$schema"`
}

// testConnectionResult is the probe endpoints' answer shape on both success
// and failure (contract TestConnectionResult). A failed model list answers in
// this shape too: the settings error code set is closed and has no code for
// "backend unreachable", and the client already renders ok/detail.
type testConnectionResult struct {
	OK bool `json:"ok"`
	// KeySent says whether the stored key was attached to the probe. The stored
	// key is sent only to the current backend's scheme, host and port, so it is
	// false for any other candidate and when no key is stored.
	KeySent bool   `json:"key_sent"`
	Detail  string `json:"detail,omitempty"`
}

// keyWithheldDetail explains a probe that was answered 401 or 403 without the
// stored key.
const keyWithheldDetail = "the host answered but needs a key; the stored key is sent only to the current backend's host and port"

// modelListResult is the model-list endpoint's success shape (contract
// settings/model-list.schema.json, ModelListResult).
type modelListResult struct {
	Models []modelEntry `json:"models"`
}

type modelEntry struct {
	ID string `json:"id"`
}

// testConnection probes a candidate backend URL once and reports whether it
// answered as the requested kind. The request's only side effect is the one
// bounded outbound probe: nothing is persisted and the running settings are
// untouched (spec "Test connection persists nothing").
func (h *settingsHandler) testConnection(w http.ResponseWriter, r *http.Request) {
	var req testConnectionBody
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "the test-connection request is not valid JSON", http.StatusBadRequest)
		return
	}
	if err := checkProbeKind(req.Kind); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCandidateURL(req.BaseURL, h.st.insecureHTTP); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key, sent, withheld := h.probeKey(req.BaseURL)
	_, err := probeOpenAIModels(r.Context(), req.BaseURL, key)
	writeSettingsJSON(w, http.StatusOK, "", probeResult(err, key, sent, withheld, true))
}

// listModels answers GET on the model-list endpoint. With no base_url it lists
// the configured backend (503 backend_not_configured when there is none); a
// base_url is a candidate under the same URL rules as test-connection.
// Nothing is persisted.
func (h *settingsHandler) listModels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	base := q.Get("base_url")
	if base == "" {
		h.st.mu.Lock()
		cur := h.st.current
		h.st.mu.Unlock()
		if cur == nil {
			h.writeError(w, http.StatusServiceUnavailable, "backend_not_configured",
				"no agent backend is configured; pass base_url to list a candidate's models", nil)
			return
		}
		base = cur.BaseURL // validated by ResolveAgent when configured
	} else if err := validateCandidateURL(base, h.st.insecureHTTP); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := checkProbeKind(q.Get("kind")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key, sent, withheld := h.probeKey(base)
	ids, err := probeOpenAIModels(r.Context(), base, key)
	if err != nil {
		writeSettingsJSON(w, http.StatusOK, "", probeResult(err, key, sent, withheld, false))
		return
	}
	res := modelListResult{Models: make([]modelEntry, 0, len(ids))}
	for _, id := range ids {
		res.Models = append(res.Models, modelEntry{ID: id})
	}
	writeSettingsJSON(w, http.StatusOK, "", res)
}

// probeKey decides whether the stored key goes to the probed host. It does only
// for the current backend's own host and port (ADR-0012): an acknowledged host,
// the "local" sentinel and any other candidate typed by a client never receive
// it, because sending it there would hand it to whoever runs that host. sent is
// false when no key goes out, including when none is stored; withheld is true
// when a stored key was held back.
func (h *settingsHandler) probeKey(baseURL string) (key secret, sent, withheld bool) {
	// The key and the current backend are read in one critical section, so a
	// concurrent move cannot pair the old key with the new host.
	key, cur := h.st.keyAndCurrent()
	if key == "" {
		return "", false, false
	}
	if cur != nil && !backendMoved(cur.BaseURL, baseURL) {
		return key, true, false
	}
	return "", false, true
}

// probeResult renders a probe outcome. A 401 or 403 from a host that was probed
// without the stored key is reachable-but-key-not-sent: for test-connection
// (reachableOK) the host did answer, so ok is true and key_sent false says why
// it may still refuse; the model list has no field for that and reports ok
// false with the same key_sent and detail.
func probeResult(err error, key secret, sent, withheld, reachableOK bool) testConnectionResult {
	res := testConnectionResult{OK: err == nil, KeySent: sent}
	if err == nil {
		return res
	}
	var le *LLMError
	if withheld && errors.As(err, &le) && (le.Status == http.StatusUnauthorized || le.Status == http.StatusForbidden) {
		res.OK = reachableOK
		res.Detail = keyWithheldDetail
		return res
	}
	res.Detail = probeDetail(err, key)
	return res
}

// checkProbeKind validates the kind field of a probe request. uar is a
// contract kind this build does not serve, the same refusal as a settings
// write.
func checkProbeKind(kind string) error {
	switch kind {
	case "", "openai":
		return nil
	case "uar":
		return errors.New("backend kind uar is not served by this build")
	default:
		return errors.New("backend kind must be openai or uar")
	}
}

// validateCandidateURL checks the URL a probe dials or a settings write names.
// It is stricter than validateBackendURL, which an operator's own config file
// passes: a candidate arrives over the network from an admin client, so plain
// http is accepted only for a loopback host or a host the operator listed in
// insecureHTTP (exact match, see insecureHostListed), and credentials, query
// and fragment are refused. The errors never echo the value, which could carry
// credentials.
func validateCandidateURL(raw string, insecureHTTP []string) error {
	if raw == "" || len(raw) > maxSettingsURLBytes || strings.ContainsAny(raw, " \t\r\n") {
		return errors.New("base_url is not a valid http(s) URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("base_url is not a valid http(s) URL")
	}
	scheme := strings.ToLower(u.Scheme)
	switch {
	case scheme != "http" && scheme != "https":
		return errors.New("base_url must use https, or http on a loopback host")
	case u.User != nil:
		return errors.New("base_url must not contain credentials; the probe authenticates with the stored key, never with the URL")
	case u.RawQuery != "" || u.Fragment != "":
		return errors.New("base_url must not contain a query or fragment (keys do not belong in URLs)")
	case scheme == "http" && !IsLocalHost(u.Host) && !insecureHostListed(u, insecureHTTP):
		return errors.New("base_url may use plain http only on a loopback host or a host listed in embeddings.insecure_http_hosts; use https")
	}
	return nil
}

// insecureHostListed reports whether the plain-http URL's host (and, for a
// listed entry with a port, its port) is named in hosts, the operator's
// embeddings.insecure_http_hosts. Matching is case-insensitive and exact: a
// bare host covers every port, host:port covers that port only (an http URL
// without a port is port 80), and there are no wildcards.
func insecureHostListed(u *url.URL, hosts []string) bool {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		port = "80"
	}
	return slices.ContainsFunc(hosts, func(h string) bool {
		h = strings.ToLower(strings.TrimSpace(h))
		return h == host || h == host+":"+port
	})
}

// keyAndCurrent returns the key a probe may authenticate with: the currently stored
// backend key (flags winning over config and overlay, exactly as the resolved
// backend sees it), never a value sent with the probe request. An unresolved
// reference (absent file, unset variable) means the probe sends no key, never
// an error (CRITICAL-2).
func (st *settingsState) keyAndCurrent() (secret, *AgentSettings) {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.storedKeyLocked(), st.current
}

// storedKeyLocked is the key part of keyAndCurrent; st.mu must be held.
func (st *settingsState) storedKeyLocked() secret {
	ref := st.base.key
	if st.over.keyGiven() {
		if flagRef, err := keyRefFromFlags(st.over); err == nil {
			ref = &flagRef
		}
	}
	if ref == nil {
		return ""
	}
	k, err := ref.Read()
	if err != nil {
		return ""
	}
	return secret(k)
}

// probeOpenAIModels dials baseURL's OpenAI-compatible model list once, bounded
// by probeTimeout, with the stored key as the only credential. Redirects are
// not followed: a redirect would carry the key to another host.
func probeOpenAIModels(ctx context.Context, baseURL string, key secret) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	endpoint := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build model list request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key.reveal())
	}
	hc := &http.Client{
		Transport:     embeddings.DirectTransport(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Do(req)
	if err != nil {
		// The error text can include the URL; the URL carries no credential
		// (validateCandidateURL), so it is safe to keep.
		return nil, fmt.Errorf("backend probe: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, backendError(resp)
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxModelListBytes)).Decode(&list); err != nil {
		return nil, fmt.Errorf("the backend's model list is not valid JSON: %w", err)
	}
	ids := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if m.ID == "" || len(m.ID) > maxSettingsModelBytes {
			continue
		}
		ids = append(ids, m.ID)
		if len(ids) >= maxModelListEntries {
			break
		}
	}
	return ids, nil
}

// probeDetail renders a probe failure for the client. A backend's error
// excerpt can repeat what it was sent, so the key is scrubbed out of it.
func probeDetail(err error, key secret) string {
	msg := err.Error()
	if key != "" {
		msg = strings.ReplaceAll(msg, key.reveal(), "<redacted>")
	}
	return clipRunes(msg, maxProbeDetail)
}
