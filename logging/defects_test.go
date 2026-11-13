package logging_test

import (
	"bytes"
	"context"
	"embed"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/CrimsonCoder42/toolbench/internal/leak"
	"github.com/CrimsonCoder42/toolbench/logging"
)

// sources is this package's own Go files, embedded at compile time (no file is opened while testing).
//
//go:embed *.go
var sources embed.FS

// TestDefect_go19_SetLevelRace: go_19 changes a shared int level with no lock while loggers read it. Here the
// level is a slog.LevelVar the caller owns; every wrapper delegates Enabled, so a concurrent Set is race-free
// under -race and the final level takes effect.
func TestDefect_go19_SetLevelRace(t *testing.T) {
	leak.Verify(t)
	ctx := testCtx(t)
	var lv slog.LevelVar
	var buf bytes.Buffer // written only by the JSON handler (which serialises its writes); read after Wait
	h := logging.Context(logging.Redact(logging.Multi(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: &lv}))),
		func(context.Context) []slog.Attr { return nil })
	l := slog.New(h).With("a", 1).WithGroup("g")
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 500 {
			if i%2 == 0 {
				lv.Set(slog.LevelError)
			} else {
				lv.Set(slog.LevelDebug)
			}
		}
	})
	for range 4 {
		wg.Go(func() {
			for range 200 {
				l.InfoContext(ctx, "m")
			}
		})
	}
	wg.Wait()
	lv.Set(slog.LevelError)
	if h.Enabled(ctx, slog.LevelInfo) {
		t.Fatal("level change not seen")
	}
	before := len(lines(buf.String()))
	l.Info("dropped")
	l.Error("kept")
	if got := len(lines(buf.String())); got != before+1 {
		t.Fatalf("lines %d -> %d, want +1", before, got)
	}
}

// TestDefect_go19_NoGlobalsNoInit: go_19 builds a global logger in init and go_79 calls slog.SetDefault from
// library code. This package's non-test sources have no func init, no package-level var and no SetDefault, os,
// time.Now or time.Sleep call; and using it never changes slog.Default().
func TestDefect_go19_NoGlobalsNoInit(t *testing.T) {
	before := slog.Default().Handler()
	files, err := sources.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		name := f.Name()
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := sources.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for _, d := range file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "init" {
					t.Errorf("%s: func init", name)
				}
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					t.Errorf("%s: package-level var", name)
				}
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && (sel.Sel.Name == "SetDefault" || id.Name == "os" ||
					id.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Sleep")) {
					t.Errorf("%s: %s.%s", name, id.Name, sel.Sel.Name)
				}
			}
			return true
		})
	}
	if checked < 5 {
		t.Fatalf("checked %d files; the embed pattern missed sources", checked)
	}
	var rec logging.Recorder
	slog.New(logging.Context(logging.Redact(logging.Multi(&rec)), logging.Combine())).Info("m")
	if slog.Default().Handler() != before {
		t.Fatal("slog.Default changed")
	}
}

// TestExamples_NoSecretInOutput scans every // Output: block in the Examples for the secrets they pass in.
func TestExamples_NoSecretInOutput(t *testing.T) {
	src, err := sources.ReadFile("example_test.go")
	if err != nil {
		t.Fatal(err)
	}
	blocks := 0
	in := false
	for line := range strings.Lines(string(src)) {
		trim := strings.TrimSpace(line)
		switch {
		case trim == "// Output:":
			in, blocks = true, blocks+1
		case in && strings.HasPrefix(trim, "//"):
			for i, s := range exampleSecrets {
				if strings.Contains(trim, s) {
					t.Errorf("an example output line carries exampleSecrets[%d]: %s", i, withheld(trim))
				}
			}
		default:
			in = false
		}
	}
	if blocks < 10 {
		t.Fatalf("found %d Output blocks, want 10", blocks)
	}
}
