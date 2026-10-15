//go:build !windows

package embeddings

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ownGroup puts the child in its own process group, so a signal aimed at the
// terminal's foreground group does not reach it before Close can stop it
// gracefully, and so killProcess can take its whole group.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// interrupt asks the store to shut down cleanly.
func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Signal(syscall.SIGTERM) }

// killProcess kills the store's process group, and only that group: its id is
// the pid of the process this supervisor started.
func killProcess(cmd *exec.Cmd) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}

// watchdogScript waits for the ipfs mcp process to disappear and then stops the
// store, so a SIGKILL of ipfs mcp does not leave it behind. It signals the pid
// only while that pid's command line still names this store's data path, so a
// recycled pid is never signalled.
const watchdogScript = `parent=$1; child=$2; kv=$3
while kill -0 "$parent" 2>/dev/null; do sleep 1; done
is_store() { ps -p "$child" -o command= 2>/dev/null | grep -F -q -- "$kv"; }
if is_store; then
  kill "$child" 2>/dev/null
  n=0
  while is_store && [ "$n" -lt 10 ]; do sleep 1; n=$((n+1)); done
  if is_store; then kill -9 "$child" 2>/dev/null; fi
fi`

// startWatchdog starts the watchdog for the store process pid. It returns nil
// when no shell is available; the next start's reapStale is then the fallback.
func startWatchdog(pid int, kv string) *exec.Cmd {
	sh, err := exec.LookPath("sh")
	if err != nil {
		return nil
	}
	cmd := exec.Command(sh, "-c", watchdogScript, "sh", strconv.Itoa(os.Getpid()), strconv.Itoa(pid), kv)
	cmd.Env = minimalEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil
	}
	return cmd
}

// stopWatchdog stops a watchdog that is still waiting, with its sleep.
func stopWatchdog(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// processIsStore reports whether pid is a live process whose command line
// names this store's data path.
func processIsStore(pid int, kv string) bool {
	if kv == "" || !pidAlive(pid) {
		return false
	}
	ps := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	ps.Env = minimalEnv()
	out, err := ps.Output()
	return err == nil && strings.Contains(string(out), kv)
}

func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// stopPID stops a process that is not this supervisor's child: SIGTERM, then
// SIGKILL after the timeout.
func stopPID(pid int, timeout time.Duration) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	deadline := time.Now().Add(timeout)
	for pidAlive(pid) {
		if time.Now().After(deadline) {
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
