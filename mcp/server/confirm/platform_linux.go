//go:build linux

package confirm

// platformFactor is the native presence factor of this OS: polkit
// authentication for the operator-installed action, through root-owned
// binaries at fixed paths.
func platformFactor() factor { return newPolkitFactor(execRunner, trustedBinary) }
