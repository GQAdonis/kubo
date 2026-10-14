//go:build !windows

package embeddings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// xbergLockFile is held, with flock, for as long as an ipfs mcp runs the
// embedded xberg of a cache_dir. The kernel drops the lock when that process
// dies, however it dies, so a lock that can be taken means no live ipfs mcp
// owns the directory and a recorded child is stale.
const xbergLockFile = "xberg-supervisor.lock"

// checkCacheDirOwner refuses a cache_dir that another user could write to or
// own: xberg's models, its configuration and the supervisor's state live there,
// and anything written into it runs as the operator the next time it is read.
func checkCacheDirOwner(dir string) error {
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("embeddings.extractor.embedded.cache_dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("embeddings.extractor.embedded.cache_dir %s is not a directory", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("embeddings.extractor.embedded.cache_dir %s is owned by uid %d, not by the user running ipfs mcp (uid %d); use a directory you own", dir, st.Uid, os.Getuid())
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("embeddings.extractor.embedded.cache_dir %s is writable by its group or by everyone (mode %04o); run chmod go-w on it", dir, fi.Mode().Perm())
	}
	return nil
}

// lockCacheDir takes the exclusive lock of cache_dir without waiting. The
// returned function releases it.
func lockCacheDir(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, xbergLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("embeddings.extractor.embedded.cache_dir: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("embeddings.extractor.embedded.cache_dir %s is in use by another ipfs mcp; give each ipfs mcp its own cache_dir", dir)
		}
		return nil, fmt.Errorf("embeddings.extractor.embedded.cache_dir: lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
