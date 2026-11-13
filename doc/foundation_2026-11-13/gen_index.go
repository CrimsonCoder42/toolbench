//go:build ignore

// gen_index regenerates INDEX.md from the code, the way testing/index_test.go reads it (the mechanism of
// calcbench D-070): every ID'd declaration's doc comment gives Pattern, Goroutines, Authority and Tags (repeated
// labels joined by "; ", "-" for a label that is absent), the package doc.go line gives Does, a literal "|" is
// written "\|", REMOVED rows are kept as they stand, rows sorted by ID. The preamble (everything up to the
// table's |---| line) is kept unchanged. internal/, compare/, testing/, doc/, the root package and folders
// starting with "_" or "." are outside the index.
//
// Run from the repo root: go run doc/foundation_2026-11-13/gen_index.go <out path>
// then compare <out path> with INDEX.md and move it into place.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	idRe       = regexp.MustCompile(`^TB-([A-Z0-9]{1,8})-([0-9]{3})$`)
	labelRe    = regexp.MustCompile(`^([A-Z][A-Za-z]*):\s+(.*)$`)
	docEntryRe = regexp.MustCompile(`^\t(TB-[A-Z0-9]{1,8}-[0-9]{3})\s+(\S+)\s*(.*)$`)
	labels     = map[string]bool{"ID": true, "Pattern": true, "Authority": true, "Goroutines": true, "Cancel": true,
		"Errors": true, "Blocking": true, "Math": true, "Tags": true}
	outside = map[string]bool{"internal": true, "compare": true, "testing": true, "doc": true}
)

func parseFields(doc string) map[string][]string {
	f := map[string][]string{}
	cur := ""
	for _, ln := range strings.Split(doc, "\n") {
		if m := labelRe.FindStringSubmatch(ln); m != nil && labels[m[1]] {
			cur = m[1]
			f[cur] = append(f[cur], strings.TrimSpace(m[2]))
			continue
		}
		if cur != "" && strings.HasPrefix(ln, "  ") && strings.TrimSpace(ln) != "" {
			l := f[cur]
			l[len(l)-1] += " " + strings.TrimSpace(ln)
			continue
		}
		cur = ""
	}
	return f
}

func receiverType(fd *ast.FuncDecl) string {
	t := fd.Recv.List[0].Type
	if s, ok := t.(*ast.StarExpr); ok {
		t = s.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func skipDir(rel string) bool {
	if rel == "." {
		return false
	}
	parts := strings.Split(rel, "/")
	if outside[parts[0]] {
		return true
	}
	for _, p := range parts {
		if strings.HasPrefix(p, "_") || strings.HasPrefix(p, ".") || p == "testdata" || p == "vendor" || p == "node_modules" {
			return true
		}
	}
	return false
}

type entry struct {
	id, name, pkg string
	f             map[string][]string
}

func esc(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gen_index.go <out path>")
		os.Exit(2)
	}
	fset := token.NewFileSet()
	var entries []entry
	docLine := map[string]string{}
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(p)
		if d.IsDir() {
			if skipDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			return nil
		}
		af, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		if filepath.Base(p) == "doc.go" && af.Doc != nil {
			for _, ln := range strings.Split(af.Doc.Text(), "\n") {
				if m := docEntryRe.FindStringSubmatch(ln); m != nil {
					docLine[m[1]] = strings.TrimSpace(m[3])
				}
			}
		}
		add := func(doc *ast.CommentGroup, name string) {
			if doc == nil {
				return
			}
			f := parseFields(doc.Text())
			if len(f["ID"]) == 0 {
				return
			}
			entries = append(entries, entry{f["ID"][0], name, pkg, f})
		}
		for _, decl := range af.Decls {
			switch dcl := decl.(type) {
			case *ast.GenDecl:
				for _, sp := range dcl.Specs {
					var doc *ast.CommentGroup
					var name string
					switch s := sp.(type) {
					case *ast.TypeSpec:
						doc, name = s.Doc, s.Name.Name
					case *ast.ValueSpec:
						doc, name = s.Doc, s.Names[0].Name
					default:
						continue
					}
					if !ast.IsExported(name) {
						continue
					}
					if doc == nil && len(dcl.Specs) == 1 {
						doc = dcl.Doc
					}
					add(doc, name)
				}
			case *ast.FuncDecl:
				if !dcl.Name.IsExported() {
					continue
				}
				name := dcl.Name.Name
				if dcl.Recv != nil {
					r := receiverType(dcl)
					if !ast.IsExported(r) {
						continue
					}
					name = r + "." + name
				}
				add(dcl.Doc, name)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	old, err := os.ReadFile("INDEX.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var preamble []string
	rows := map[string]string{}
	live := map[string]bool{} // IDs with a live row before regenerating
	inTable := false
	for _, ln := range strings.Split(strings.TrimRight(string(old), "\n"), "\n") {
		if !inTable {
			preamble = append(preamble, ln)
			if strings.HasPrefix(ln, "|---|") {
				inTable = true
			}
			continue
		}
		cells := strings.SplitN(strings.TrimPrefix(ln, "| "), " |", 2)
		if len(cells) == 2 && idRe.MatchString(cells[0]) {
			if strings.Contains(ln, "REMOVED") {
				rows[cells[0]] = ln
			} else {
				live[cells[0]] = true
			}
		}
	}
	code := map[string]bool{}
	for _, e := range entries {
		if code[e.id] {
			fmt.Fprintln(os.Stderr, "duplicate ID in code:", e.id)
			os.Exit(1)
		}
		code[e.id] = true
		if _, ok := rows[e.id]; ok {
			fmt.Fprintln(os.Stderr, "REMOVED ID is back in code (IDs are never reused):", e.id)
			os.Exit(1)
		}
		j := func(l string) string {
			if len(e.f[l]) == 0 {
				return "-"
			}
			return esc(strings.Join(e.f[l], "; "))
		}
		rows[e.id] = fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |", e.id, esc(e.name), e.pkg,
			j("Pattern"), esc(docLine[e.id]), j("Goroutines"), j("Authority"), j("Tags"))
	}
	for id := range live {
		if !code[id] {
			fmt.Fprintln(os.Stderr, "INDEX.md row has no code; mark it REMOVED with the replacement's ID by hand first:", id)
			os.Exit(1)
		}
	}
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString(strings.Join(preamble, "\n") + "\n")
	for _, id := range ids {
		b.WriteString(rows[id] + "\n")
	}
	if err := os.WriteFile(os.Args[1], []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("code IDs %d, rows %d (%d REMOVED)\n", len(entries), len(ids), len(ids)-len(entries))
}
