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
	"os"
	"path/filepath"
	"time"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create pairing directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write pairing: %w", err)
	}
	return key, nil
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
