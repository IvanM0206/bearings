// Command gen builds the web interface (web/) and copies the result into the
// server package's webdist/ directory, which is committed and embedded in the
// binary. Go alone is enough to build bearings; Node is only needed by the
// person who regenerates the interface.
//
// Usage (from go:generate): go run ./gen <web dir> <destination dir>
//
// It runs `npm ci` (exact versions from package-lock.json, install scripts
// disabled) and `npm run build` in the web directory, then replaces the
// destination with the build output. File names carry a content hash and the
// copy is byte for byte, so building the same sources twice gives the same
// tree and CI can check it with `go generate` followed by `git diff`.
// Symlinks and special files in the build output make the generator fail,
// and hidden files are skipped.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen <web dir> <destination dir>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

// maxFile bounds a single file of the build output.
const maxFile = 8 << 20

func run(webDir, dst string) error {
	if _, err := os.Stat(filepath.Join(webDir, "package-lock.json")); err != nil { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		return fmt.Errorf("%s is not the web directory (package-lock.json not found)", webDir)
	}
	npm, err := exec.LookPath("npm")
	if err != nil {
		return errors.New("npm not found: install Node.js 22 or newer to regenerate the web interface")
	}
	for _, args := range [][]string{
		{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		{"run", "build"},
	} {
		cmd := exec.CommandContext(context.Background(), npm, args...) //nolint:gosec // G204: fixed arguments, dev-time generator
		cmd.Dir = webDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "npm_config_update_notifier=false")
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("npm %s: %w", strings.Join(args, " "), err)
		}
	}
	return copyTree(filepath.Join(webDir, "dist"), dst)
}

func copyTree(src, dst string) error {
	if fi, err := os.Lstat(src); err != nil || !fi.IsDir() { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		return fmt.Errorf("build output %q is not a directory", src)
	}
	if err := os.RemoveAll(dst); err != nil { //nolint:gosec // G703: dev-time generator, paths come from go:generate
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil { //nolint:gosec // G301,G703: generated tree, committed to the repository
		return err
	}
	copied := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error { //nolint:gosec // G703: dev-time generator, paths come from go:generate
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
			return fmt.Errorf("%s: symlinks are not allowed in the build output", rel)
		case d.IsDir():
			return os.MkdirAll(out, 0o755) //nolint:gosec // G301: generated tree
		case !d.Type().IsRegular():
			return fmt.Errorf("%s: not a regular file", rel)
		}
		data, err := os.ReadFile(p) //nolint:gosec // G304: walking the web build output
		if err != nil {
			return err
		}
		if len(data) > maxFile {
			return errors.New(rel + ": file too large")
		}
		copied++
		return os.WriteFile(out, data, 0o644) //nolint:gosec // G306,G703: generated file, committed to the repository
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dst, "index.html")); err != nil { //nolint:gosec // G703: dev-time generator
		return errors.New("the build output has no index.html")
	}
	fmt.Printf("gen: copied %d files to %s\n", copied, dst)
	return nil
}
