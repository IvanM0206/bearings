//go:build !unix

package notebook

import "os"

// openNoFollow opens a file for reading. Platforms without O_NOFOLLOW rely
// on the lstat check done just before and on the os.SameFile comparison
// done just after.
func openNoFollow(path string) (*os.File, error) { return os.Open(path) }

func isSymlinkErr(error) bool { return false }

// syncDir is a no-op: directories cannot be fsynced on these platforms.
func syncDir(string) {}
