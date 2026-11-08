//go:build unix

package outline

import (
	"os"
	"syscall"
)

// openFile opens a path for Read without following a symlink and without
// blocking: if the path was swapped for a symlink after Read's Lstat, the
// open fails; if it was swapped for a named pipe, the open returns at once
// and Read's check of the opened file rejects it.
func openFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
