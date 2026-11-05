//go:build unix

package notebook

import (
	"syscall"
	"testing"
)

func TestSpecialFilesAreSkipped(t *testing.T) {
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	fifo := h.path("sections/tech/pipe.md")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	if ts := must[[]Topic](t)(h.nb.Topics("tech")); len(ts) != 0 {
		t.Fatal(ts)
	}
	if c := h.codes(); len(c) != 1 || c[0] != CodeNotRegular {
		t.Fatalf("%v", c)
	}
	if _, err := h.nb.Topic(TopicRef{"tech", "pipe"}); CodeOf(err) != CodeNotRegular {
		t.Fatalf("direct read of a FIFO: %v", err)
	}
	// A section "directory" that is a regular file is not a section.
	h.write(t, "sections/file", "x")
	if secs := must[[]Section](t)(h.nb.Sections()); len(secs) != 1 {
		t.Fatal(secs)
	}
}
