package cmd

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		escapes bool
		want    []string
		wantErr bool
	}{
		{"simple", "vim", true, []string{"vim"}, false},
		{"flags", "code --wait -n", true, []string{"code", "--wait", "-n"}, false},
		{"extra spaces and tabs", "  nano \t -w  ", true, []string{"nano", "-w"}, false},
		{"double quotes with spaces", `"/opt/my editor/ed" -w`, true, []string{"/opt/my editor/ed", "-w"}, false},
		{"single quotes literal", `ed 'a "b" $HOME \n'`, true, []string{"ed", `a "b" $HOME \n`}, false},
		{"escaped quote in double quotes", `ed "a \"b\" c"`, true, []string{"ed", `a "b" c`}, false},
		{"escaped backslash in double quotes", `ed "a\\b"`, true, []string{"ed", `a\b`}, false},
		{"other backslash in double quotes is literal", `ed "a\nb"`, true, []string{"ed", `a\nb`}, false},
		{"backslash escapes space", `my\ editor -x`, true, []string{"my editor", "-x"}, false},
		{"backslash literal on windows", `C:\Tools\ed.exe -w`, false, []string{`C:\Tools\ed.exe`, "-w"}, false},
		{"empty quoted argument", `ed "" x`, true, []string{"ed", "", "x"}, false},
		{"adjacent quoting joins", `a"b c"'d e'f`, true, []string{"ab cd ef"}, false},
		{"no shell expansion", "ed $(rm -rf x) `id` ; | > &&", true, []string{"ed", "$(rm", "-rf", "x)", "`id`", ";", "|", ">", "&&"}, false},
		{"empty", "", true, nil, false},
		{"only spaces", "   ", true, nil, false},
		{"unterminated double", `ed "abc`, true, nil, true},
		{"unterminated single", `ed 'abc`, true, nil, true},
		{"trailing backslash is literal", `ed a\`, true, []string{"ed", `a\`}, false},
		{"unicode", `éditeur "ü ö"`, true, []string{"éditeur", "ü ö"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitCommand(tt.in, tt.escapes)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// quoteArg quotes s so that splitCommand returns it unchanged.
func quoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func FuzzSplitCommand(f *testing.F) {
	for _, s := range []string{"", "vim", `a "b c" 'd'`, `\`, `"`, `'`, "a\x00b", `"\\"`, "x\ty"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// Never panics, on any input and either escape mode.
		_, _ = splitCommand(s, true)
		_, _ = splitCommand(s, false)

		// Round trip: quoting each piece and splitting gives the pieces back.
		if !utf8.ValidString(s) {
			return
		}
		pieces := strings.Split(s, "\n")
		var q []string
		for _, p := range pieces {
			q = append(q, quoteArg(p))
		}
		got, err := splitCommand(strings.Join(q, " "), true)
		if err != nil || !reflect.DeepEqual(got, pieces) {
			t.Fatalf("round trip of %q: got %q, err %v", pieces, got, err)
		}
	})
}

func TestEditorArgs(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    []string
		wantErr string
	}{
		{"visual wins", map[string]string{"VISUAL": "code -w", "EDITOR": "nano"}, []string{"code", "-w"}, ""},
		{"editor", map[string]string{"EDITOR": `"/a b/ed"`}, []string{"/a b/ed"}, ""},
		{"blank visual falls back", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, []string{"nano"}, ""},
		{"none", map[string]string{}, nil, "no editor configured"},
		{"empty values", map[string]string{"VISUAL": "", "EDITOR": " "}, nil, "no editor configured"},
		{"bad quote", map[string]string{"EDITOR": `ed "x`}, nil, "$EDITOR is not a valid editor command"},
		{"empty program", map[string]string{"EDITOR": `""`}, nil, "$EDITOR is not a valid editor command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &app{env: Env{Getenv: func(k string) string { return tt.env[k] }}}
			a.initLanguage(nil)
			got, err := a.editorArgs()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if ee, ok := err.(*exitError); !ok || ee.code != ExitUsage {
					t.Errorf("want a usage error, got %v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestConfirm(t *testing.T) {
	tests := []struct {
		name        string
		yes         bool
		interactive bool
		stdin       string
		wantCode    int // -1: confirmed
		wantMsg     string
	}{
		{"yes flag skips the question", true, false, "", -1, ""},
		{"yes flag on a terminal", true, true, "", -1, ""},
		{"no terminal without --yes refuses", false, false, "y\n", ExitUsage, "needs confirmation"},
		{"terminal, y", false, true, "y\n", -1, ""},
		{"terminal, yes with spaces and case", false, true, "  YES \n", -1, ""},
		{"terminal, n", false, true, "n\n", ExitFailure, "cancelled"},
		{"terminal, empty answer", false, true, "\n", ExitFailure, "cancelled"},
		{"terminal, end of input", false, true, "", ExitFailure, "cancelled"},
		{"terminal, other text", false, true, "maybe\n", ExitFailure, "cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errb strings.Builder
			a := &app{env: Env{Stdin: strings.NewReader(tt.stdin), Stderr: &errb, Interactive: tt.interactive}}
			a.initLanguage(nil)
			err := a.confirm(tt.yes, "Delete the thing?")
			if tt.wantCode < 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				ee, ok := err.(*exitError)
				if !ok || ee.code != tt.wantCode || !strings.Contains(ee.msg, tt.wantMsg) {
					t.Fatalf("got %v, want code %d containing %q", err, tt.wantCode, tt.wantMsg)
				}
			}
			asked := strings.Contains(errb.String(), "Delete the thing? [y/N]")
			if asked != (tt.interactive && !tt.yes) {
				t.Errorf("asked = %v, prompt output %q", asked, errb.String())
			}
		})
	}
}
