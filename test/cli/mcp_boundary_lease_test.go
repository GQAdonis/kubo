//go:build !noagent && !windows

package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ipfs/kubo/mcp/contract"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// This optional setup fixture never represents a scenario PASS. Its private
// FD protocol is documented in mcp_boundary_lease.md; absent collaborators emit
// BLOCKED. It launches no provider/UAR and reads no operator credentials.
type boundaryLeaseSetup struct {
	Version         int                  `json:"version"`
	TestOnly        bool                 `json:"test_only"`
	Mode            string               `json:"mode"`
	MakeBuildSHA256 string               `json:"make_build_sha256"`
	AllowedOrigins  []string             `json:"allowed_origins"`
	Collaborator    boundaryCollaborator `json:"collaborator"`
}

type boundaryCollaborator struct {
	TestOnly        bool                    `json:"test_only"`
	Disposable      bool                    `json:"disposable"`
	LeaseID         string                  `json:"lease_id"`
	OwnerPID        int                     `json:"owner_pid"`
	Endpoint        string                  `json:"endpoint"`
	Model           string                  `json:"model"`
	Credential      string                  `json:"credential,omitempty"`
	Provider        string                  `json:"provider,omitempty"`
	TrustedHostID   string                  `json:"trusted_host_id,omitempty"`
	ReleaseArtifact string                  `json:"release_artifact,omitempty"`
	ReleaseReceipt  *boundaryReleaseReceipt `json:"release_receipt,omitempty"`
}

// This binds the external owner's actual public inspection artifact. The final
// runner must retain that artifact and check its bytes against SHA256; neither
// a version card nor this binding alone proves the pinned image was running.
type boundaryReleaseReceipt struct {
	Kind       string `json:"kind"`
	LeaseID    string `json:"lease_id"`
	OwnerPID   int    `json:"owner_pid"`
	Endpoint   string `json:"endpoint"`
	Artifact   string `json:"artifact"`
	Subject    string `json:"subject"`
	ObservedAt string `json:"observed_at"`
	SHA256     string `json:"sha256"`
}

func verifiedBoundaryReceiptBinding(c boundaryCollaborator, artifact string, now time.Time) bool {
	r := c.ReleaseReceipt
	if r == nil || (r.Kind != "image_inspect" && r.Kind != "source_process") || r.LeaseID != c.LeaseID || r.OwnerPID != c.OwnerPID || r.Endpoint != c.Endpoint || r.Artifact != artifact || !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`).MatchString(r.Subject) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(r.SHA256) {
		return false
	}
	observed, err := time.Parse(time.RFC3339, r.ObservedAt)
	return err == nil && !observed.After(now.Add(5*time.Second)) && !observed.Before(now.Add(-5*time.Minute))
}

// Child output is bounded, memory-only and never emitted on a failure. Only a
// parsed listener address leaves this buffer; backend bodies may be sensitive.
type boundaryCapture struct {
	mu   sync.Mutex
	data []byte
}

func (b *boundaryCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(b.data) < 65536 {
		b.data = append(b.data, p[:min(n, 65536-len(b.data))]...)
	}
	return n, nil
}
func (b *boundaryCapture) text() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.data) }

type boundaryChild struct {
	cmd  *exec.Cmd
	done chan error
}

func startBoundaryChild(ctx context.Context, bin string, args, env []string, dir string, capture io.Writer) (*boundaryChild, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdout = capture
	cmd.Stderr = capture
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 3 * time.Second
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	child := &boundaryChild{cmd: cmd, done: make(chan error, 1)}
	go func() { child.done <- cmd.Wait(); close(child.done) }()
	return child, nil
}
func (p *boundaryChild) stop() {
	_ = p.cmd.Process.Signal(os.Interrupt)
	select {
	case <-p.done:
		return
	case <-time.After(3 * time.Second):
	}
	_ = p.cmd.Process.Kill()
	<-p.done
}

func boundaryToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value[:]), nil
}
func boundaryLoopback(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || !mcpserver.IsLocalHost(u.Host) || u.Port() == "" {
		return nil, false
	}
	port, err := strconv.Atoi(u.Port())
	return u, err == nil && port > 0 && port <= 65535
}
func boundaryHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func boundaryJSON(ctx context.Context, client *http.Client, method, endpoint, credential string, body any, result any) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if credential != "" {
		r.Header.Set("Authorization", "Bearer "+credential)
	}
	r.Header.Set("Content-Type", "application/json")
	response, err := client.Do(r)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("collaborator request refused")
	}
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return err
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
}

func validateBoundarySetup(input boundaryLeaseSetup) bool {
	if input.Version != 1 || !input.TestOnly || (input.Mode != "builtin" && input.Mode != "uar") || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(input.MakeBuildSHA256) || len(input.AllowedOrigins) > 8 {
		return false
	}
	c := input.Collaborator
	if !c.TestOnly || !c.Disposable || c.OwnerPID <= 0 || c.OwnerPID == os.Getpid() || !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(c.LeaseID) || len(c.Credential) > 16384 || !utf8.ValidString(c.Model) || len(c.Model) == 0 || len(c.Model) > 256 {
		return false
	}
	if _, ok := boundaryLoopback(c.Endpoint); !ok {
		return false
	}
	for _, origin := range input.AllowedOrigins {
		u, ok := boundaryLoopback(origin)
		if !ok || u.Path != "" && u.Path != "/" {
			return false
		}
	}
	if input.Mode == "uar" && (c.Credential == "" || !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`).MatchString(c.Provider) || !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`).MatchString(c.TrustedHostID)) {
		return false
	}
	return true
}

func decodeBoundarySetup(raw []byte) (boundaryLeaseSetup, bool) {
	var input boundaryLeaseSetup
	if len(raw) > 65536 || !utf8.Valid(raw) {
		return input, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || !validateBoundarySetup(input) {
		return boundaryLeaseSetup{}, false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return boundaryLeaseSetup{}, false
	}
	return input, true
}

func TestMCPBoundaryLeaseSetupRejectsUnownedAndCredentialURLs(t *testing.T) {
	valid := boundaryLeaseSetup{Version: 1, TestOnly: true, Mode: "builtin", MakeBuildSHA256: strings.Repeat("a", 64), AllowedOrigins: []string{"http://127.0.0.1:19101"}, Collaborator: boundaryCollaborator{TestOnly: true, Disposable: true, LeaseID: "owned-provider", OwnerPID: os.Getpid() + 1, Endpoint: "http://127.0.0.1:19102/v1", Model: "qwen3:8b"}}
	encode := func(input boundaryLeaseSetup) []byte {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal("fixture setup encoding failed")
		}
		return raw
	}
	if _, ok := decodeBoundarySetup(encode(valid)); !ok {
		t.Fatal("test-only owned setup must decode")
	}
	for name, change := range map[string]func(*boundaryLeaseSetup){
		"operator service":      func(v *boundaryLeaseSetup) { v.Collaborator.TestOnly = false },
		"nondisposable":         func(v *boundaryLeaseSetup) { v.Collaborator.Disposable = false },
		"self ownership":        func(v *boundaryLeaseSetup) { v.Collaborator.OwnerPID = os.Getpid() },
		"remote endpoint":       func(v *boundaryLeaseSetup) { v.Collaborator.Endpoint = "https://example.test:443/v1" },
		"credential URL":        func(v *boundaryLeaseSetup) { v.Collaborator.Endpoint = "http://user:private@127.0.0.1:19102/v1" },
		"query credential":      func(v *boundaryLeaseSetup) { v.Collaborator.Endpoint += "?key=private" },
		"default port":          func(v *boundaryLeaseSetup) { v.Collaborator.Endpoint = "http://localhost/v1" },
		"invalid binary digest": func(v *boundaryLeaseSetup) { v.MakeBuildSHA256 = "unbuilt" },
		"origin path":           func(v *boundaryLeaseSetup) { v.AllowedOrigins = []string{"http://127.0.0.1:19101/path"} },
		"uar identity absent":   func(v *boundaryLeaseSetup) { v.Mode = "uar" },
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			change(&input)
			if _, ok := decodeBoundarySetup(encode(input)); ok {
				t.Fatal("unsafe setup was accepted")
			}
		})
	}
	for _, raw := range [][]byte{
		append(encode(valid), []byte(` {}`)...),
		[]byte(strings.Replace(string(encode(valid)), `"model":`, `"unknown":true,"model":`, 1)),
		[]byte(strings.Repeat(" ", 65537)),
		[]byte{0xff},
	} {
		if _, ok := decodeBoundarySetup(raw); ok {
			t.Fatal("unbounded, unknown or trailing setup was accepted")
		}
	}
	capture := &boundaryCapture{}
	if n, err := capture.Write(bytes.Repeat([]byte("x"), 100000)); err != nil || n != 100000 || len(capture.text()) != 65536 {
		t.Fatal("private child capture must be bounded without partial-write retries")
	}
}

func TestMCPBoundaryLeaseRequiresLiveOwnedReleaseReceipt(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	artifact := "sha256:" + strings.Repeat("a", 64)
	c := boundaryCollaborator{LeaseID: "owned-uar", OwnerPID: 12345, Endpoint: "http://127.0.0.1:19102"}
	if verifiedBoundaryReceiptBinding(c, artifact, now) {
		t.Fatal("card/version or configuration alone cannot verify the image pin")
	}
	r := boundaryReleaseReceipt{Kind: "image_inspect", LeaseID: c.LeaseID, OwnerPID: c.OwnerPID, Endpoint: c.Endpoint, Artifact: artifact, Subject: "owned-container-id", ObservedAt: now.Format(time.RFC3339), SHA256: strings.Repeat("b", 64)}
	c.ReleaseReceipt = &r
	if !verifiedBoundaryReceiptBinding(c, artifact, now) {
		t.Fatal("live owned public inspection binding must be preserved")
	}
	for name, change := range map[string]func(*boundaryReleaseReceipt){
		"other lease":           func(r *boundaryReleaseReceipt) { r.LeaseID = "operator-uar" },
		"other owner":           func(r *boundaryReleaseReceipt) { r.OwnerPID++ },
		"other endpoint":        func(r *boundaryReleaseReceipt) { r.Endpoint = "http://127.0.0.1:19103" },
		"other artifact":        func(r *boundaryReleaseReceipt) { r.Artifact = "sha256:" + strings.Repeat("c", 64) },
		"stale inspection":      func(r *boundaryReleaseReceipt) { r.ObservedAt = now.Add(-6 * time.Minute).Format(time.RFC3339) },
		"future inspection":     func(r *boundaryReleaseReceipt) { r.ObservedAt = now.Add(time.Minute).Format(time.RFC3339) },
		"missing artifact hash": func(r *boundaryReleaseReceipt) { r.SHA256 = "" },
		"unsupported evidence":  func(r *boundaryReleaseReceipt) { r.Kind = "assertion" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := r
			change(&changed)
			copy := c
			copy.ReleaseReceipt = &changed
			if verifiedBoundaryReceiptBinding(copy, artifact, now) {
				t.Fatal("unverified release binding must block fixture readiness")
			}
		})
	}
}

// TestMCPBoundaryLease is an opt-in owned setup process for browser UI108/111.
// It requires an already make-built binary, never builds/downloads/runs mocks,
// and joins exactly its own daemon/MCP children on stdin EOF or max lifetime.
func TestMCPBoundaryLease(t *testing.T) {
	if os.Getenv("IPFS_MCP_BOUNDARY_LEASE") != "1" {
		t.Skip("leased browser fixture is opt-in; no scenario acceptance implied")
	}
	public := os.NewFile(4, "boundary-public")
	private := os.NewFile(5, "boundary-private")
	configFD := os.NewFile(3, "boundary-setup")
	defer public.Close()
	defer private.Close()
	defer configFD.Close()
	emit := func(value any) { _ = json.NewEncoder(public).Encode(value) }
	blocked := func(reason string) {
		emit(map[string]any{"status": "BLOCKED", "reason": reason})
		t.Skip("BLOCKED: leased fixture unavailable; see public status channel")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()
	leaseClosed := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); close(leaseClosed); cancel() }()
	defer func() { _ = os.Stdin.Close(); <-leaseClosed }()
	stopConfigClose := context.AfterFunc(ctx, func() { _ = configFD.Close() })
	defer stopConfigClose()
	raw, err := io.ReadAll(io.LimitReader(configFD, 65537))
	if err != nil || len(raw) > 65536 {
		blocked("invalid_setup_channel")
		return
	}
	input, valid := decodeBoundarySetup(raw)
	if !valid {
		blocked("invalid_test_only_setup")
		return
	}
	if ctx.Err() != nil {
		blocked("lease_closed")
		return
	}
	root, err := os.Getwd()
	if err != nil {
		blocked("repository_root_required")
		return
	}
	bin := filepath.Join(root, "cmd", "ipfs", "ipfs")
	file, err := os.Open(bin)
	if err != nil {
		blocked("make_built_binary_unavailable")
		return
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	_ = file.Close()
	if err != nil || hex.EncodeToString(hash.Sum(nil)) != input.MakeBuildSHA256 {
		blocked("make_build_hash_mismatch")
		return
	}
	client := boundaryHTTPClient()
	defer client.CloseIdleConnections()
	base := strings.TrimRight(input.Collaborator.Endpoint, "/")
	var pinned struct {
		MinVersion string `json:"minVersion"`
		PinnedFrom struct {
			Artifact string `json:"artifact"`
		} `json:"pinnedFrom"`
	}
	if input.Mode == "builtin" {
		var models struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if boundaryJSON(ctx, client, "GET", base+"/models", input.Collaborator.Credential, nil, &models) != nil {
			blocked("real_provider_unavailable")
			return
		}
		found := false
		for _, model := range models.Data {
			if model.ID == input.Collaborator.Model {
				found = true
			}
		}
		if !found {
			blocked("configured_model_unavailable")
			return
		}
	} else {
		schema, readErr := fs.ReadFile(contract.FS(), "agentic/uar-preflight.json")
		if readErr != nil || json.Unmarshal(schema, &pinned) != nil || input.Collaborator.ReleaseArtifact != pinned.PinnedFrom.Artifact {
			blocked("pinned_release_attestation_mismatch")
			return
		}
		if !verifiedBoundaryReceiptBinding(input.Collaborator, pinned.PinnedFrom.Artifact, time.Now()) {
			blocked("pinned_release_unverified")
			return
		}
		var card struct {
			Version string `json:"version"`
		}
		if boundaryJSON(ctx, client, "GET", base+"/.well-known/agent-card.json", "", nil, &card) != nil {
			blocked("real_uar_unavailable")
			return
		}
		if card.Version != pinned.MinVersion {
			blocked("pinned_uar_release_mismatch")
			return
		}
	}
	scratch := t.TempDir()
	repo := filepath.Join(scratch, "repo")
	env := []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + scratch, "IPFS_PATH=" + repo, "IPFS_TELEMETRY=off", "DO_NOT_TRACK=1", "IPFS_MCP_CONFIRM=none"}
	run := func(args []string, stdin string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = env
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(stdin)
		cmd.Stderr = io.Discard
		return cmd.Output()
	}
	if _, err := run([]string{"init", "--profile=test"}, ""); err != nil {
		blocked("isolated_daemon_init_failed")
		return
	}
	configPath := filepath.Join(repo, "config")
	daemonRaw, err := os.ReadFile(configPath)
	var daemonConfig map[string]any
	if err != nil || json.Unmarshal(daemonRaw, &daemonConfig) != nil {
		blocked("isolated_daemon_config_failed")
		return
	}
	daemonConfig["Bootstrap"] = []any{}
	daemonConfig["Addresses"] = map[string]any{"API": "/ip4/127.0.0.1/tcp/0", "Gateway": "/ip4/127.0.0.1/tcp/0", "Swarm": []string{}}
	daemonConfig["API"] = map[string]any{"HTTPHeaders": map[string]any{"Access-Control-Allow-Origin": input.AllowedOrigins, "Access-Control-Allow-Methods": []string{"POST", "GET"}}}
	daemonRaw, err = json.Marshal(daemonConfig)
	if err != nil || os.WriteFile(configPath, daemonRaw, 0o600) != nil {
		blocked("isolated_daemon_config_failed")
		return
	}
	children := make([]*boundaryChild, 0, 2)
	defer func() {
		cancel()
		for i := len(children) - 1; i >= 0; i-- {
			children[i].stop()
		}
	}()
	daemon, err := startBoundaryChild(ctx, bin, []string{"daemon", "--offline"}, env, root, io.Discard)
	if err != nil {
		blocked("isolated_daemon_start_failed")
		return
	}
	children = append(children, daemon)
	var rpcURL string
	setupEnd := time.Now().Add(20 * time.Second)
	apiPattern := regexp.MustCompile(`^/ip4/127\.0\.0\.1/tcp/([0-9]+)$`)
	for ctx.Err() == nil && time.Now().Before(setupEnd) {
		api, _ := os.ReadFile(filepath.Join(repo, "api"))
		if match := apiPattern.FindStringSubmatch(strings.TrimSpace(string(api))); match != nil {
			rpcURL = "http://127.0.0.1:" + match[1]
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	if rpcURL == "" {
		blocked("isolated_rpc_unavailable")
		return
	}
	cids := make([]string, 0, 2)
	for _, text := range []string{"boundary explicit approval fixture\n", "boundary explicit denial fixture\n"} {
		out, err := run([]string{"add", "--pin=false", "--cid-version=1", "--chunker=size-262144", "-Q"}, text)
		if err != nil {
			blocked("explicit_seed_failed")
			return
		}
		cids = append(cids, strings.TrimSpace(string(out)))
	}
	uiToken, err := boundaryToken()
	if err != nil {
		blocked("ephemeral_auth_unavailable")
		return
	}
	uarToken, err := boundaryToken()
	if err != nil {
		blocked("ephemeral_auth_unavailable")
		return
	}
	leaseID, err := boundaryToken()
	if err != nil {
		blocked("ephemeral_auth_unavailable")
		return
	}
	leaseID = "boundary-" + leaseID[:22]
	llm := map[string]any{"base_url": base, "model": input.Collaborator.Model}
	if input.Collaborator.Credential != "" {
		llm["api_key"] = map[string]any{"env": "KUBO_BOUNDARY_BACKEND_KEY"}
	}
	principals := []any{map[string]any{"name": "boundary-ui", "kind": "ui", "credential": map[string]any{"env": "KUBO_BOUNDARY_UI_TOKEN"}, "write_groups": []string{"pin"}, "egress_ack": []string{"local"}}}
	if input.Mode == "uar" {
		llm["kind"] = "uar"
		llm["uar"] = map[string]any{"agent_id": leaseID, "provider": input.Collaborator.Provider, "mcp_name": leaseID, "mcp_principal": "boundary-uar", "mcp_credential_revision": leaseID}
		principals = append(principals, map[string]any{"name": "boundary-uar", "kind": "uar", "credential": map[string]any{"env": "KUBO_BOUNDARY_UAR_TOKEN"}, "write_groups": []string{"pin"}, "egress_ack": []string{"local"}, "check": "assert", "uar": map[string]any{"card_url": base + "/.well-known/agent-card.json"}})
	}
	mcpConfig := map[string]any{"agent": map[string]any{"agui": true, "origin_allowlist": input.AllowedOrigins}, "principals": principals, "llm": llm}
	mcpRaw, err := json.Marshal(mcpConfig)
	mcpPath := filepath.Join(scratch, "mcp.json")
	if err != nil || os.WriteFile(mcpPath, mcpRaw, 0o600) != nil {
		blocked("fixture_config_failed")
		return
	}
	env = append(env, "KUBO_BOUNDARY_UI_TOKEN="+uiToken, "KUBO_BOUNDARY_UAR_TOKEN="+uarToken, "KUBO_BOUNDARY_BACKEND_KEY="+input.Collaborator.Credential)
	capture := &boundaryCapture{}
	mcp, err := startBoundaryChild(ctx, bin, []string{"mcp", "--http=127.0.0.1:0", "--mcp-config", mcpPath, "--settings-dir", filepath.Join(scratch, "settings")}, env, root, capture)
	if err != nil {
		blocked("fixture_mcp_start_failed")
		return
	}
	children = append(children, mcp)
	var endpoint string
	setupEnd = time.Now().Add(20 * time.Second)
	for ctx.Err() == nil && time.Now().Before(setupEnd) {
		if match := mcpHTTPListening.FindStringSubmatch(capture.text()); match != nil {
			endpoint = strings.TrimSuffix(match[1], "/mcp")
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	if endpoint == "" {
		blocked("fixture_mcp_unavailable")
		return
	}
	if boundaryJSON(ctx, client, "GET", endpoint+"/decisions", uiToken, nil, &map[string]any{}) != nil {
		blocked("protected_read_unavailable")
		return
	}
	if input.Mode == "uar" {
		registration := map[string]any{"name": leaseID, "enabled": true, "transport": "remote_http", "url": endpoint + "/mcp", "env": map[string]any{}, "headers": map[string]any{}, "sandboxed": false,
			"grant_policy": map[string]any{"destination_id": leaseID, "trusted_hosts": []string{input.Collaborator.TrustedHostID}, "required_scopes": []string{"ipfs"}, "allow_private_http": true, "allowed_agent_ids": []string{leaseID}}}
		if boundaryJSON(ctx, client, "PUT", base+"/api/uar/mcp/servers/"+leaseID, input.Collaborator.Credential, registration, nil) != nil {
			blocked("owned_uar_registration_unavailable")
			return
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = boundaryJSON(cleanup, client, "DELETE", base+"/api/uar/mcp/servers/"+leaseID, input.Collaborator.Credential, nil, nil)
			_ = boundaryJSON(cleanup, client, "DELETE", base+"/api/agents/"+leaseID, input.Collaborator.Credential, nil, nil)
		}()
	}
	if json.NewEncoder(private).Encode(map[string]any{"lease_id": leaseID, "ui_bearer": uiToken}) != nil {
		blocked("private_auth_channel_unavailable")
		return
	}
	_ = private.Close()
	fingerprint := sha256.Sum256(mcpRaw)
	emit(map[string]any{"status": "READY", "version": 1, "test_only": true, "mode": input.Mode, "lease_id": leaseID, "endpoint": endpoint + "/agui", "decisions_url": endpoint + "/decisions", "rpc_url": rpcURL, "fixture_cids": cids, "config_fingerprint": hex.EncodeToString(fingerprint[:]), "make_build_sha256": input.MakeBuildSHA256,
		"owned_pids": []int{daemon.cmd.Process.Pid, mcp.cmd.Process.Pid}, "collaborator": map[string]any{"lease_id": input.Collaborator.LeaseID, "owner_pid": input.Collaborator.OwnerPID, "lifecycle": "external_owner", "release_artifact_attestation": input.Collaborator.ReleaseArtifact, "release_receipt": input.Collaborator.ReleaseReceipt},
		"native_node_confirmation_available": false, "authentication": "private_fd5", "browser_capture": "no_trace_or_network"})
	select {
	case <-leaseClosed:
	case <-ctx.Done():
	}
	cancel()
	for i := len(children) - 1; i >= 0; i-- {
		children[i].stop()
	}
	children = nil
	emit(map[string]any{"status": "CLOSED", "lease_id": leaseID, "owned_children_joined": true})
}
