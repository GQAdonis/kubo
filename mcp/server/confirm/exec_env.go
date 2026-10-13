package confirm

import (
	"strings"
)

// The helpers run to prove presence (/usr/bin/swift, pkcheck, pkaction) are
// started with an environment built from an allowlist. An inherited variable
// must never redirect them to a different toolchain or library: an MCP host's
// env block is chosen by whoever wrote the MCP config, which is not always the
// person at the keyboard.

// trustedPath is the only PATH a helper sees.
const trustedPath = "/usr/bin:/bin:/usr/sbin:/sbin"

// envAllowlist names the inherited variables a helper may keep: where the
// user's home and temporary files are, and the locale.
var envAllowlist = []string{
	"HOME", "TMPDIR", "USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE",
	"__CF_USER_TEXT_ENCODING",
}

// envForbiddenPrefixes are dropped even if someone adds them to the allowlist:
// they select a developer toolchain, an SDK or a dynamic loader path.
var envForbiddenPrefixes = []string{"DEVELOPER_DIR", "TOOLCHAINS", "SDKROOT", "DYLD_", "LD_", "PATH"}

// safeEnv builds a helper's environment from the inherited one: PATH is fixed,
// allowlisted variables are copied, extra (the helper's own parameters) are
// appended last, and nothing else is passed.
func safeEnv(inherited, extra []string) []string {
	out := make([]string, 0, len(envAllowlist)+len(extra)+1)
	out = append(out, "PATH="+trustedPath)
	for _, kv := range inherited {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || forbiddenEnv(name) {
			continue
		}
		for _, allowed := range envAllowlist {
			if name == allowed {
				out = append(out, kv)
				break
			}
		}
	}
	return append(out, extra...)
}

func forbiddenEnv(name string) bool {
	for _, p := range envForbiddenPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
