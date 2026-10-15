package embeddings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Names inside the operator's data_dir.
const (
	// kvDirName is the surrealkv store.
	kvDirName = "db"
	// stateFileName records the child process so a later start can reap it.
	stateFileName = "supervisor.json"
	// credentialsFileName holds the current root credentials, mode 0600.
	credentialsFileName = "credentials.json"
	// pendingFileName holds credentials while a start rotates to them.
	pendingFileName = "credentials.pending.json"
)

// The namespace and database of the embedded store.
const (
	embeddedNamespace = "ipfs_mcp"
	embeddedDatabase  = "index"
	rootUser          = "root"
)

const (
	startTimeout   = 30 * time.Second
	stopTimeout    = 10 * time.Second
	versionTimeout = 10 * time.Second
	maxStartTries  = 3
	stderrTail     = 2048
)

// Supervisor owns the SurrealDB process of embedded mode. The process binds
// 127.0.0.1 on an ephemeral port, runs with credentials generated for this
// start, and does not outlive the ipfs mcp process: Close stops it, a watchdog
// stops it if ipfs mcp is killed, and the next start reaps it if both failed.
type Supervisor struct {
	cfg  EmbeddedSettings
	log  *slog.Logger
	addr string
	cred Secret
	// credFile is the 0600 file that holds the credentials.
	credFile string

	cmd      *exec.Cmd
	tail     *tailBuffer
	exited   chan struct{}
	watchdog *exec.Cmd
	closeMu  sync.Mutex
	closed   bool
}

// StartSupervisor validates the binary, reaps a process a crashed ipfs mcp left
// behind, starts SurrealDB and waits until it is healthy and the credentials
// are in force. It returns an error, with nothing left running, otherwise.
func StartSupervisor(ctx context.Context, cfg EmbeddedSettings, log *slog.Logger) (*Supervisor, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := (Settings{Mode: ModeEmbedded, Embedded: &cfg}).validateEmbedded(); len(err) > 0 {
		return nil, errors.Join(err...)
	}
	if err := checkBinary(ctx, cfg); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("embeddings.embedded.data_dir: %w", err)
	}
	s := &Supervisor{cfg: cfg, log: log, credFile: filepath.Join(cfg.DataDir, credentialsFileName)}
	if err := s.reapStale(); err != nil {
		return nil, err
	}
	var lastErr error
	for range maxStartTries {
		err := s.start(ctx)
		if err == nil {
			return s, nil
		}
		lastErr = err
		if !errors.Is(err, errEarlyExit) || ctx.Err() != nil {
			break
		}
		// The port was taken between choosing and binding; choose again.
	}
	return nil, lastErr
}

// Endpoint is the base URL of the store.
func (s *Supervisor) Endpoint() string { return "http://" + s.addr }

// Addr is the host:port the store listens on.
func (s *Supervisor) Addr() string { return s.addr }

// Credential is the "user:password" of the store's root user for this start.
func (s *Supervisor) Credential() Secret { return s.cred }

// CredentialsFile is the path of the 0600 file that holds the credentials.
func (s *Supervisor) CredentialsFile() string { return s.credFile }

// PID is the process id of the store, or 0 if it is not running.
func (s *Supervisor) PID() int {
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}

// Done is closed when the store process has exited.
func (s *Supervisor) Done() <-chan struct{} { return s.exited }

func (s *Supervisor) kvPath() string { return filepath.Join(s.cfg.DataDir, kvDirName) }

// errEarlyExit marks a store that exited before it became healthy.
var errEarlyExit = errors.New("surreal exited during startup")

// tailBuffer keeps the last bytes written to it: the store's output is kept
// only to explain a failed start, never forwarded (stdout is ipfs mcp's
// protocol channel) and never logged.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTail {
		t.buf = t.buf[len(t.buf)-stderrTail:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// checkBinaryPath refuses a path that is missing, not a regular file or not
// executable. It is a configuration error: startup fails, naming key.
func checkBinaryPath(key, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s %s is not a regular file", key, path)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("%s %s is not executable", key, path)
	}
	return nil
}

// checkBinary refuses a binary that is missing, not a regular executable file
// or older than the minimum version. The path is absolute (validated) and is
// never resolved through PATH.
func checkBinary(ctx context.Context, cfg EmbeddedSettings) error {
	if err := checkBinaryPath("embeddings.embedded.surreal_binary", cfg.Binary); err != nil {
		return err
	}
	min := cfg.MinVersion
	if min == "" {
		min = DefaultMinVersion
	}
	want, err := parseVersion(min)
	if err != nil {
		return fmt.Errorf("embeddings.embedded.min_version: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	// The binary is the operator's, but it does not need the operator's
	// environment: nothing of the shell (API keys, tokens) is passed on.
	vcmd := exec.CommandContext(ctx, cfg.Binary, "version")
	vcmd.Env = minimalEnv()
	out, err := vcmd.Output()
	if err != nil {
		return fmt.Errorf("embeddings.embedded.surreal_binary: running 'version': %w", err)
	}
	got, err := parseVersion(string(out))
	if err != nil {
		return fmt.Errorf("embeddings.embedded.surreal_binary %s does not report a version: %w", cfg.Binary, err)
	}
	if compareVersions(got, want) < 0 {
		return fmt.Errorf("embeddings.embedded.surreal_binary %s is version %d.%d.%d, older than the minimum %s (embeddings.embedded.min_version)",
			cfg.Binary, got[0], got[1], got[2], min)
	}
	return nil
}

type version [3]int

// parseVersion reads "3.3.0", "v3.3.0" or "3.3.0 for macos on aarch64": the
// first word, up to three dot-separated numbers, any pre-release suffix ignored.
func parseVersion(s string) (version, error) {
	word, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	word = strings.TrimPrefix(strings.TrimSpace(word), "v")
	parts := strings.SplitN(word, ".", 3)
	if len(parts) != 3 {
		return version{}, fmt.Errorf("%q is not a major.minor.patch version", strings.TrimSpace(s))
	}
	var v version
	for i, p := range parts {
		p, _, _ = strings.Cut(p, "-")
		p, _, _ = strings.Cut(p, "+")
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return version{}, fmt.Errorf("%q is not a major.minor.patch version", strings.TrimSpace(s))
		}
		v[i] = n
	}
	return v, nil
}

func compareVersions(a, b version) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// start launches the process once and brings the store into a usable state.
func (s *Supervisor) start(ctx context.Context) error {
	candidates := s.knownCredentials()
	fresh, err := newCredentials()
	if err != nil {
		return err
	}
	if err := writeCredentials(filepath.Join(s.cfg.DataDir, pendingFileName), fresh); err != nil {
		return err
	}
	port, err := s.pickPort()
	if err != nil {
		return err
	}
	s.addr = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if err := s.launch(fresh); err != nil {
		return err
	}
	if err := s.waitHealthy(ctx); err != nil {
		s.terminate()
		return err
	}
	if err := s.takeOver(ctx, fresh, candidates); err != nil {
		s.terminate()
		return err
	}
	if err := os.Rename(filepath.Join(s.cfg.DataDir, pendingFileName), s.credFile); err != nil {
		s.terminate()
		return fmt.Errorf("record credentials: %w", err)
	}
	s.cred = Secret(rootUser + ":" + fresh)
	s.log.Info("embedded SurrealDB started", "addr", s.addr, "pid", s.PID())
	return nil
}

// pickPort asks the OS for a free loopback port that is not another local
// service's well-known port.
func (s *Supervisor) pickPort() (int, error) { return pickLoopbackPort(s.cfg.AvoidPorts) }

// pickLoopbackPort asks the OS for a free loopback port that is not in avoid.
func pickLoopbackPort(avoid []int) (int, error) {
	return pickPort(avoid, func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") })
}

// pickPort takes the port of each listener listen returns, closes the listener
// and returns the first port not in avoid. listen is a seam of tests.
func pickPort(avoid []int, listen func() (net.Listener, error)) (int, error) {
	for range 20 {
		l, err := listen()
		if err != nil {
			return 0, fmt.Errorf("choose a loopback port: %w", err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		if err := l.Close(); err != nil {
			return 0, fmt.Errorf("choose a loopback port: %w", err)
		}
		if !slices.Contains(avoid, port) {
			return port, nil
		}
	}
	return 0, errors.New("choose a loopback port: the OS kept offering reserved ports")
}

// launch starts the process, records it for crash cleanup and arms the watchdog.
func (s *Supervisor) launch(password string) error {
	cmd := exec.Command(s.cfg.Binary, "start", "--no-banner", "--deny-guests", "--bind", s.addr, "surrealkv://"+s.kvPath())
	// A minimal environment: the store needs none of the operator's other
	// secrets. The initial root credentials travel in the environment, not
	// in the argument list, which any local user can read.
	cmd.Env = append(minimalEnv(), "SURREAL_USER="+rootUser, "SURREAL_PASS="+password)
	s.tail = &tailBuffer{}
	cmd.Stdout, cmd.Stderr = s.tail, s.tail
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start surreal: %w", err)
	}
	s.cmd = cmd
	s.exited = make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()
	if err := writeState(s.cfg.DataDir, state{PID: cmd.Process.Pid, Binary: s.cfg.Binary, KV: s.kvPath()}); err != nil {
		s.terminate()
		return err
	}
	s.watchdog = startWatchdog(cmd.Process.Pid, s.kvPath())
	return nil
}

func minimalEnv() []string {
	var env []string
	for _, k := range []string{"PATH", "HOME", "TMPDIR", "SystemRoot", "TEMP", "TMP"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// waitHealthy polls /health until the store answers, exits or times out.
func (s *Supervisor) waitHealthy(ctx context.Context) error {
	c := newSurrealClient(s.Endpoint(), "", "", auth{}, nil)
	deadline := time.NewTimer(startTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.exited:
			return fmt.Errorf("%w: %s", errEarlyExit, s.tail.String())
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("surreal did not become healthy within %s: %s", startTimeout, s.tail.String())
		case <-tick.C:
			hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := c.health(hctx)
			cancel()
			if err == nil {
				return nil
			}
		}
	}
}

// takeOver makes the credentials of this start the ones in force. A new data
// dir takes them from the environment. A data dir that already has a root user
// ignores them, so the supervisor signs in with the credentials it stored
// earlier and replaces the password.
func (s *Supervisor) takeOver(ctx context.Context, fresh string, candidates []string) error {
	works := func(pass string) (bool, error) {
		c := newSurrealClient(s.Endpoint(), "", "", auth{user: rootUser, pass: pass}, nil)
		_, err := c.query(ctx, "RETURN 1;", nil)
		if errors.Is(err, ErrAuth) {
			return false, nil
		}
		return err == nil, err
	}
	ok, err := works(fresh)
	if err != nil {
		return fmt.Errorf("sign in to the embedded store: %w", err)
	}
	if ok {
		return nil
	}
	for _, old := range candidates {
		ok, err := works(old)
		if err != nil {
			return fmt.Errorf("sign in to the embedded store: %w", err)
		}
		if !ok {
			continue
		}
		c := newSurrealClient(s.Endpoint(), "", "", auth{user: rootUser, pass: old}, nil)
		// The password is lower-case hex generated here, so it is safe in a
		// string literal; DEFINE USER takes no bound parameter for it.
		if _, err := c.query(ctx, `DEFINE USER OVERWRITE `+rootUser+` ON ROOT PASSWORD "`+fresh+`" ROLES OWNER;`, nil); err != nil {
			return fmt.Errorf("rotate the embedded store credentials: %w", err)
		}
		if ok, err := works(fresh); err != nil || !ok {
			return fmt.Errorf("rotate the embedded store credentials: the new password is not accepted (%v)", err)
		}
		return nil
	}
	return fmt.Errorf("the store in %s has a root user that none of the credentials in %s or %s opens; "+
		"restore credentials.json or remove %s to start a new index", s.cfg.DataDir, credentialsFileName, pendingFileName, s.kvPath())
}

// knownCredentials returns the passwords an earlier start may have left in
// force, newest first: a rotation that crashed halfway leaves the pending one.
func (s *Supervisor) knownCredentials() []string {
	var out []string
	for _, name := range []string{pendingFileName, credentialsFileName} {
		if pass, err := readCredentials(filepath.Join(s.cfg.DataDir, name)); err == nil {
			out = append(out, pass)
		}
	}
	return out
}

type credentialsDoc struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newCredentials() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate credentials: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func writeCredentials(path, password string) error {
	data, err := json.Marshal(credentialsDoc{Username: rootUser, Password: password})
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	return writeFileAtomic(path, data, 0o600)
}

func readCredentials(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var d credentialsDoc
	if err := json.Unmarshal(data, &d); err != nil || d.Password == "" {
		return "", errors.New("unreadable credentials file")
	}
	return d.Password, nil
}

// writeFileAtomic writes a file with the given mode and renames it into place,
// so a reader never sees a partial file and the mode is never wider than perm.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

// state is what a later start needs to recognise and stop a leftover process.
type state struct {
	PID    int    `json:"pid"`
	Binary string `json:"binary"`
	KV     string `json:"kv"`
}

func writeState(dir string, st state) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("encode supervisor state: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, stateFileName), data, 0o600)
}

// reapStale stops a store a previous ipfs mcp left running (it was killed, and
// its watchdog did not run), but only a process whose command line shows it is
// that store: a recycled pid is left alone.
func (s *Supervisor) reapStale() error {
	path := filepath.Join(s.cfg.DataDir, stateFileName)
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
		s.log.Warn("stopping an embedded SurrealDB left by an earlier ipfs mcp", "pid", st.PID)
		if err := stopPID(st.PID, stopTimeout); err != nil {
			return fmt.Errorf("stop leftover surreal (pid %d): %w", st.PID, err)
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove supervisor state: %w", err)
	}
	return nil
}

// terminate stops the process without waiting for graceful shutdown.
func (s *Supervisor) terminate() {
	stopWatchdog(s.watchdog)
	s.watchdog = nil
	if s.cmd != nil && s.cmd.Process != nil {
		killProcess(s.cmd)
		<-s.exited
	}
	_ = os.Remove(filepath.Join(s.cfg.DataDir, stateFileName))
}

// Close stops the store: a graceful signal first, a kill after the timeout. It
// is safe to call more than once.
func (s *Supervisor) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed || s.cmd == nil {
		return nil
	}
	s.closed = true
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
	if err := os.Remove(filepath.Join(s.cfg.DataDir, stateFileName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove supervisor state: %w", err)
	}
	return nil
}
