package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/IvanM0206/bearings/cli/internal/mcpserver"
)

// lockedBuffer is a bytes.Buffer safe for the server goroutine and the test.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// startMCP runs `bearings mcp args...` over real OS pipes, as a client
// would launch it, and connects the SDK client to it over stdio.
func startMCP(t *testing.T, dir string, args ...string) (*mcp.ClientSession, *lockedBuffer, <-chan int) {
	t.Helper()
	inR, inW, err := os.Pipe()
	must(t, err)
	outR, outW, err := os.Pipe()
	must(t, err)
	stderr := &lockedBuffer{}
	done := make(chan int, 1)
	go func() {
		code := Execute(append([]string{"mcp", "--notebook", dir}, args...), Env{
			Stdin: inR, Stdout: outW, Stderr: stderr,
			Getenv: func(string) string { return "" },
			Now:    func() time.Time { return fixtureNow },
		})
		_ = outW.Close()
		_ = inR.Close()
		done <- code
	}()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil).
		Connect(context.Background(), &mcp.IOTransport{Reader: outR, Writer: inW}, nil)
	must(t, err)
	return cs, stderr, done
}

func waitExit(t *testing.T, cs *mcp.ClientSession, done <-chan int) {
	t.Helper()
	_ = cs.Close()
	select {
	case code := <-done:
		if code != ExitOK {
			t.Fatalf("bearings mcp exited with %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("bearings mcp did not exit after the client closed stdin")
	}
}

func listToolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	must(t, err)
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func TestMCPOverStdioReadOnly(t *testing.T) {
	dir := newFixture(t)
	cs, stderr, done := startMCP(t, dir, "--log-level", "debug")
	names := listToolNames(t, cs)
	if len(names) != 7 || slices.Contains(names, mcpserver.ToolAddJournalEntry) {
		t.Fatalf("tools %v", names)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpserver.ToolGetTopic, Arguments: map[string]any{"topic": "technical/scratch"}})
	must(t, err)
	if res.IsError {
		t.Fatalf("get_topic failed: %+v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	if !strings.Contains(string(raw), "Secret handshake.") {
		t.Fatalf("result %s", raw)
	}
	waitExit(t, cs, done)
	logs := stderr.String()
	if !strings.Contains(logs, "tool=get_topic") || strings.Contains(logs, "Secret handshake") {
		t.Fatalf("logs %q", logs)
	}
}

func TestMCPOverStdioAllowWrite(t *testing.T) {
	dir := newFixture(t)
	cs, _, done := startMCP(t, dir, "--allow-write")
	if names := listToolNames(t, cs); len(names) != 11 || !slices.Contains(names, mcpserver.ToolAppendTopicNotes) {
		t.Fatalf("tools %v", names)
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: mcpserver.ToolSetTopicStatus,
		Arguments: map[string]any{"topic": "team/rituals", "status": "in-progress"}})
	must(t, err)
	if res.IsError {
		t.Fatalf("set_topic_status failed: %+v", res.Content)
	}
	waitExit(t, cs, done)
	if r := run(t, dir, "show", "team/rituals"); !strings.Contains(r.stdout, "in progress") {
		t.Fatalf("status not written:\n%s", r.stdout)
	}
}

func TestMCPCommandErrors(t *testing.T) {
	dir := newFixture(t)
	if r := run(t, dir, "mcp", "--log-level", "loud"); r.code != ExitUsage || !strings.Contains(r.stderr, "log level") {
		t.Fatalf("bad log level: %+v", r)
	}
	if r := run(t, t.TempDir(), "mcp"); r.code != ExitNotebook || r.stdout != "" {
		t.Fatalf("no notebook: %+v", r)
	}
	// With stdin already closed the server ends at once, writing nothing.
	var out bytes.Buffer
	code := Execute([]string{"mcp", "--notebook", dir}, Env{Stdin: io.LimitReader(nil, 0), Stdout: &out, Stderr: io.Discard, Getenv: func(string) string { return "" }})
	if code != ExitOK || out.Len() != 0 {
		t.Fatalf("closed stdin: code %d, stdout %q", code, out.String())
	}
}
