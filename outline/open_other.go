//go:build !unix

package outline

import "os"

// openFile opens a path for Read. Without O_NOFOLLOW and O_NONBLOCK, Read's
// checks before and after opening are what keep symlinks and devices out.
func openFile(path string) (*os.File, error) { return os.Open(path) }
