//go:build !unix

package confirm

import "os"

// fileOwner reports no owner: this platform has no numeric file owners, so a
// check that needs one fails closed.
func fileOwner(os.FileInfo) (uint32, bool) { return 0, false }

// currentUID is unused where fileOwner reports nothing.
func currentUID() uint32 { return 0 }
