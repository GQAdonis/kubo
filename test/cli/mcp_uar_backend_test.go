//go:build !noagent

package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/kubo/mcp/contract"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// agent-backend-uar 1.5: `ipfs mcp` with llm.kind "uar" against the published
// Universal Agent Runtime v1.1.0 image, with liter-llm as UAR's model provider.
//
// The test starts the image by the digest pinned in
// mcp/contract/agentic/uar-preflight.json on a Docker-chosen loopback port,
// starts `ipfs mcp --http=0.0.0.0:0 --agui` (the OS chooses the port) against an
// isolated harness daemon, and checks:
//
//   - GET /api/agents/{id} on UAR shows kubo's configured system prompt (fixed
//     safety prefix included), an MCP binding that names the kubo server, and
//     skill bindings equal to the directory names under mcp/skills;
//   - one chat run through /agui completes;
//   - one write run ends in a tool_approval interrupt (translated to the kubo
//     shape) that resumes after approval and finishes;
//   - a denied write ends the run with nothing executed (the CID stays unpinned).
//
// A write the agent runs after approval still needs kubo's own confirmation: the
// uar principal never answers by elicitation, so with no resolver the tool
// reports the confirmation as unavailable and the run goes on. The test asserts
// the approval path and the denial; it does not require the pin to land.
//
// It never runs by default and is skipped, with the list of what is missing,
// unless the operator opts in. Nothing is defaulted that names a service,
// credential or model.
//
// Run it (from the repository root, after `make build`):
//
//	export PATH="$PWD/cmd/ipfs:$PATH"
//	export IPFS_MCP_TEST_UAR=1
//	# liter-llm, the operator's model provider for UAR (same names as the R7 test):
//	export IPFS_MCP_TEST_LLM_BASE_URL=https://liter.example/v1
//	export IPFS_MCP_TEST_LLM_API_KEY_ENV=LITER_API_KEY   # NAME of the variable that holds the key
//	export IPFS_MCP_TEST_LLM_MODEL=<model>
//	# UAR credentials, by variable name (values are never printed):
//	export IPFS_MCP_TEST_UAR_TOKEN_ENV=UAR_ISSUER_JWT        # issuer-signed JWT kubo presents (trusted host)
//	export IPFS_MCP_TEST_UAR_ADMIN_TOKEN_ENV=UAR_ADMIN_JWT   # admin JWT, to register the kubo destination
//	# A literal IP at which the UAR container reaches this machine (UAR refuses plaintext
//	# hostnames for a private destination), for example the docker bridge gateway:
//	export IPFS_MCP_TEST_UAR_KUBO_HOST_IP=172.17.0.1
//	go test ./test/cli/... -run TestMCPUARBackend -v
//
// Optional:
//
//	IPFS_MCP_TEST_UAR_IMAGE            image reference; default is the pinned digest of ghcr.io/prometheus-ags/universal-agent-runtime
//	IPFS_MCP_TEST_UAR_CONTAINER_PORT   port UAR listens on inside the container (default 8080)
//	IPFS_MCP_TEST_UAR_CONFIG_FILE      host path of a UAR configuration file, mounted read-only at
//	IPFS_MCP_TEST_UAR_CONFIG_MOUNT     this container path. It must require authentication (jwt_required) and list
//	                                   security.trusted_host_principals with host_id IPFS_MCP_TEST_UAR_TRUSTED_HOST
//	                                   (default "kubo-host") for the issuer of the kubo JWT; see mcp/uar/v1.1.0-kubo.md.
//	IPFS_MCP_TEST_UAR_DOCKER_ENV       extra container environment, KEY=VALUE separated by newlines or commas
//	                                   (model-provider variables of the image beyond LLM_MODEL, LLM_BASE_URL, LLM_API_KEY)
//	IPFS_MCP_TEST_UAR_PROVIDER         UAR provider id for llm.uar.provider
//	IPFS_MCP_TEST_UAR_CREDENTIAL_REVISION  run-grant credential_revision (default "kubo-it")
//	IPFS_MCP_TEST_UAR_CHECK            "verify" (default) or "assert" for the uar principal's check-uar mode
//
// What the image takes as configuration beyond the documented keys is not in the pinned contract; the
// container environment above (LLM_MODEL is documented, LLM_BASE_URL and LLM_API_KEY are the compose names
// kubo assumes) is the first thing to adjust if UAR reports no model.
const (
	uarRealOptInEnv     = "IPFS_MCP_TEST_UAR"
	uarTokenEnvName     = "IPFS_MCP_TEST_UAR_TOKEN_ENV"
	uarAdminEnvName     = "IPFS_MCP_TEST_UAR_ADMIN_TOKEN_ENV"
	uarHostIPEnv        = "IPFS_MCP_TEST_UAR_KUBO_HOST_IP"
	uarImageEnv         = "IPFS_MCP_TEST_UAR_IMAGE"
	uarContainerPortEnv = "IPFS_MCP_TEST_UAR_CONTAINER_PORT"
	uarConfigFileEnv    = "IPFS_MCP_TEST_UAR_CONFIG_FILE"
	uarConfigMountEnv   = "IPFS_MCP_TEST_UAR_CONFIG_MOUNT"
	uarDockerEnvEnv     = "IPFS_MCP_TEST_UAR_DOCKER_ENV"
	uarProviderEnv      = "IPFS_MCP_TEST_UAR_PROVIDER"
	uarRevisionEnv      = "IPFS_MCP_TEST_UAR_CREDENTIAL_REVISION"
	uarCheckEnv         = "IPFS_MCP_TEST_UAR_CHECK"
	uarTrustedHostEnv   = "IPFS_MCP_TEST_UAR_TRUSTED_HOST"

	uarImageRepository = "ghcr.io/prometheus-ags/universal-agent-runtime"
	uarTestAgentID     = "kubo-ipfs"
	uarTestMCPName     = "kubo-ipfs"
	uarTestPrompt      = "You are the IPFS agent of a test node. Be brief."
)

// uarRealSetup is the validated environment of the real-service run.
type uarRealSetup struct {
	llmBase, llmKeyEnv, llmModel string
	tokenEnv, adminEnv           string
	hostIP, image                string
	containerPort                string
	configFile, configMount      string
	dockerEnv                    []string
	provider, revision, check    string
	trustedHost                  string
}

// uarRealSetupOrSkip returns the setup, or skips the test naming what is
// missing. A setting that is present but unusable fails the test: the operator
// opted in, and a silent skip would hide a broken gate run.
func uarRealSetupOrSkip(t *testing.T) uarRealSetup {
	t.Helper()
	get := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	if get(uarRealOptInEnv) != "1" {
		t.Skipf("UAR v1.1.0 real-service test (agent-backend-uar 1.5) not enabled: set %s=1 and the variables in this test's header comment; the UAR backend is unverified against the published image until it runs", uarRealOptInEnv)
	}
	s := uarRealSetup{
		llmBase: get(literBaseURLEnv), llmKeyEnv: get(literKeyEnvName), llmModel: get(literModelEnvVar),
		tokenEnv: get(uarTokenEnvName), adminEnv: get(uarAdminEnvName), hostIP: get(uarHostIPEnv),
		image: get(uarImageEnv), containerPort: get(uarContainerPortEnv),
		configFile: get(uarConfigFileEnv), configMount: get(uarConfigMountEnv),
		provider: get(uarProviderEnv), revision: get(uarRevisionEnv), check: get(uarCheckEnv), trustedHost: get(uarTrustedHostEnv),
	}
	var missing []string
	for name, v := range map[string]string{
		literBaseURLEnv: s.llmBase, literKeyEnvName: s.llmKeyEnv, literModelEnvVar: s.llmModel,
		uarTokenEnvName: s.tokenEnv, uarAdminEnvName: s.adminEnv, uarHostIPEnv: s.hostIP,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("UAR real-service test: %s is set but %s are not; see the header comment of this test", uarRealOptInEnv, strings.Join(missing, ", "))
	}
	for _, name := range []string{s.llmKeyEnv, s.tokenEnv, s.adminEnv} {
		require.NotEmpty(t, os.Getenv(name), "the environment variable %s, named by the operator, is empty", name)
	}
	_, err := exec.LookPath("docker")
	require.NoError(t, err, "the real-service test starts the UAR image with docker")
	u, err := url.Parse(s.llmBase)
	require.NoError(t, err, "%s must be a URL", literBaseURLEnv)
	require.Empty(t, u.User, "%s must not carry credentials", literBaseURLEnv)
	if s.image == "" {
		s.image = pinnedUARImage(t)
	}
	if s.containerPort == "" {
		s.containerPort = "8080"
	}
	if s.revision == "" {
		s.revision = "kubo-it"
	}
	if s.check == "" {
		s.check = "verify"
	}
	if s.trustedHost == "" {
		s.trustedHost = "kubo-host"
	}
	require.Equal(t, s.configFile == "", s.configMount == "", "%s and %s go together", uarConfigFileEnv, uarConfigMountEnv)
	for _, kv := range strings.FieldsFunc(get(uarDockerEnvEnv), func(r rune) bool { return r == '\n' || r == ',' }) {
		require.Contains(t, kv, "=", "%s entries are KEY=VALUE", uarDockerEnvEnv)
		s.dockerEnv = append(s.dockerEnv, strings.TrimSpace(kv))
	}
	return s
}

// pinnedUARImage is the image reference of the release the contract pins.
func pinnedUARImage(t *testing.T) string {
	t.Helper()
	pre, err := contract.LoadUARPreflight()
	require.NoError(t, err)
	require.NotNil(t, pre.PinnedFrom, "the contract pins no UAR release")
	return uarImageRepository + "@" + pre.PinnedFrom.Artifact
}

// uarContainer is a running UAR container.
type uarContainer struct {
	name string
	base string // http://127.0.0.1:<port>
}

// startUAR runs the image with a Docker-chosen loopback host port and waits for
// its agent card. Only the container this test created, by its generated name,
// is ever removed.
func startUAR(t *testing.T, s uarRealSetup) *uarContainer {
	t.Helper()
	var rnd [6]byte
	_, err := rand.Read(rnd[:])
	require.NoError(t, err)
	c := &uarContainer{name: "kubo-uar-it-" + hex.EncodeToString(rnd[:])}
	args := []string{"run", "-d", "--name", c.name,
		"-p", "127.0.0.1::" + s.containerPort, // a dynamic host port, loopback only
		"--add-host", "host.docker.internal:host-gateway",
		"-e", "LLM_MODEL=" + s.llmModel, "-e", "LLM_BASE_URL=" + s.llmBase,
		"-e", "LLM_API_KEY", // inherited from the client environment, never on a command line
	}
	for _, kv := range s.dockerEnv {
		args = append(args, "-e", kv)
	}
	if s.configFile != "" {
		abs, err := filepath.Abs(s.configFile)
		require.NoError(t, err)
		args = append(args, "-v", abs+":"+s.configMount+":ro")
	}
	args = append(args, s.image)
	cmd := exec.CommandContext(t.Context(), "docker", args...)
	cmd.Env = append(os.Environ(), "LLM_API_KEY="+os.Getenv(s.llmKeyEnv))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "docker run: %s", redact(string(out), os.Getenv(s.llmKeyEnv)))
	t.Cleanup(func() {
		logs, _ := exec.Command("docker", "logs", "--tail", "60", c.name).CombinedOutput()
		t.Logf("UAR container log tail:\n%s", redact(string(logs), os.Getenv(s.llmKeyEnv)))
		_ = exec.Command("docker", "rm", "-f", c.name).Run()
	})

	port, err := exec.CommandContext(t.Context(), "docker", "port", c.name, s.containerPort+"/tcp").Output()
	require.NoError(t, err, "docker port")
	m := regexp.MustCompile(`127\.0\.0\.1:(\d+)`).FindStringSubmatch(string(port))
	require.NotNil(t, m, "no loopback host port in %q", port)
	c.base = "http://127.0.0.1:" + m[1]

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if v, ok := uarCardVersion(t, c.base); ok {
			t.Logf("UAR %s is up at %s", v, c.base)
			require.Equal(t, "1.1.0", v, "the pinned release")
			return c
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("UAR did not serve its agent card at %s within 3 minutes", c.base)
	return nil
}

func redact(s string, secrets ...string) string {
	for _, sec := range secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "<redacted>")
		}
	}
	return s
}

func uarCardVersion(t *testing.T, base string) (string, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/.well-known/agent-card.json", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var card struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(resp.Body).Decode(&card) != nil || card.Version == "" {
		return "", false
	}
	return card.Version, true
}

// uarJSON performs a JSON request to UAR with a bearer token and returns the
// status and body.
func uarJSON(t *testing.T, method, rawURL, token string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

const (
	uarTestChatToken  = "cli-uar-chat-token-0123456789"
	uarTestAdminToken = "cli-uar-admin-token-012345678"
	uarTestMCPToken   = "cli-uar-principal-token-01234567"
)

// uarMCPConfig writes the --mcp-config of the run: the chat and admin
// principals, the uar principal the agent calls back as, and llm.kind uar.
func uarMCPConfig(t *testing.T, s uarRealSetup, uarBase, skillsDir string) string {
	t.Helper()
	llmHost := ""
	if u, err := url.Parse(s.llmBase); err == nil {
		llmHost = u.Host
	}
	ack := []string{"local"}
	if llmHost != "" && !strings.HasPrefix(llmHost, "127.0.0.1") && !strings.HasPrefix(llmHost, "localhost") && !strings.HasPrefix(llmHost, "[::1]") {
		ack = []string{llmHost}
	}
	uar := map[string]any{
		"agent_id": uarTestAgentID, "mcp_name": uarTestMCPName, "mcp_principal": "uar",
		"mcp_credential_revision": s.revision, "skills_dir": skillsDir,
	}
	if s.provider != "" {
		uar["provider"] = s.provider
	}
	cfg := map[string]any{
		"agent": map[string]any{"agui": true, "origin_allowlist": []string{aguiOriginWeb}},
		"principals": []any{
			map[string]any{"name": "chat", "kind": "ui", "credential": map[string]any{"env": "CLI_UAR_CHAT_TOKEN"}, "egress_ack": []string{"local"}},
			map[string]any{"name": "admin", "kind": "admin", "credential": map[string]any{"env": "CLI_UAR_ADMIN_TOKEN"}},
			map[string]any{
				"name": "uar", "kind": "uar", "credential": map[string]any{"env": "CLI_UAR_MCP_TOKEN"},
				"write_groups": []string{"pin"}, "egress_ack": ack, "check": s.check,
				"uar": map[string]any{
					"card_url":         uarBase + "/.well-known/agent-card.json",
					"admin_credential": map[string]any{"env": s.adminEnv},
				},
			},
		},
		"llm": map[string]any{
			"kind": "uar", "base_url": uarBase, "model": s.llmModel,
			"api_key": map[string]any{"env": s.tokenEnv}, "uar": uar,
		},
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-uar.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

var uarListenPort = regexp.MustCompile(`serving MCP over HTTP on http://[^:\s]+:(\d+)`)

// startKuboMCP starts `ipfs mcp --http=0.0.0.0:0 --agui` (the OS chooses the
// port; the UAR container must reach it) and returns the process and its port.
func startKuboMCP(t *testing.T, node *harness.Node, s uarRealSetup, cfgPath string) (*mcpHTTPProc, string) {
	t.Helper()
	cmd := exec.Command(node.IPFSBin, "mcp", "--http=0.0.0.0:0", "--agui", "--mcp-config", cfgPath, "--settings-dir", t.TempDir())
	cmd.Env = append(mcpEnv(node.Dir),
		"CLI_UAR_CHAT_TOKEN="+uarTestChatToken, "CLI_UAR_ADMIN_TOKEN="+uarTestAdminToken, "CLI_UAR_MCP_TOKEN="+uarTestMCPToken)
	cmd.Dir = node.Dir
	p := &mcpHTTPProc{t: t, cmd: cmd, stderr: &syncBuffer{}}
	cmd.Stderr = p.stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(mcpTimeout)
	for time.Now().Before(deadline) {
		if m := uarListenPort.FindStringSubmatch(p.stderr.String()); m != nil {
			p.base = "http://127.0.0.1:" + m[1]
			return p, m[1]
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ipfs mcp --http did not report its address; stderr: %s", redact(p.stderr.String(), uarTestChatToken, uarTestAdminToken, uarTestMCPToken))
	return nil, ""
}

// kuboSettingsPrefix reads the fixed safety prefix through the settings API.
func kuboSettings(t *testing.T, p *mcpHTTPProc) (etag, prefix string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, p.base+"/agent/settings", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+uarTestAdminToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var doc struct {
		Backend      struct{ Kind string } `json:"backend"`
		SystemPrompt struct {
			SafetyPrefix string `json:"safety_prefix"`
		} `json:"system_prompt"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&doc))
	require.Equal(t, "uar", doc.Backend.Kind, "the settings read reports the UAR backend")
	return resp.Header.Get("ETag"), doc.SystemPrompt.SafetyPrefix
}

func setKuboPrompt(t *testing.T, p *mcpHTTPProc, etag, prompt string) {
	t.Helper()
	b, err := json.Marshal(map[string]any{"system_prompt": prompt})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, p.base+"/agent/settings", bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+uarTestAdminToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", etag)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
}

// registerKuboDestination registers the kubo MCP server on UAR as an
// administrator, restricted to the kubo agent (contract, "Trusted-host MCP
// setup"). UAR reaches this machine at hostIP:kuboPort.
func registerKuboDestination(t *testing.T, s uarRealSetup, uarBase, kuboPort string) {
	t.Helper()
	body := map[string]any{
		"name": uarTestMCPName, "enabled": true, "transport": "remote_http",
		"url": "http://" + s.hostIP + ":" + kuboPort + "/mcp", "headers": map[string]any{},
		"grant_policy": map[string]any{
			"destination_id": uarTestMCPName, "trusted_hosts": []string{s.trustedHost}, "required_scopes": []string{"ipfs"},
			"allow_private_http": true, "allowed_agent_ids": []string{uarTestAgentID},
		},
	}
	status, resp := uarJSON(t, http.MethodPut, uarBase+"/api/uar/mcp/servers/"+uarTestMCPName, os.Getenv(s.adminEnv), body)
	require.Less(t, status, 300, "registering the kubo destination: HTTP %d %s", status, resp)
}

// skillDirs lists the skill directory names under mcp/skills.
func skillDirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(dir, e.Name(), "SKILL.md")); e.IsDir() && err == nil {
			names = append(names, e.Name())
		}
	}
	require.NotEmpty(t, names)
	return names
}

// stringsIn collects every string value of a decoded JSON document.
func stringsIn(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, e := range x {
			stringsIn(e, out)
		}
	case map[string]any:
		for _, e := range x {
			stringsIn(e, out)
		}
	}
}

// uarInterruptReason reads the UAR reason the kubo-shaped interrupt carries.
func uarInterruptReason(in map[string]any) string {
	meta, _ := in["metadata"].(map[string]any)
	uar, _ := meta["uar"].(map[string]any)
	reason, _ := uar["reason"].(string)
	return reason
}

func TestMCPUARBackend(t *testing.T) {
	s := uarRealSetupOrSkip(t)
	skillsDir, err := filepath.Abs("../../mcp/skills")
	require.NoError(t, err)
	wantSkills := skillDirs(t, skillsDir)

	node := agentLLMNode(t)
	uar := startUAR(t, s)
	cfg := uarMCPConfig(t, s, uar.base, skillsDir)
	p, kuboPort := startKuboMCP(t, node, s, cfg)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("ipfs mcp stderr:\n%s", redact(p.stderr.String(), uarTestChatToken, uarTestAdminToken, uarTestMCPToken, os.Getenv(s.tokenEnv)))
		}
	})

	// kubo's configured prompt: the fixed prefix plus a custom part the test sets
	// through the settings API, which the provisioning on the first run reads.
	etag, prefix := kuboSettings(t, p)
	setKuboPrompt(t, p, etag, uarTestPrompt)
	wantPrompt := prefix + "\n\n" + uarTestPrompt

	// One chat run completes; it also provisions the agent.
	chat := aguiPost(t, p, uarTestChatToken, aguiInput("uar-chat", "run-chat", "Reply with one short sentence about IPFS."), nil)
	require.Equal(t, http.StatusOK, chat.Status, chat.Body)
	last := chat.Events[len(chat.Events)-1]
	require.Equal(t, "RUN_FINISHED", last["type"], "events: %v", chat.types())
	require.Equal(t, "success", last["outcome"].(map[string]any)["type"], "events: %v", chat.types())
	assert.Contains(t, chat.types(), "TEXT_MESSAGE_CONTENT")

	// GET /api/agents/{id} shows kubo's prompt, the MCP binding and the skills.
	token := os.Getenv(s.tokenEnv)
	status, body := uarJSON(t, http.MethodGet, uar.base+"/api/agents/"+uarTestAgentID, token, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var agent map[string]any
	require.NoError(t, json.Unmarshal(body, &agent))
	var all []string
	stringsIn(agent, &all)
	assert.Contains(t, all, wantPrompt, "the agent's system prompt equals kubo's, safety prefix included")
	ext, _ := agent["extensions"].(map[string]any)
	var bound []string
	stringsIn(ext, &bound)
	assert.Contains(t, bound, uarTestMCPName, "the agent has an MCP binding that names the kubo server")
	status, skillBody := uarJSON(t, http.MethodGet, uar.base+"/api/uar/agents/"+uarTestAgentID+"/skills", token, nil)
	require.Equal(t, http.StatusOK, status, string(skillBody))
	for _, name := range wantSkills {
		assert.Contains(t, string(skillBody)+string(body), name, "skill %s is bound to the agent", name)
	}

	// The kubo destination is registered after the agent exists (its policy names
	// the persisted agent id).
	registerKuboDestination(t, s, uar.base, kuboPort)

	// A write: the agent asks for approval, the person approves, the run ends.
	cid := node.IPFSAddStr("uar backend write", "--pin=false", "--cid-version=1")
	require.False(t, isPinned(node, cid))
	prompt := fmt.Sprintf("Pin /ipfs/%s on this node with the pin_add tool, then say done.", cid)

	var in map[string]any
	const attempts = 2
	for i := 1; i <= attempts; i++ {
		r := aguiPost(t, p, uarTestChatToken, aguiInput(fmt.Sprintf("uar-approve-%d", i), fmt.Sprintf("run-ap-%d", i), prompt), nil)
		if r.Status == http.StatusOK && len(r.Events) > 0 {
			if out, _ := r.Events[len(r.Events)-1]["outcome"].(map[string]any); out["type"] == "interrupt" {
				in = out["interrupts"].([]any)[0].(map[string]any)
				thread := fmt.Sprintf("uar-approve-%d", i)
				require.Equal(t, "tool_call", in["reason"], "the interrupt has the kubo shape")
				assert.Equal(t, "tool_approval", uarInterruptReason(in), "UAR asked for a tool approval")
				res := aguiPost(t, p, uarTestChatToken, aguiResume(thread, fmt.Sprintf("run-ap-%d-b", i), in["id"].(string), "resolved", map[string]any{"approved": true}), nil)
				require.Equal(t, http.StatusOK, res.Status, res.Body)
				// A further approval (a second tool call) is answered the same way.
				for hops := 0; hops < 3; hops++ {
					lo, _ := res.Events[len(res.Events)-1]["outcome"].(map[string]any)
					if lo["type"] != "interrupt" {
						break
					}
					next := lo["interrupts"].([]any)[0].(map[string]any)
					res = aguiPost(t, p, uarTestChatToken, aguiResume(thread, fmt.Sprintf("run-ap-%d-c%d", i, hops), next["id"].(string), "resolved", map[string]any{"approved": true}), nil)
					require.Equal(t, http.StatusOK, res.Status, res.Body)
				}
				fin := res.Events[len(res.Events)-1]
				require.Equal(t, "RUN_FINISHED", fin["type"], "events: %v", res.types())
				require.Equal(t, "success", fin["outcome"].(map[string]any)["type"], "events: %v", res.types())
				t.Logf("approved write finished; pinned afterwards: %v (kubo's own confirmation decides)", isPinned(node, cid))
				break
			}
		}
		t.Logf("attempt %d/%d: the model did not ask for a tool approval (events %v)", i, attempts, r.types())
	}
	require.NotNil(t, in, "no write run ended in a tool_approval interrupt in %d attempts with %s", attempts, s.llmModel)

	// A denied write ends the run with nothing executed.
	denyCID := node.IPFSAddStr("uar backend denied write", "--pin=false", "--cid-version=1")
	denyPrompt := fmt.Sprintf("Pin /ipfs/%s on this node with the pin_add tool, then say done.", denyCID)
	var denied bool
	for i := 1; i <= attempts && !denied; i++ {
		thread := fmt.Sprintf("uar-deny-%d", i)
		r := aguiPost(t, p, uarTestChatToken, aguiInput(thread, fmt.Sprintf("run-dn-%d", i), denyPrompt), nil)
		out, _ := r.Events[len(r.Events)-1]["outcome"].(map[string]any)
		if r.Status != http.StatusOK || out["type"] != "interrupt" {
			t.Logf("deny attempt %d/%d: no interrupt (events %v)", i, attempts, r.types())
			continue
		}
		intr := out["interrupts"].([]any)[0].(map[string]any)
		res := aguiPost(t, p, uarTestChatToken, aguiResume(thread, fmt.Sprintf("run-dn-%d-b", i), intr["id"].(string), "resolved", map[string]any{"approved": false}), nil)
		require.Equal(t, http.StatusOK, res.Status, res.Body)
		fin := res.Events[len(res.Events)-1]
		assert.Equal(t, "RUN_FINISHED", fin["type"], "events: %v", res.types())
		assert.False(t, isPinned(node, denyCID), "a denied write executes nothing")
		denied = true
	}
	require.True(t, denied, "no write run reached an interrupt to deny in %d attempts", attempts)

	assert.NotContains(t, p.stderr.String(), os.Getenv(s.tokenEnv), "kubo logs no UAR credential")
}
