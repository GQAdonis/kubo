//go:build darwin

package confirm

// platformFactor is the native presence factor of this OS: macOS
// LocalAuthentication, through the system swift interpreter (no cgo).
func platformFactor() factor { return newMacFactor(execRunner) }
