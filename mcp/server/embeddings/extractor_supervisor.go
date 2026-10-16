package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Names inside the operator's extractor cache_dir.
const (
	// xbergStateFile records the child process so a later start can reap it.
	xbergStateFile = "xberg-supervisor.json"
	// xbergConfigFile is the (empty) config file passed to xberg serve. Naming
	// one stops xberg from discovering an xberg.toml in the working directory
	// or its parents, and puts a path in the command line that identifies the
	// process on reaping.
	xbergConfigFile = "xberg-serve.json"
	// xbergHFDir is the Hugging Face home inside cache_dir.
	xbergHFDir = "huggingface"
)

// ExtractorSupervisor owns the xberg process of embedded mode. It runs
// `xberg serve` bound to 127.0.0.1 on an ephemeral port with a minimal
// environment, and does not outlive the ipfs mcp process: Close stops it, a
// watchdog stops it if ipfs mcp is killed, and the next start reaps it if both
// failed. It follows the SurrealDB Supervisor.
type ExtractorSupervisor struct {
	cfg      ExtractorEmbedded
	log      *slog.Logger
	addr     string
	confPath string

	// want is the version `xberg --version` reported; /health must report the
	// same one before the port is trusted.
	want version
	// proxy is the authenticated loopback path ipfs mcp uses, nil until
	// startProxy runs (see extractor_proxy.go).
	proxy *extractorProxy
	// unlock releases the cache_dir lock.
	unlock func()

	cmd      *exec.Cmd
	tail     *tailBuffer
	exited   chan struct{}
	watchdog *exec.Cmd
	closeMu  sync.Mutex
	closed   bool
}

// StartExtractorSupervisor validates the binary, reaps a process a crashed
// ipfs mcp left behind, starts xberg and waits until it is healthy. It returns
// an error, with nothing left running, otherwise.
func StartExtractorSupervisor(ctx context.Context, cfg ExtractorEmbedded, log *slog.Logger) (*ExtractorSupervisor, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := (ExtractorSettings{Mode: ModeEmbedded, Embedded: &cfg}).Validate(); err != nil {
		return nil, err
	}
	want, err := checkXbergBinary(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if !cfg.AllowModelDownloads {
		log.Info("embeddings.extractor: model downloads are off (HF_HUB_OFFLINE=1); a file that needs a model not in cache_dir fails extraction",
			"cache_dir", cfg.CacheDir)
	} else {
		log.Warn("embeddings.extractor.embedded.allow_model_downloads is on: xberg may download layout, OCR and transcription models from the Hugging Face Hub (huggingface.co)")
	}
	if err := os.MkdirAll(filepath.Join(cfg.CacheDir, xbergHFDir), 0o700); err != nil {
		return nil, fmt.Errorf("embeddings.extractor.embedded.cache_dir: %w", err)
	}
	if err := checkCacheDirOwner(cfg.CacheDir); err != nil {
		return nil, err
	}
	unlock, err := lockCacheDir(cfg.CacheDir)
	if err != nil {
		return nil, err
	}
	s := &ExtractorSupervisor{cfg: cfg, log: log, want: want, unlock: unlock, confPath: filepath.Join(cfg.CacheDir, xbergConfigFile)}
	// The lock is held, so no live ipfs mcp runs a child in this directory: a
	// recorded one is stale.
	if err := s.reapStale(); err != nil {
		unlock()
		return nil, err
	}
	if err := writeFileAtomic(s.confPath, []byte("{}\n"), 0o600); err != nil {
		unlock()
		return nil, err
	}
	var lastErr error
	for range maxStartTries {
		err := s.start(ctx)
		if err == nil {
			return s, nil
		}
		lastErr = err
		if (!errors.Is(err, errXbergEarlyExit) && !errors.Is(err, errXbergForeign)) || ctx.Err() != nil {
			break
		}
		// The port was taken between choosing and binding; choose again.
	}
	unlock()
	return nil, lastErr
}

// Endpoint is the base URL of xberg.
func (s *ExtractorSupervisor) Endpoint() string { return "http://" + s.addr }

// Addr is the host:port xberg listens on.
func (s *ExtractorSupervisor) Addr() string { return s.addr }

// PID is the process id of xberg, or 0 if it is not running.
func (s *ExtractorSupervisor) PID() int {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Done is closed when the xberg process has exited.
func (s *ExtractorSupervisor) Done() <-chan struct{} { return s.exited }

var (
	errXbergEarlyExit = errors.New("xberg exited during startup")
	// errXbergForeign: something other than the xberg this supervisor started
	// answered on its port.
	errXbergForeign = errors.New("another process answered on the xberg port")
)

// ProxyEndpoint is the base URL of the authenticated proxy in front of xberg,
// the address ipfs mcp sends documents to. Empty before startProxy.
func (s *ExtractorSupervisor) ProxyEndpoint() string {
	if s.proxy == nil {
		return ""
	}
	return s.proxy.Endpoint()
}

// ProxyToken is the bearer token the proxy requires. Empty before startProxy.
func (s *ExtractorSupervisor) ProxyToken() Secret {
	if s.proxy == nil {
		return ""
	}
	return s.proxy.Token()
}

// startProxy opens the authenticated loopback proxy in front of the running
// xberg; maxFile is the largest file it forwards.
func (s *ExtractorSupervisor) startProxy(maxFile int) error {
	p, err := startExtractorProxy(s.Endpoint(), maxFile)
	if err != nil {
		return err
	}
	s.proxy = p
	// When xberg exits after startup the proxy stops at once, so nothing is
	// forwarded to the freed port; Extract also refuses (see Extractor.Extract).
	go func() {
		<-s.exited
		p.Close()
	}()
	return nil
}

// checkXbergBinary refuses a binary that is missing, not a regular executable
// file or older than the minimum version. The path is absolute (validated) and
// is never resolved through PATH.
func checkXbergBinary(ctx context.Context, cfg ExtractorEmbedded) (version, error) {
	if err := checkBinaryPath("embeddings.extractor.embedded.xberg_binary", cfg.Binary); err != nil {
		return version{}, err
	}
	minVer := cfg.MinVersion
	if minVer == "" {
		minVer = DefaultExtractorMinVersion
	}
	want, err := parseVersion(minVer)
	if err != nil {
		return version{}, fmt.Errorf("embeddings.extractor.embedded.min_version: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cfg.Binary, "--version")
	cmd.Env = minimalEnv()
	out, err := cmd.Output()
	if err != nil {
		return version{}, fmt.Errorf("embeddings.extractor.embedded.xberg_binary: running '--version': %w", err)
	}
	got, err := parseXbergVersion(string(out))
	if err != nil {
		return version{}, fmt.Errorf("embeddings.extractor.embedded.xberg_binary %s does not report a version: %w", cfg.Binary, err)
	}
	if compareVersions(got, want) < 0 {
		return version{}, fmt.Errorf("embeddings.extractor.embedded.xberg_binary %s is version %d.%d.%d, older than the minimum %s (embeddings.extractor.embedded.min_version)",
			cfg.Binary, got[0], got[1], got[2], minVer)
	}
	return got, nil
}

// parseXbergVersion reads the version out of `xberg --version` ("xberg
// 1.3.10"): the first word that is a major.minor.patch version.
func parseXbergVersion(s string) (version, error) {
	for _, w := range strings.Fields(s) {
		if v, err := parseVersion(w); err == nil {
			return v, nil
		}
	}
	return version{}, fmt.Errorf("%q has no major.minor.patch version", strings.TrimSpace(s))
}

// start launches the process once and waits until it is healthy.
func (s *ExtractorSupervisor) start(ctx context.Context) error {
	port, err := pickLoopbackPort(s.cfg.AvoidPorts)
	if err != nil {
		return err
	}
	s.addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if err := s.launch(port); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx); err != nil {
		s.terminate()
		return err
	}
	s.log.Info("embedded xberg started", "addr", s.addr, "pid", s.PID())
	return nil
}

// env is the environment of the xberg process: a minimal one, so that nothing
// of the operator's shell reaches it. In particular XBERG_API_ALLOW_LOCAL_URI_INPUTS
// (server-side file reads over the API) and the XBERG_LLM_* keys are never
// inherited.
func (s *ExtractorSupervisor) env(port int) []string {
	hf := filepath.Join(s.cfg.CacheDir, xbergHFDir)
	env := append(minimalEnv(),
		"XBERG_CACHE_DIR="+s.cfg.CacheDir,
		"HF_HOME="+hf,
		"HF_HUB_CACHE="+filepath.Join(hf, "hub"),
		// xberg answers every origin by default; only its own is allowed, so a
		// web page cannot post files to it through the operator's browser.
		"XBERG_CORS_ORIGINS=http://127.0.0.1:"+strconv.Itoa(port),
	)
	if !s.cfg.AllowModelDownloads {
		env = append(env, "HF_HUB_OFFLINE=1")
	}
	// The operator's own Hugging Face mirror, if any, and the cross-tool opt-out
	// of telemetry: xberg is asked not to track, whatever it does by default.
	if v, ok := os.LookupEnv("HF_ENDPOINT"); ok && v != "" {
		env = append(env, "HF_ENDPOINT="+v)
	}
	env = append(env, "DO_NOT_TRACK=1")
	// Libraries that xberg finds by environment, not by anything it can download.
	for _, k := range []string{"TESSDATA_PREFIX", "ORT_DYLIB_PATH"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// launch starts the process, records it for crash cleanup and arms the watchdog.
func (s *ExtractorSupervisor) launch(port int) error {
	cmd := exec.Command(s.cfg.Binary, "serve", "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--config", s.confPath)
	cmd.Dir = s.cfg.CacheDir
	cmd.Env = s.env(port)
	s.tail = &tailBuffer{}
	cmd.Stdout, cmd.Stderr = s.tail, s.tail
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start xberg: %w", err)
	}
	s.cmd = cmd
	exited := make(chan struct{})
	s.exited = exited
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	if err := s.writeState(); err != nil {
		s.terminate()
		return err
	}
	s.watchdog = startWatchdog(cmd.Process.Pid, s.confPath)
	return nil
}

func (s *ExtractorSupervisor) writeState() error {
	data, err := json.Marshal(state{PID: s.cmd.Process.Pid, Binary: s.cfg.Binary, KV: s.confPath})
	if err != nil {
		return fmt.Errorf("encode supervisor state: %w", err)
	}
	return writeFileAtomic(filepath.Join(s.cfg.CacheDir, xbergStateFile), data, 0o600)
}

// waitHealthy polls GET /health until the xberg this supervisor started answers
// healthy with the version it was started as, exits or times out.
func (s *ExtractorSupervisor) waitHealthy(ctx context.Context) error {
	hc := &http.Client{Timeout: 2 * time.Second}
	exited := s.exited
	deadline := time.NewTimer(startTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-exited:
			return fmt.Errorf("%w: %s", errXbergEarlyExit, s.tail.String())
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("xberg did not become healthy within %s: %s", startTimeout, s.tail.String())
		case <-tick.C:
			got, ok := s.healthVersion(ctx, hc)
			if !ok {
				continue
			}
			if v, err := parseVersion(got); err != nil || v != s.want {
				// The pick-close-bind gap let another process take the port.
				return fmt.Errorf("%w: it reports version %q, xberg %d.%d.%d was started", errXbergForeign, clipRunes(got, 32), s.want[0], s.want[1], s.want[2])
			}
			select {
			case <-exited:
				// A healthy answer from a port whose own process is gone is a
				// stranger's, not xberg's.
				return fmt.Errorf("%w: %s", errXbergEarlyExit, s.tail.String())
			default:
			}
			return nil
		}
	}
}

// healthVersion asks GET /health and returns the version it reports, with
// whether the answer was a healthy one.
func (s *ExtractorSupervisor) healthVersion(ctx context.Context, hc *http.Client) (string, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Endpoint()+"/health", nil)
	if err != nil {
		return "", false
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var h struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&h) != nil || h.Status != "healthy" {
		return "", false
	}
	return h.Version, true
}

// reapStale stops an xberg a previous ipfs mcp left running (it was killed, and
// its watchdog did not run), but only a process whose command line shows it is
// that xberg: a recycled pid is left alone.
func (s *ExtractorSupervisor) reapStale() error {
	path := filepath.Join(s.cfg.CacheDir, xbergStateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read supervisor state: %w", err)
	}
	var st state
	if json.Unmarshal(data, &st) != nil || st.PID <= 0 {
		_ = os.Remove(path)
		return nil
	}
	if processIsStore(st.PID, st.KV) {
		s.log.Warn("stopping an embedded xberg left by an earlier ipfs mcp", "pid", st.PID)
		if err := stopPID(st.PID, stopTimeout); err != nil {
			return fmt.Errorf("stop leftover xberg (pid %d): %w", st.PID, err)
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove supervisor state: %w", err)
	}
	return nil
}

// terminate stops the process without waiting for graceful shutdown.
func (s *ExtractorSupervisor) terminate() {
	if s.proxy != nil {
		s.proxy.Close()
	}
	stopWatchdog(s.watchdog)
	s.watchdog = nil
	if s.cmd != nil && s.cmd.Process != nil {
		killProcess(s.cmd)
		<-s.exited
	}
	_ = os.Remove(filepath.Join(s.cfg.CacheDir, xbergStateFile))
}

// Close stops xberg: a graceful signal first, a kill after the timeout. It is
// safe to call more than once.
func (s *ExtractorSupervisor) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed || s.cmd == nil {
		return nil
	}
	s.closed = true
	if s.proxy != nil {
		s.proxy.Close()
	}
	if s.unlock != nil {
		defer s.unlock()
	}
	// The watchdog goes first: it must not signal a pid after the process is
	// gone and the number recycled.
	stopWatchdog(s.watchdog)
	s.watchdog = nil
	select {
	case <-s.exited:
	default:
		interrupt(s.cmd)
		select {
		case <-s.exited:
		case <-time.After(stopTimeout):
			killProcess(s.cmd)
			<-s.exited
		}
	}
	if err := os.Remove(filepath.Join(s.cfg.CacheDir, xbergStateFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove supervisor state: %w", err)
	}
	return nil
}
