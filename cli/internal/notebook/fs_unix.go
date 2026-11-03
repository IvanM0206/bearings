//go:build unix

package notebook

import (
	"errors"
	"os"
	"syscall"
)

// openNoFollow opens a file for reading without following a symlink in the
// last path element. O_NONBLOCK keeps a FIFO swapped in at the last moment
// from blocking the reader; the caller then rejects it with fstat.
func openNoFollow(path string) (*os.File, error) {
	//nolint:gosec // G304: path is built from validated parts and checked to stay inside the root
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

func isSymlinkErr(err error) bool { return errors.Is(err, syscall.ELOOP) }

// syncDir fsyncs a directory so that a rename in it is durable. Errors are
// ignored: some filesystems do not support it, and the data itself was
// already synced.
func syncDir(dir string) {
	d, err := os.Open(dir) //nolint:gosec // G304: a directory inside the root, opened only to fsync it
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
