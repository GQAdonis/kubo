// Package confirm holds the out-of-band confirmation resolvers of the IPFS MCP
// server: the native operating system prompt with a presence factor, the
// paired IPFS Desktop channel and the headless `ipfs mcp confirm` opt-in.
//
// A resolver here is the human gate for sessions that cannot elicit. Nothing
// in this package approves anything on its own: a confirmation is approved
// only when a presence factor (Touch ID or the login password, a polkit
// authentication, a gesture in the paired Desktop window) says so, and every
// failure, absence or doubt leaves the call unrun.
//
// The package is on by default with the native and desktop mechanisms. A
// mechanism the platform cannot offer is skipped, so a non-eliciting client
// gets confirmation_unavailable. --confirm=none or IPFS_MCP_CONFIRM=none turns
// every resolver off; cli is only ever enabled by naming it.
package confirm

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Modes an operator can name.
const (
	// ModeNative is LocalAuthentication on macOS and polkit on Linux.
	ModeNative = "native"
	// ModeDesktop is the channel paired with IPFS Desktop.
	ModeDesktop = "desktop"
	// ModeCLI is `ipfs mcp confirm` for headless servers.
	ModeCLI = "cli"
)

// DefaultModes is what runs when neither --confirm nor IPFS_MCP_CONFIRM is
// set: the human gate is on. A mechanism the platform cannot offer (no
// /usr/bin/swift, no polkit action, an unpaired Desktop) is left out and the
// call is refused with confirmation_unavailable. cli is never a default.
// --confirm=none (or IPFS_MCP_CONFIRM=none) turns the gate's resolvers off.
var DefaultModes = []string{ModeNative, ModeDesktop}

// EnvModes is the environment variable that carries --confirm.
const EnvModes = "IPFS_MCP_CONFIRM"

// Environment variables that locate the channel files.
const (
	EnvSocketDir   = "IPFS_MCP_CONFIRM_DIR"
	EnvPairingFile = "IPFS_MCP_PAIRING_FILE"
)

// Config selects and locates the resolvers.
type Config struct {
	// Modes is a subset of ModeNative, ModeDesktop and ModeCLI.
	Modes []string
	// SocketDir holds the per-process channel sockets. Empty means the
	// default private directory.
	SocketDir string
	// PairingFile holds the Desktop pairing verifier. Empty means the
	// default location.
	PairingFile string
}

// ParseModes parses a comma-separated --confirm value. The empty string and
// "none" mean no resolver.
func ParseModes(s string) ([]string, error) {
	var out []string
	for _, m := range strings.Split(s, ",") {
		m = strings.ToLower(strings.TrimSpace(m))
		switch m {
		case "", "none":
		case ModeNative, ModeDesktop, ModeCLI:
			if !slices.Contains(out, m) {
				out = append(out, m)
			}
		default:
			return nil, fmt.Errorf("unknown confirmation mechanism %q: choose from %s, %s, %s or none", m, ModeNative, ModeDesktop, ModeCLI)
		}
	}
	return out, nil
}

// Has reports whether a mode is enabled.
func (c Config) Has(mode string) bool { return slices.Contains(c.Modes, mode) }

// socketDir returns the directory for channel sockets.
func (c Config) socketDir() string {
	if c.SocketDir != "" {
		return c.SocketDir
	}
	if d := os.Getenv(EnvSocketDir); d != "" {
		return d
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "ipfs-mcp")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("ipfs-mcp-%d", os.Getuid()))
}

// pairingFile returns the path of the pairing verifier.
func (c Config) pairingFile() string {
	if c.PairingFile != "" {
		return c.PairingFile
	}
	if p := os.Getenv(EnvPairingFile); p != "" {
		return p
	}
	return filepath.Join(ipfsPath(), "mcp-confirm-pairing.json")
}

// ipfsPath is $IPFS_PATH, or ~/.ipfs. It is only a file location; the repo
// is never opened.
func ipfsPath() string {
	if p := os.Getenv("IPFS_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ipfs"
	}
	return filepath.Join(home, ".ipfs")
}

var current struct {
	mu  sync.Mutex
	cfg *Config
}

// Configure sets the configuration the registered provider uses. Without a
// call the provider reads IPFS_MCP_CONFIRM.
func Configure(c Config) {
	current.mu.Lock()
	defer current.mu.Unlock()
	current.cfg = &c
}

// configured returns the active configuration.
func configured() (Config, error) {
	current.mu.Lock()
	defer current.mu.Unlock()
	if current.cfg != nil {
		return *current.cfg, nil
	}
	v, set := os.LookupEnv(EnvModes)
	if !set || strings.TrimSpace(v) == "" {
		return Config{Modes: slices.Clone(DefaultModes)}, nil
	}
	modes, err := ParseModes(v)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", EnvModes, err)
	}
	return Config{Modes: modes}, nil
}

// PairingPath is the file that stores the Desktop pairing verifier.
func (c Config) PairingPath() string { return c.pairingFile() }
