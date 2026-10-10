//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup makes cmd the leader of a new process group, so its children
// (Ollama starts a runner process per model) can be killed with it.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the group that ownProcessGroup made, and only it: the
// group ID is the PID of the process the test started.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
