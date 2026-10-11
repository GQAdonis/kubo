package confirm

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"
)

// pairingFile is the stored form of a pairing. It holds only a hash of the
// key: the key itself lives in IPFS Desktop's safeStorage, so reading this file
// does not let a same-user process act as Desktop.
type pairingFile struct {
	Version  int       `json:"version"`
	Verifier string    `json:"verifier"`
	Created  time.Time `json:"created"`
}

// ErrNotPaired is returned when no pairing exists.
var ErrNotPaired = errors.New("not paired with IPFS Desktop: run `ipfs mcp pair` and enter the key in Desktop")

// Pair creates a new pairing key, stores its hash at path (replacing any
// earlier pairing) and returns the key. The key is shown once; it is never
// stored by the server.
func Pair(path string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate pairing key: %w", err)
	}
	key := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(key))
	data, err := json.Marshal(pairingFile{Version: 1, Verifier: hex.EncodeToString(sum[:]), Created: time.Now().UTC()})
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", fmt.Errorf("write pairing: %w", err)
	}
	return key, nil
}

// writeFileAtomic replaces path in one step (write a private temporary file in
// the same directory, then rename it), so a running server that reads the
// pairing for each request sees either the old verifier or the new one, never
// a partial file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create pairing directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".pairing-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// loadVerifier reads the stored key hash.
func loadVerifier(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotPaired
	}
	if err != nil {
		return nil, fmt.Errorf("read pairing: %w", err)
	}
	var f pairingFile
	if err := json.Unmarshal(data, &f); err != nil || f.Version != 1 {
		return nil, fmt.Errorf("pairing file %s is not valid; pair again", path)
	}
	v, err := hex.DecodeString(f.Verifier)
	if err != nil || len(v) != sha256.Size {
		return nil, fmt.Errorf("pairing file %s is not valid; pair again", path)
	}
	return v, nil
}

// keyMatches reports whether key hashes to verifier, in constant time.
func keyMatches(verifier []byte, key string) bool {
	if len(verifier) == 0 || key == "" {
		return false
	}
	sum := sha256.Sum256([]byte(key))
	return subtle.ConstantTimeCompare(sum[:], verifier) == 1
}

// Errors of RunPair.
var (
	// ErrPairNoTTY is returned when stdin is not a terminal.
	ErrPairNoTTY = errors.New("ipfs mcp pair needs a terminal on stdin; it will not create a pairing key for a script or an agent")
	// ErrPairServerRunning is returned while a server holds a confirmation socket.
	ErrPairServerRunning = errors.New("an ipfs mcp server is running with a confirmation channel; stop it before replacing the pairing")
)

// PairOptions configure RunPair.
type PairOptions struct {
	// PairingFile and SocketDir override the default locations.
	PairingFile string
	SocketDir   string
	// In is the terminal. IsTerminal reports whether f is one; nil uses the
	// real check.
	In         *os.File
	IsTerminal func(f *os.File) bool
}

// RunPair is `ipfs mcp pair`: it creates the pairing key and returns it. It
// refuses without a terminal on stdin, so a process that only runs the command
// (an agent, a script) cannot obtain a key, and it refuses while a server holds
// a confirmation socket, so a pairing is never swapped under a running server.
func RunPair(o PairOptions) (string, error) {
	isTTY := o.IsTerminal
	if isTTY == nil {
		isTTY = func(f *os.File) bool { return f != nil && term.IsTerminal(int(f.Fd())) }
	}
	if !isTTY(o.In) {
		return "", ErrPairNoTTY
	}
	cfg := Config{SocketDir: o.SocketDir, PairingFile: o.PairingFile}
	if serverHoldsSocket(cfg.socketDir()) {
		return "", ErrPairServerRunning
	}
	return Pair(cfg.pairingFile())
}

// serverHoldsSocket reports whether any socket in dir accepts a connection. A
// stale socket file that nothing listens on is not a server.
func serverHoldsSocket(dir string) bool {
	socks, _ := filepath.Glob(filepath.Join(dir, "*.sock"))
	for _, sock := range socks {
		conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
		if err != nil {
			continue
		}
		_ = conn.Close()
		return true
	}
	return false
}
