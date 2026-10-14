//go:build windows

package embeddings

// Windows has no Unix owner or mode bits to check here and no flock: the
// directory inherits the ACL of its parent (the operator's profile), and a
// second ipfs mcp on the same cache_dir is not detected. The watchdog and the
// start-up reaping of supervisor.go apply as they do for the SurrealDB store.

func checkCacheDirOwner(string) error { return nil }

func lockCacheDir(string) (func(), error) { return func() {}, nil }
