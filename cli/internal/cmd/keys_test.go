package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/i18n"
)

// translatorFuncs are the functions whose first string literal argument is
// a message key.
var translatorFuncs = map[string]bool{"t": true, "T": true, "usagef": true, "notFoundf": true}

// TestEveryKeyUsedInCodeExistsInEnglish parses the command sources and
// checks that each message key written as a string literal in a call to a
// translation function exists in the English catalog, so a missing message
// is a test failure and never a raw key on screen.
func TestEveryKeyUsedInCodeExistsInEnglish(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no sources: %v", err)
	}
	fset := token.NewFileSet()
	used := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			var fn string
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				fn = fun.Sel.Name
			case *ast.Ident:
				fn = fun.Name
			}
			if !translatorFuncs[fn] {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				// Dynamic keys ("status." + s) are covered by the prefix test.
				if bin, ok := call.Args[0].(*ast.BinaryExpr); ok {
					if l, ok := bin.X.(*ast.BasicLit); ok {
						checkPrefix(t, fset.Position(l.Pos()), l)
					}
				}
				return true
			}
			key, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("%s: %v", fset.Position(lit.Pos()), err)
				return true
			}
			used++
			if !i18n.Has("en", key) {
				t.Errorf("%s: message key %q is missing from the en catalog", fset.Position(lit.Pos()), key)
			}
			return true
		})
	}
	if used < 50 {
		t.Fatalf("only %d keys found; the scanner is probably broken", used)
	}
}

// checkPrefix verifies that at least one key exists for a dynamic prefix.
func checkPrefix(t *testing.T, pos token.Position, lit *ast.BasicLit) {
	t.Helper()
	prefix, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	for _, k := range i18n.Keys("en") {
		if strings.HasPrefix(k, prefix) {
			return
		}
	}
	t.Errorf("%s: no message key starts with %q", pos, prefix)
}

// TestDynamicKeys checks the families of keys that are built at run time.
func TestDynamicKeys(t *testing.T) {
	for _, s := range []string{"pending", "in-progress", "mastered"} {
		if !i18n.Has("en", "status."+s) {
			t.Errorf("missing status.%s", s)
		}
	}
	for _, s := range []string{"role", "personal"} {
		if !i18n.Has("en", "type."+s) {
			t.Errorf("missing type.%s", s)
		}
	}
	for _, k := range []string{"section.title", "section.body", "topic.title", "topic.body", "topic.resources", "entry.body"} {
		if !i18n.Has("en", "search.in."+k) {
			t.Errorf("missing search.in.%s", k)
		}
	}
}

// TestEveryNotebookErrorCodeHasAMessage reads the error codes declared in
// the notebook package and checks each has a message.
func TestEveryNotebookErrorCodeHasAMessage(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "notebook", "errors.go"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "errors.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != "Code" || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			code, _ := strconv.Unquote(lit.Value)
			n++
			if !i18n.Has("en", "err.code."+code) {
				t.Errorf("no message for notebook error code %q", code)
			}
		}
	}
	if n < 30 {
		t.Fatalf("found only %d codes; the scanner is probably broken", n)
	}
}

func TestTerminalWidth(t *testing.T) {
	for _, tt := range []struct {
		cols string
		want int
	}{{"", 80}, {"120", 120}, {"abc", 80}, {"5", 80}, {"-3", 80}, {"99999", 80}} {
		got := terminalWidth(func(string) string { return tt.cols })
		if got != tt.want {
			t.Errorf("COLUMNS=%q: got %d, want %d", tt.cols, got, tt.want)
		}
	}
}

func TestNoWrapWhenNotTerminal(t *testing.T) {
	src := newFixture(t)
	long := strings.Repeat("word ", 40)
	wantOK(t, run(t, src, "log", strings.TrimSpace(long), "-s", "team"))
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": src, "COLUMNS": "40"}}, "journal")
	wantOK(t, r)
	if !strings.Contains(r.stdout, strings.TrimSpace(long)) {
		t.Errorf("output wrapped although not a terminal: %q", r.stdout)
	}
}
