package archive

import (
	"bytes"
	"testing"
)

// FuzzReadArchive feeds arbitrary bytes to the archive reader. It must
// never panic or hang, and anything it accepts must be a valid notebook
// that can be exported and read back unchanged.
func FuzzReadArchive(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("PK\x05\x06" + string(make([]byte, 18))))
	f.Add(buildRaw(withConfig()))
	f.Add(buildRaw(withConfig(stored("../x", "x"))))
	f.Add(buildRaw(withConfig(stored("sections/", ""), stored("README.md", "hello"))))
	f.Add(buildRaw(withConfig(deflated("notes.txt", make([]byte, 4096)))))
	e := stored("x.md", "x")
	e.flags = 1
	f.Add(buildRaw(withConfig(e)))
	first := deflated("a.txt", bytes.Repeat([]byte("ab"), 100))
	second := first
	second.name, second.shareWith = "b.txt", 2
	f.Add(buildRaw(withConfig(first, second)))
	f.Add(exportBytes(f, fixtureFiles(f), Options{}))
	f.Add(exportBytes(f, fixtureFiles(f), Options{RoleOnly: true}))

	f.Fuzz(func(t *testing.T, data []byte) {
		tree, err := Read(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return
		}
		files, err := tree.Files()
		if err != nil {
			t.Fatalf("accepted an archive whose notebook does not encode: %v", err)
		}
		again := exportBytes(t, files, Options{})
		tree2, err := readBytes(again)
		if err != nil {
			t.Fatalf("re-export not readable: %v", err)
		}
		files2, err := tree2.Files()
		if err != nil || len(files2) != len(files) {
			t.Fatalf("round trip changed the notebook: %v", err)
		}
		for p, d := range files {
			if !bytes.Equal(files2[p], d) {
				t.Fatalf("round trip changed %s", p)
			}
		}
	})
}
