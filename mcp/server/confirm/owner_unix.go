//go:build unix

package confirm

import (
	"os"
	"syscall"
)

// fileOwner returns the numeric owner of a file. ok is false when the platform
// does not report one.
func fileOwner(fi os.FileInfo) (uid uint32, ok bool) {
	st, isStat := fi.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, false
	}
	return st.Uid, true
}

// currentUID is the user the process runs as.
func currentUID() uint32 { return uint32(os.Getuid()) }
