//go:build linux

package confirm

import "os/exec"

// platformFactor is the native presence factor of this OS: polkit
// authentication for the operator-installed action.
func platformFactor() factor { return newPolkitFactor(execRunner, exec.LookPath) }
