package server

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
)

// Opener opens a URL in the person's browser.
type Opener func(rawURL string) error

// OpenBrowser opens rawURL with the platform's opener. It never goes through
// a shell: the program and its arguments are an explicit list, and the URL
// must be one of our own loopback addresses.
func OpenBrowser(rawURL string) error {
	name, args, err := browserCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	// The program name is one of three constants and the only variable
	// argument is the validated loopback URL.
	cmd := exec.Command(name, args...) //nolint:gosec // G204: fixed program names, validated URL, no shell
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening the browser: %w", err)
	}
	go func() { _ = cmd.Wait() }() // reap the child
	return nil
}

// browserCommand returns the program and arguments that open rawURL on goos.
func browserCommand(goos, rawURL string) (string, []string, error) {
	if err := validateLocalURL(rawURL); err != nil {
		return "", nil, err
	}
	switch goos {
	case "windows":
		return "rundll32.exe", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	case "darwin":
		return "open", []string{rawURL}, nil
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		return "xdg-open", []string{rawURL}, nil
	}
	return "", nil, fmt.Errorf("no browser opener for %s", goos)
}

// validateLocalURL accepts only http://127.0.0.1:PORT/... addresses without
// credentials, so that nothing else can ever be handed to the opener.
func validateLocalURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid URL")
	}
	port, perr := strconv.Atoi(u.Port())
	if u.Scheme != "http" || u.Opaque != "" || u.User != nil || u.Hostname() != Host ||
		perr != nil || port < 1 || port > 65535 || u.Fragment != "" {
		return errors.New("not a local server URL")
	}
	for _, r := range rawURL {
		if r <= ' ' || r == 0x7f {
			return errors.New("invalid URL")
		}
	}
	return nil
}
