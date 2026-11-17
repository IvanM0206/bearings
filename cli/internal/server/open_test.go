package server

import (
	"slices"
	"testing"
)

func TestBrowserCommand(t *testing.T) {
	const u = "http://127.0.0.1:43210/auth?token=abc-DEF_123"
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{"windows", "rundll32.exe", []string{"url.dll,FileProtocolHandler", u}},
		{"darwin", "open", []string{u}},
		{"linux", "xdg-open", []string{u}},
		{"freebsd", "xdg-open", []string{u}},
		{"openbsd", "xdg-open", []string{u}},
		{"netbsd", "xdg-open", []string{u}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := browserCommand(tt.goos, u)
			if err != nil {
				t.Fatal(err)
			}
			if name != tt.name || !slices.Equal(args, tt.args) {
				t.Errorf("got %q %q", name, args)
			}
			// Never a shell, never "start" or "cmd".
			for _, bad := range []string{"sh", "bash", "cmd", "cmd.exe", "powershell", "powershell.exe", "start"} {
				if name == bad {
					t.Errorf("uses %s", bad)
				}
			}
		})
	}
	if _, _, err := browserCommand("plan9", u); err == nil {
		t.Error("unknown OS accepted")
	}
}

func TestOnlyLocalURLsAreOpened(t *testing.T) {
	for _, u := range []string{
		"", "http://127.0.0.1", "http://127.0.0.1:0/", "http://127.0.0.1:99999/",
		"https://127.0.0.1:8080/", "http://localhost:8080/", "http://[::1]:8080/", "http://0.0.0.0:8080/",
		"http://example.com:8080/", "http://127.0.0.1.evil.com:8080/", "http://user@127.0.0.1:8080/",
		"http://user:pw@127.0.0.1:8080/", "file:///etc/passwd", "javascript:alert(1)", "-flag", "calc.exe",
		"http://127.0.0.1:8080/ x", "http://127.0.0.1:8080/\nx", "http://127.0.0.1:8080/#frag",
		"http://127.0.0.1:8080/\x00", "http://127.0.0.1:abc/", "http:127.0.0.1:8080", "//127.0.0.1:8080/",
		"http://127.0.0.1:8080@evil.com/", "http://evil.com#@127.0.0.1:8080/",
	} {
		for _, goos := range []string{"windows", "darwin", "linux"} {
			if _, _, err := browserCommand(goos, u); err == nil {
				t.Errorf("%s: %q accepted", goos, u)
			}
		}
	}
	for _, u := range []string{"http://127.0.0.1:8080/", "http://127.0.0.1:1/auth?token=x", "http://127.0.0.1:65535/"} {
		if _, _, err := browserCommand("linux", u); err != nil {
			t.Errorf("%q: %v", u, err)
		}
	}
}
