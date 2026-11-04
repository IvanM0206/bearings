package output

import (
	"encoding/json"
	"io"
)

// JSON writes v as indented JSON followed by a newline. HTML characters are
// not escaped. Control characters inside strings are escaped by the
// encoder, so the output is safe to print to a terminal.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
