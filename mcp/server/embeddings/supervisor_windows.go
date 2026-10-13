//go:build windows

package embeddings

import (
	"os/exec"
	"time"
)

func ownGroup(*exec.Cmd) {}

func interrupt(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

func killProcess(cmd *exec.Cmd) { _ = cmd.Process.Kill() }

// There is no watchdog on Windows: a killed ipfs mcp leaves the store running
// until the next start, which cannot verify a recorded pid and so leaves it;
// the data dir stays locked by the leftover process until it is stopped.
func startWatchdog(int, string) *exec.Cmd { return nil }

func stopWatchdog(*exec.Cmd) {}

func processIsStore(int, string) bool { return false }

func stopPID(int, time.Duration) error { return nil }
