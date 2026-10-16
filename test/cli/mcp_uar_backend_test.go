//go:build !noagent

package cli

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
// It starts UAR the way UAR's own release acceptance does
// (scripts/release/uar_1_1_acceptance.py at the tag; mcp/uar/v1.1.0-image-config.md):
//
//   - a Docker network with the pinned SurrealDB companion (the acceptance's
//     image and digest, network alias "surreal"), UAR persisting to it;
//   - the UAR image by the digest pinned in mcp/contract/agentic/uar-preflight.json,
//     run as `universal-agent-runtime --config /opt/uar/release-config.yaml`
//     with a generated configuration mounted read-only, HTTP port 1906 published
//     on 127.0.0.1:<random>, --add-host host.docker.internal:host-gateway, and
//     LLM_BASE_URL, LLM_MODEL, LLM_API_KEY and LLM_PROTOCOL=chat as the model;
//   - HS256 JWTs minted from a random per-run secret with the acceptance's claims,
//     and the configuration lists the kubo host as a trusted host principal.
//
// Then it starts `ipfs mcp --http=0.0.0.0:0 --agui` (the OS chooses the port)
// against an isolated harness daemon, and checks:
//
//   - GET /api/agents/{id} on UAR shows kubo's configured system prompt (fixed
//     safety prefix included), the kubo MCP tools as the agent's tool allowlist,
//     and skill bindings equal to the directory names under mcp/skills;
//   - one chat run through /agui completes;
//   - one write run ends in a tool_approval interrupt (translated to the kubo
//     shape) that resumes after approval and finishes;
//   - a denied write ends the run with nothing executed (the CID stays unpinned).
//
// Registering the kubo MCP server on UAR is an operator step (ADR-0010
// amendment): the test performs it, as the acceptance does, with the trusted-host
// token, before the first run (a run that names an unregistered destination is
// refused with 422). The destination's grant_policy may name the agent before it
// exists.
//
// A write the agent runs after approval still needs kubo's own confirmation: the
// uar principal never answers by elicitation, so with no resolver the tool
// reports the confirmation as unavailable and the run goes on. The test asserts
// the approval path and the denial; it does not require the pin to land.
//
// It never runs by default. It is skipped, with the list of what is missing,
// unless the operator opts in with IPFS_MCP_TEST_UAR=1 and the three liter-llm
// variables. It never pulls an image (docker run --pull never): both images must
// already be present locally. Nothing is defaulted that names a service,
// credential or model.
//
// Run it (from the repository root, after `make build`, with the UAR and
// SurrealDB images already pulled):
//
//	export PATH="$PWD/cmd/ipfs:$PATH"
//	export IPFS_MCP_TEST_UAR=1
//	# liter-llm, the operator's model provider for UAR (same names as the R7 test):
//	export IPFS_MCP_TEST_LLM_BASE_URL=https://liter.example/v1
//	export IPFS_MCP_TEST_LLM_API_KEY_ENV=LITER_API_KEY   # NAME of the variable that holds the key
//	export IPFS_MCP_TEST_LLM_MODEL=<provider>/<model>
//	go test ./test/cli/... -run TestMCPUARBackend -v
//
// Optional:
//
//	IPFS_MCP_TEST_UAR_IMAGE            image reference; default is the pinned digest of ghcr.io/prometheus-ags/universal-agent-runtime
//	IPFS_MCP_TEST_UAR_PROVIDER         UAR provider id for llm.uar.provider; default is the part of the model before the first slash
//	IPFS_MCP_TEST_UAR_KUBO_HOST_IP     the literal IP at which the UAR container reaches this machine; default is what
//	                                   host.docker.internal resolves to inside the container (UAR refuses plaintext hostnames)
//	IPFS_MCP_TEST_UAR_CREDENTIAL_REVISION  run-grant credential_revision (default "kubo-it")
//	IPFS_MCP_TEST_UAR_CHECK            "assert" (default) or "verify" for the uar principal's check-uar mode ("verify" reads
//	                                   UAR's settings API, which needs the X-UAR-Admin-Key header that check-uar does not send)
//
// Which provider/model pair UAR accepts for the agent is not stated by the release notes: the acceptance ran with
// LLM_MODEL=fixture/gpt-4o-mini and an agent whose default selection was provider "fixture", model "gpt-4o-mini".
// The same split is assumed here.
const (
	uarRealOptInEnv   = "IPFS_MCP_TEST_UAR"
	uarHostIPEnv      = "IPFS_MCP_TEST_UAR_KUBO_HOST_IP"
	uarImageEnv       = "IPFS_MCP_TEST_UAR_IMAGE"
	uarProviderEnv    = "IPFS_MCP_TEST_UAR_PROVIDER"
	uarRevisionEnv    = "IPFS_MCP_TEST_UAR_CREDENTIAL_REVISION"
	uarCheckEnv       = "IPFS_MCP_TEST_UAR_CHECK"
	uarImageRepo      = "ghcr.io/prometheus-ags/universal-agent-runtime"
	uarTestAgentID    = "kubo-ipfs"
	uarTestMCPName    = "kubo-ipfs"
	uarTestPrompt     = "You are the IPFS agent of a test node. Be brief."
	uarHTTPPort       = "1906"
	uarConfigMount    = "/opt/uar/release-config.yaml"
	uarKuboKeyEnvName = "CLI_UAR_KEY"

	// uarSurrealImage is the SurrealDB companion of UAR's release acceptance,
	// scripts/release/uar_1_1_acceptance.py:42.
	uarSurrealImage = "surrealdb/surrealdb:v3.3.0@sha256:681c6c22c287421b5c7d99e0fde79b6e0d32c36c1ddeaab2762a1661cb04cd20"

	// Token claims, the acceptance's (acc. 43-56): the issuer, audience and tenant
	// of the generated configuration, and the trusted host's subject and id.
	uarTestIssuer   = "uar-release-acceptance"
	uarTestAudience = "uar-release-candidate"
	uarTestTenant   = "release-tenant"
	uarTestHostSub  = "release-host"
	uarTestHostID   = "release-host"
	uarSurrealUser  = "root"
)

// uarRealSetup is the validated environment of the real-service run.
type uarRealSetup struct {
	llmBase, llmKeyEnv, llmModel string
	hostIP, image                string
	provider, revision, check    string
}

// uarRealSetupOrSkip returns the setup, or skips the test naming what is
// missing. A setting that is present but unusable fails the test: the operator
// opted in, and a silent skip would hide a broken gate run.
func uarRealSetupOrSkip(t *testing.T) uarRealSetup {
	t.Helper()
	get := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	if get(uarRealOptInEnv) != "1" {
		t.Skipf("UAR v1.1.0 real-service test (agent-backend-uar 1.5) not enabled: set %s=1 and the liter-llm variables in this test's header comment; the UAR backend is unverified against the published image until it runs", uarRealOptInEnv)
	}
	s := uarRealSetup{
		llmBase: get(literBaseURLEnv), llmKeyEnv: get(literKeyEnvName), llmModel: get(literModelEnvVar),
		hostIP: get(uarHostIPEnv), image: get(uarImageEnv),
		provider: get(uarProviderEnv), revision: get(uarRevisionEnv), check: get(uarCheckEnv),
	}
	var missing []string
	for name, v := range map[string]string{literBaseURLEnv: s.llmBase, literKeyEnvName: s.llmKeyEnv, literModelEnvVar: s.llmModel} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("UAR real-service test: %s is set but %s are not; see the header comment of this test", uarRealOptInEnv, strings.Join(missing, ", "))
	}
	require.NotEmpty(t, os.Getenv(s.llmKeyEnv), "the environment variable %s, named by the operator, is empty", s.llmKeyEnv)
	_, err := exec.LookPath("docker")
	require.NoError(t, err, "the real-service test starts the UAR image with docker")
	u, err := url.Parse(s.llmBase)
	require.NoError(t, err, "%s must be a URL", literBaseURLEnv)
	require.Empty(t, u.User, "%s must not carry credentials", literBaseURLEnv)
	if s.image == "" {
		s.image = pinnedUARImage(t)
	}
	if s.revision == "" {
		s.revision = "kubo-it"
	}
	if s.check == "" {
		s.check = "assert"
	}
	if s.provider == "" {
		prov, _, ok := strings.Cut(s.llmModel, "/")
		require.True(t, ok && prov != "", "%s is %q: UAR's agent needs a provider, so give the model as provider/model or set %s", literModelEnvVar, s.llmModel, uarProviderEnv)
	}
	return s
}

// pinnedUARImage is the image reference of the release the contract pins.
func pinnedUARImage(t *testing.T) string {
	t.Helper()
	pre, err := contract.LoadUARPreflight()
	require.NoError(t, err)
	require.NotNil(t, pre.PinnedFrom, "the contract pins no UAR release")
	return uarImageRepo + "@" + pre.PinnedFrom.Artifact
}

// uarSecrets are the per-run secrets of the UAR stack, all random.
type uarSecrets struct {
	jwtSecret, adminKey, surrealPass string
}

func newUARSecrets(t *testing.T) uarSecrets {
	t.Helper()
	rnd := func() string {
		var b [24]byte
		_, err := rand.Read(b[:])
		require.NoError(t, err)
		return hex.EncodeToString(b[:])
	}
	return uarSecrets{jwtSecret: rnd(), adminKey: rnd(), surrealPass: rnd()}
}

func b64url(raw []byte) string { return base64.RawURLEncoding.EncodeToString(raw) }

// mintUARJWT mints an HS256 token the way the acceptance does (acc. 85-104):
// uar_credential_kind "issuer", the tenant, issuer and audience, nbf and exp,
// and uar_instance_id for a trusted host.
func mintUARJWT(t *testing.T, secret, subject string, roles []string, instanceID string) string {
	t.Helper()
	now := time.Now().Unix()
	claims := map[string]any{
		"sub": subject, "uar_credential_kind": "issuer", "roles": roles, "tenant_id": uarTestTenant,
		"iss": uarTestIssuer, "aud": uarTestAudience, "nbf": now - 5, "exp": now + 3600,
	}
	if instanceID != "" {
		claims["uar_instance_id"] = instanceID
	}
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	require.NoError(t, err)
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	signing := b64url(header) + "." + b64url(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + b64url(mac.Sum(nil))
}

// uarReleaseYAML is the acceptance's configuration template (acc. 374-416 yaml_config),
// with the shared MCP transports off.
func uarReleaseYAML(sec uarSecrets) string {
	return fmt.Sprintf(`server:
  host: "0.0.0.0"
  port: 1906
  grpc_port: 50051
  cors_allowed_origins: []
  mcp_shared_transports_enabled: false
security:
  jwt_required: true
  jwt_secret: "%[1]s"
  jwt_algorithm: HS256
  jwt_issuer: "%[2]s"
  jwt_audience: "%[3]s"
  jwt_validate_nbf: true
  settings_admin_key: "%[4]s"
  skill_required_roles: {}
  trusted_host_principals:
    - issuer: "%[2]s"
      subject: "%[5]s"
      tenant_id: "%[6]s"
      host_id: "%[7]s"
persistence:
  provider: surreal
  database_url: "http://surreal:8000"
  surreal_user: "%[8]s"
  surreal_pass: "%[9]s"
  surreal_ns: "release"
  surreal_db: "kubo"
  vector_dimension: 384
  external_cache_enabled: false
memory:
  enabled: false
providers: []
native_tools:
  file_tools_enabled: false
  web_fetch_enabled: false
  terminal_exec_enabled: false
  session_search_enabled: false
`, sec.jwtSecret, uarTestIssuer, uarTestAudience, sec.adminKey, uarTestHostSub, uarTestTenant, uarTestHostID, uarSurrealUser, sec.surrealPass)
}

// uarStack is the running UAR deployment of the test.
type uarStack struct {
	network, surreal, uar string
	base                  string // http://127.0.0.1:<port>
	hostToken             string // the trusted host's JWT, which kubo presents
	hostIP                string
	secrets               uarSecrets
}

func randomName(t *testing.T, prefix string) string {
	t.Helper()
	var rnd [6]byte
	_, err := rand.Read(rnd[:])
	require.NoError(t, err)
	return prefix + hex.EncodeToString(rnd[:])
}

// docker runs the docker client and returns its trimmed stdout. secrets are
// removed from a failure message.
func docker(t *testing.T, secrets []string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "docker", args...)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(t, err, "docker %s: %s", args[0], redact(stderr.String(), secrets...))
	return strings.TrimSpace(string(out))
}

// startUAR creates the network, starts the SurrealDB companion and UAR on it, and
// waits for UAR's agent card. Only the containers and the network this test
// created, by their generated names, are ever removed. No image is pulled: a
// missing image fails the run.
func startUAR(t *testing.T, s uarRealSetup) *uarStack {
	t.Helper()
	sec := newUARSecrets(t)
	st := &uarStack{
		network: randomName(t, "kubo-uar-net-"), surreal: randomName(t, "kubo-uar-surreal-"), uar: randomName(t, "kubo-uar-it-"),
		secrets: sec,
	}
	st.hostToken = mintUARJWT(t, sec.jwtSecret, uarTestHostSub, []string{"admin"}, uarTestHostID)
	redactions := []string{sec.jwtSecret, sec.adminKey, sec.surrealPass, st.hostToken, os.Getenv(s.llmKeyEnv)}
	dock := func(args ...string) string { return docker(t, redactions, args...) }

	dock("network", "create", st.network)
	t.Cleanup(func() { _ = exec.Command("docker", "network", "rm", st.network).Run() })

	dock("run", "-d", "--pull", "never", "--name", st.surreal, "--network", st.network, "--network-alias", "surreal",
		"-p", "127.0.0.1::8000", uarSurrealImage, "start", "--log", "warn",
		"--user", uarSurrealUser, "--pass", sec.surrealPass, "--bind", "0.0.0.0:8000", "rocksdb:/tmp/release.db")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", st.surreal).Run() })
	surrealAddr := dock("port", st.surreal, "8000/tcp")
	surrealPort := regexp.MustCompile(`127\.0\.0\.1:(\d+)`).FindStringSubmatch(surrealAddr)
	require.NotNil(t, surrealPort, "no loopback port in %q", surrealAddr)
	waitFor(t, 3*time.Minute, "SurrealDB health", func() bool {
		return httpStatus(t, "http://127.0.0.1:"+surrealPort[1]+"/health") == http.StatusOK
	})

	cfgPath := filepath.Join(t.TempDir(), "release-config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(uarReleaseYAML(sec)), 0o644))
	args := []string{"run", "-d", "--pull", "never", "--name", st.uar, "--network", st.network,
		"--add-host", "host.docker.internal:host-gateway",
		"-v", cfgPath + ":" + uarConfigMount + ":ro",
		"-p", "127.0.0.1::" + uarHTTPPort, // a dynamic host port, loopback only
		"-e", "UAR_PERSISTENCE__PROVIDER=surreal",
		"-e", "UAR_PERSISTENCE__DATABASE_URL=http://surreal:8000",
		"-e", "UAR_REMOTE_SURREAL_DURABILITY_ATTESTED=1",
		"-e", "LLM_MODEL=" + s.llmModel, "-e", "LLM_BASE_URL=" + s.llmBase, "-e", "LLM_PROTOCOL=chat",
		"-e", "LLM_API_KEY", // inherited from the client environment, never on a command line
		s.image, "universal-agent-runtime", "--config", uarConfigMount}
	cmd := exec.CommandContext(t.Context(), "docker", args...)
	cmd.Env = append(os.Environ(), "LLM_API_KEY="+os.Getenv(s.llmKeyEnv))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "docker run: %s", redact(string(out), redactions...))
	t.Cleanup(func() {
		logs, _ := exec.Command("docker", "logs", "--tail", "400", st.uar).CombinedOutput()
		t.Logf("UAR container log tail (warnings, errors and request lines; startup noise dropped):\n%s",
			uarLogTail(redact(string(logs), redactions...), 60))
		_ = exec.Command("docker", "rm", "-f", st.uar).Run()
	})

	port := dock("port", st.uar, uarHTTPPort+"/tcp")
	m := regexp.MustCompile(`127\.0\.0\.1:(\d+)`).FindStringSubmatch(port)
	require.NotNil(t, m, "no loopback host port in %q", port)
	st.base = "http://127.0.0.1:" + m[1]
	waitFor(t, 3*time.Minute, "UAR agent card", func() bool {
		v, ok := uarCardVersion(t, st.base)
		if ok {
			t.Logf("UAR %s is up at %s", v, st.base)
			require.Equal(t, "1.1.0", v, "the pinned release")
		}
		return ok
	})

	st.hostIP = s.hostIP
	if st.hostIP == "" {
		st.hostIP = dock("exec", st.uar, "python3", "-c", "import socket; print(socket.gethostbyname('host.docker.internal'))")
		// Docker Desktop resolves host.docker.internal to an address outside the
		// private ranges (0.250.250.254 was seen), which UAR refuses for a
		// private-HTTP destination (422 run_mcp_destination_invalid). Fall back to
		// a private address of this machine, which ipfs mcp (bound to 0.0.0.0)
		// answers on.
		if ip := net.ParseIP(st.hostIP); ip == nil || !ip.IsPrivate() {
			if own := privateHostIP(); own != "" {
				t.Logf("host.docker.internal resolved to %s, not a private address; using %s (set %s to choose)", st.hostIP, own, uarHostIPEnv)
				st.hostIP = own
			}
		}
	}
	require.Regexp(t, `^\d+\.\d+\.\d+\.\d+$`, st.hostIP, "the address at which UAR reaches this machine must be a literal IP")
	return st
}

// uarLogTail keeps the last n lines of a UAR container log that are not the
// per-table "waiting for first write" startup noise, so a failed request's own
// line is not pushed out of the tail by it.
func uarLogTail(logs string, n int) string {
	var keep []string
	for _, line := range strings.Split(logs, "\n") {
		if line == "" || strings.Contains(line, "table not yet created") {
			continue
		}
		keep = append(keep, line)
	}
	if len(keep) > n {
		keep = keep[len(keep)-n:]
	}
	return strings.Join(keep, "\n")
}

// privateHostIP is the first private IPv4 address of an up, non-loopback
// interface of this machine, or "".
func privateHostIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.To4() == nil || !ipn.IP.IsPrivate() {
			continue
		}
		return ipn.IP.String()
	}
	return ""
}

func waitFor(t *testing.T, limit time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func httpStatus(t *testing.T, rawURL string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
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

// uarJSON performs a JSON request to UAR as the trusted host and returns the
// status and body. The admin key goes with every request, as in the acceptance.
func uarJSON(t *testing.T, st *uarStack, method, path string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, st.base+path, rd)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+st.hostToken)
	req.Header.Set("X-UAR-Admin-Key", st.secrets.adminKey)
	req.Header.Set("Accept", "application/json")
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
				"uar": map[string]any{"card_url": uarBase + "/.well-known/agent-card.json"},
			},
		},
		"llm": map[string]any{
			"kind": "uar", "base_url": uarBase, "model": s.llmModel,
			"api_key": map[string]any{"env": uarKuboKeyEnvName}, "uar": uar,
		},
	}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "mcp-uar.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

var uarListenPort = regexp.MustCompile(`serving MCP over HTTP on http://\S*:(\d+)/`)

// startKuboMCP starts `ipfs mcp --http=0.0.0.0:0 --agui` (the OS chooses the
// port; the UAR container must reach it) and returns the process and its port.
func startKuboMCP(t *testing.T, node *harness.Node, st *uarStack, cfgPath string) (*mcpHTTPProc, string) {
	t.Helper()
	cmd := exec.Command(node.IPFSBin, "mcp", "--http=0.0.0.0:0", "--agui", "--mcp-config", cfgPath, "--settings-dir", t.TempDir())
	cmd.Env = append(mcpEnv(node.Dir),
		"CLI_UAR_CHAT_TOKEN="+uarTestChatToken, "CLI_UAR_ADMIN_TOKEN="+uarTestAdminToken, "CLI_UAR_MCP_TOKEN="+uarTestMCPToken,
		uarKuboKeyEnvName+"="+st.hostToken)
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
	t.Fatalf("ipfs mcp --http did not report its address; stderr: %s", redact(p.stderr.String(), uarTestChatToken, uarTestAdminToken, uarTestMCPToken, st.hostToken))
	return nil, ""
}

// registerKuboDestination registers the kubo MCP server on UAR as the trusted
// host, restricted to the kubo agent: the operator step of the contract
// ("Trusted-host MCP setup"; acc. 578-584). UAR reaches this machine at
// hostIP:kuboPort. required_scopes is the run grant's default scope, "ipfs".
func registerKuboDestination(t *testing.T, st *uarStack, kuboPort string) {
	t.Helper()
	body := map[string]any{
		"name": uarTestMCPName, "enabled": true, "transport": "remote_http",
		"url": "http://" + st.hostIP + ":" + kuboPort + "/mcp", "env": map[string]any{}, "headers": map[string]any{}, "sandboxed": false,
		"grant_policy": map[string]any{
			"destination_id": uarTestMCPName, "trusted_hosts": []string{uarTestHostID}, "required_scopes": []string{"ipfs"},
			"allow_private_http": true, "allowed_agent_ids": []string{uarTestAgentID},
		},
	}
	status, resp := uarJSON(t, st, http.MethodPut, "/api/uar/mcp/servers/"+uarTestMCPName, body)
	require.Equal(t, http.StatusOK, status, "registering the kubo destination: %s", resp)
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
	st := startUAR(t, s)
	cfg := uarMCPConfig(t, s, st.base, skillsDir)
	p, kuboPort := startKuboMCP(t, node, st, cfg)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("ipfs mcp stderr:\n%s", redact(p.stderr.String(), uarTestChatToken, uarTestAdminToken, uarTestMCPToken, st.hostToken))
		}
	})

	// The kubo destination is registered before the first run: a run whose grant
	// names an unregistered destination is refused with HTTP 422
	// run_mcp_destination_unregistered (UAR src/uar/runtime/turn/host/mcp.rs). The
	// policy may name an agent id that does not exist yet (verified against the
	// v1.1.0 image: PUT answers 200).
	registerKuboDestination(t, st, kuboPort)

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

	// GET /api/agents/{id} shows kubo's prompt, the kubo tools and the skills.
	status, body := uarJSON(t, st, http.MethodGet, "/api/agents/"+uarTestAgentID, nil)
	require.Equal(t, http.StatusOK, status, string(body))
	var agent map[string]any
	require.NoError(t, json.Unmarshal(body, &agent))
	var all []string
	stringsIn(agent, &all)
	assert.Contains(t, all, wantPrompt, "the agent's system prompt equals kubo's, safety prefix included")
	assert.Contains(t, all, uarTestMCPName+"__pin_add", "the agent may use the kubo tools, by their UAR names")
	status, skillBody := uarJSON(t, st, http.MethodGet, "/api/uar/agents/"+uarTestAgentID+"/skills", nil)
	require.Equal(t, http.StatusOK, status, string(skillBody))
	var bound []string
	require.NoError(t, json.Unmarshal(skillBody, &bound), "the bindings reply is a list of skill ids")
	assert.ElementsMatch(t, wantSkills, bound, "skill bindings equal the directory names under mcp/skills")

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
				assert.True(t, strings.HasPrefix(in["message"].(string), "UAR approval, not a node confirmation: "), "the card says whose approval it is")
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

	assert.NotContains(t, p.stderr.String(), st.hostToken, "kubo logs no UAR credential")
	assert.NotContains(t, p.stderr.String(), uarTestMCPToken, "kubo logs no grant credential")
}
