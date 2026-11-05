// Command gen copies the starter notebooks from the repository's
// templates/ directory into the templates package, so that they can be
// embedded in the binary (go:embed cannot reach outside the module).
//
// Usage (from go:generate): go run ./gen <source dir> <destination dir>
//
// The destination is replaced entirely. Only regular files are copied;
// symlinks and special files make the generator fail, and hidden files
// (such as .DS_Store) are skipped.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen <source dir> <destination dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

const maxFile = 1 << 20

func run(src, dst string) error {
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		return fmt.Errorf("source %q is not a directory", src)
	}
	if err := os.RemoveAll(dst); err != nil { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil { //nolint:gosec // G301,G703: generated source tree, committed to the repository
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("%s: symlinks are not allowed in templates", rel)
		case d.IsDir():
			return os.MkdirAll(out, 0o755) //nolint:gosec // G301: generated source tree
		case !d.Type().IsRegular():
			return fmt.Errorf("%s: not a regular file", rel)
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: walking the repository's own templates directory
		if err != nil {
			return err
		}
		if len(data) > maxFile {
			return errors.New(rel + ": file too large")
		}
		return os.WriteFile(out, data, 0o644) //nolint:gosec // G306,G703: generated source file, committed to the repository
	})
}
