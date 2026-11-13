// Package index_test keeps the three places that describe an export in agreement: the doc comment, the
// package doc.go and INDEX.md, plus doc/taxonomy.md against the tree (.claude/CLAUDE.md "Finding things",
// modelled on calcbench testing/index_test.go and its D-070, D-071, D-072). It reads files only; it imports no
// toolbench package. internal/, compare/, testing/, doc/ and folders starting with "_" or "." are outside the
// index; the root package holds no exports.
package index_test

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const root = ".."

// indexHeader is the INDEX.md header row (.claude/CLAUDE.md "Finding things").
const indexHeader = "| ID | Name | Package | Pattern | Does (one line) | Goroutines | Authority | Tags |"

// outside are the top-level folders that hold no indexed package.
var outside = map[string]bool{"internal": true, "compare": true, "testing": true, "doc": true}

// labelOrder is the order of the doc-comment labels (.claude/rules/go-functions.md). ID, Pattern, Authority and
// Tags are always present; the rest where they apply.
var labelOrder = []string{"ID", "Pattern", "Authority", "Goroutines", "Cancel", "Errors", "Blocking", "Math", "Tags"}

var (
	idRe       = regexp.MustCompile(`^TB-([A-Z0-9]{1,8})-([0-9]{3})$`)
	labelRe    = regexp.MustCompile(`^([A-Z][A-Za-z]*):\s+(.*)$`)
	docEntryRe = regexp.MustCompile(`^\t(TB-[A-Z0-9]{1,8}-[0-9]{3})\s+(\S+)\s*(.*)$`)
	shortRe    = regexp.MustCompile(`ID short form: ([A-Z0-9]+)`)
	anyLabelRe = regexp.MustCompile(`^([A-Z][A-Za-z]*):\s`)
	labels     = map[string]bool{"ID": true, "Pattern": true, "Authority": true, "Goroutines": true, "Cancel": true,
		"Errors": true, "Blocking": true, "Math": true, "Tags": true}
	always = []string{"ID", "Pattern", "Authority", "Tags"}
)

// entry is one ID'd declaration: a function, a method, a type, a constant or a variable.
type entry struct {
	pkg, file, name string
	line            int
	fields          map[string][]string
	order           []string
	unknown         []string // labels after ID: that go-functions.md does not define
}

func (e entry) where() string { return fmt.Sprintf("%s:%d %s", e.file, e.line, e.name) }

func (e entry) id() string {
	if v := e.fields["ID"]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// cell is the INDEX.md cell for a label: repeated lines joined by "; ", "-" when the label is absent.
func (e entry) cell(label string) string {
	if len(e.fields[label]) == 0 {
		return "-"
	}
	return strings.Join(e.fields[label], "; ")
}

// parseFields reads the labelled lines of a doc comment. A line indented by two or more spaces after a
// labelled line continues it (joined with one space).
func parseFields(doc string) (map[string][]string, []string) {
	f := map[string][]string{}
	var order []string
	cur := ""
	for _, ln := range strings.Split(doc, "\n") {
		if m := labelRe.FindStringSubmatch(ln); m != nil && labels[m[1]] {
			cur = m[1]
			f[cur] = append(f[cur], strings.TrimSpace(m[2]))
			order = append(order, cur)
			continue
		}
		if cur != "" && strings.HasPrefix(ln, "  ") && strings.TrimSpace(ln) != "" {
			l := f[cur]
			l[len(l)-1] += " " + strings.TrimSpace(ln)
			continue
		}
		cur = ""
	}
	return f, order
}

// unknownLabels returns the labels after the ID: line that are not in labelOrder (a misspelt "Goroutine:"
// would otherwise read as prose and never reach INDEX.md).
func unknownLabels(doc string) []string {
	var out []string
	seenID := false
	for _, ln := range strings.Split(doc, "\n") {
		m := anyLabelRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		if m[1] == "ID" {
			seenID = true
		} else if seenID && !labels[m[1]] {
			out = append(out, m[1])
		}
	}
	return out
}

// tree is everything the checks need, read once.
type tree struct {
	entries    []entry             // every ID'd declaration
	uncovered  []string            // exported declarations with no ID and no covering ID (calcbench D-071)
	rootExport []string            // exported declarations in the root package, which must hold none
	pkgs       map[string]bool     // indexed packages on disk (folders holding non-test .go files)
	short      map[string]string   // package → SHORT from doc.go
	docIDs     map[string][]string // package → IDs listed in doc.go
	docLine    map[string]string   // ID → doc.go "Does" text
	docName    map[string]string   // ID → doc.go name
	readErrors []string
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

// resultTypes names the types a function returns (pointer, slice and generic wrappers removed).
func resultTypes(fd *ast.FuncDecl) []string {
	var out []string
	if fd.Type.Results == nil {
		return out
	}
	for _, r := range fd.Type.Results.List {
		t := r.Type
		for {
			switch x := t.(type) {
			case *ast.StarExpr:
				t = x.X
				continue
			case *ast.IndexExpr:
				t = x.X
				continue
			case *ast.IndexListExpr:
				t = x.X
				continue
			case *ast.ArrayType:
				t = x.Elt
				continue
			}
			break
		}
		if id, ok := t.(*ast.Ident); ok {
			out = append(out, id.Name)
		}
	}
	return out
}

// skipDir reports whether a folder (path relative to root, slash-separated) is outside the index.
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

func readTree(t *testing.T) *tree {
	t.Helper()
	tr := &tree{pkgs: map[string]bool{}, short: map[string]string{}, docIDs: map[string][]string{},
		docLine: map[string]string{}, docName: map[string]string{}}
	fset := token.NewFileSet()
	type pending struct {
		pkg, recv, name, where string
		results                []string
	}
	var noID []pending
	idTypes := map[string]bool{}  // pkg.Type carrying an ID
	retTypes := map[string]bool{} // pkg.Type returned by an ID'd function or method
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
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
		af, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			tr.readErrors = append(tr.readErrors, err.Error())
			return nil
		}
		if pkg == "." {
			for _, decl := range af.Decls {
				switch dcl := decl.(type) {
				case *ast.FuncDecl:
					if dcl.Name.IsExported() {
						tr.rootExport = append(tr.rootExport, rel+" "+dcl.Name.Name)
					}
				case *ast.GenDecl:
					for _, sp := range dcl.Specs {
						switch s := sp.(type) {
						case *ast.TypeSpec:
							if s.Name.IsExported() {
								tr.rootExport = append(tr.rootExport, rel+" "+s.Name.Name)
							}
						case *ast.ValueSpec:
							for _, n := range s.Names {
								if n.IsExported() {
									tr.rootExport = append(tr.rootExport, rel+" "+n.Name)
								}
							}
						}
					}
				}
			}
			return nil
		}
		tr.pkgs[pkg] = true
		if filepath.Base(p) == "doc.go" && af.Doc != nil {
			for _, ln := range strings.Split(af.Doc.Text(), "\n") {
				if m := docEntryRe.FindStringSubmatch(ln); m != nil {
					tr.docIDs[pkg] = append(tr.docIDs[pkg], m[1])
					tr.docName[m[1]] = m[2]
					tr.docLine[m[1]] = strings.TrimSpace(m[3])
				}
				if m := shortRe.FindStringSubmatch(ln); m != nil {
					tr.short[pkg] = m[1]
				}
			}
		}
		for _, decl := range af.Decls {
			switch dcl := decl.(type) {
			case *ast.GenDecl:
				for _, sp := range dcl.Specs {
					var doc *ast.CommentGroup
					var name *ast.Ident
					switch s := sp.(type) {
					case *ast.TypeSpec:
						doc, name = s.Doc, s.Name
					case *ast.ValueSpec:
						doc, name = s.Doc, s.Names[0]
					default:
						continue
					}
					if !name.IsExported() {
						continue
					}
					if doc == nil && len(dcl.Specs) == 1 {
						doc = dcl.Doc
					}
					text := ""
					if doc != nil {
						text = doc.Text()
					}
					f, order := parseFields(text)
					if len(f["ID"]) == 0 {
						// An exported type needs an ID; constants and variables may share their group's
						// documentation (sentinel errors, enum values) and are not required to carry one.
						if _, ok := sp.(*ast.TypeSpec); ok {
							tr.uncovered = append(tr.uncovered,
								fmt.Sprintf("%s:%d %s", rel, fset.Position(name.Pos()).Line, name.Name))
						}
						continue
					}
					if _, ok := sp.(*ast.TypeSpec); ok {
						idTypes[pkg+"."+name.Name] = true
					}
					tr.entries = append(tr.entries, entry{pkg: pkg, file: rel, name: name.Name,
						line: fset.Position(name.Pos()).Line, fields: f, order: order, unknown: unknownLabels(text)})
				}
			case *ast.FuncDecl:
				if !dcl.Name.IsExported() {
					continue
				}
				name, recv := dcl.Name.Name, ""
				if dcl.Recv != nil {
					recv = receiverType(dcl)
					if !ast.IsExported(recv) {
						continue
					}
					name = recv + "." + name
				}
				doc := ""
				if dcl.Doc != nil {
					doc = dcl.Doc.Text()
				}
				f, order := parseFields(doc)
				where := fmt.Sprintf("%s:%d %s", rel, fset.Position(dcl.Pos()).Line, name)
				if len(f["ID"]) == 0 {
					noID = append(noID, pending{pkg, recv, dcl.Name.Name, where, resultTypes(dcl)})
					continue
				}
				for _, rt := range resultTypes(dcl) {
					retTypes[pkg+"."+rt] = true
				}
				tr.entries = append(tr.entries, entry{pkg: pkg, file: rel, name: name,
					line: fset.Position(dcl.Pos()).Line, fields: f, order: order, unknown: unknownLabels(doc)})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	// calcbench D-071: a method is covered by its receiver type's ID or by a covered or ID'd function returning
	// that type; a constructor New<T> is covered by T's ID. Repeat until nothing new is covered.
	covered := make([]bool, len(noID))
	for changed := true; changed; {
		changed = false
		for i, p := range noID {
			if covered[i] {
				continue
			}
			if (p.recv != "" && (idTypes[p.pkg+"."+p.recv] || retTypes[p.pkg+"."+p.recv])) ||
				(p.recv == "" && strings.HasPrefix(p.name, "New") && idTypes[p.pkg+"."+strings.TrimPrefix(p.name, "New")]) {
				covered[i], changed = true, true
				for _, rt := range p.results {
					retTypes[p.pkg+"."+rt] = true
				}
			}
		}
	}
	for i, p := range noID {
		if !covered[i] {
			tr.uncovered = append(tr.uncovered, p.where)
		}
	}
	slices.Sort(tr.uncovered)
	slices.SortFunc(tr.entries, func(a, b entry) int { return cmp.Compare(a.id(), b.id()) })
	return tr
}

// indexRow is one row of INDEX.md, cells unescaped.
type indexRow struct {
	line                                                      int
	id, name, pkg, pattern, does, goroutines, authority, tags string
}

func (r indexRow) removed() bool { return strings.Contains(r.does, "REMOVED") }

// splitRow splits a markdown table row on "|" not preceded by a backslash and unescapes "\|".
func splitRow(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	var cells []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '|':
			b.WriteByte('|')
			i++
		case s[i] == '|':
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(s[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

func readIndex(t *testing.T) []indexRow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "\n"+indexHeader+"\n") {
		t.Errorf("INDEX.md: header row %q not found", indexHeader)
	}
	var rows []indexRow
	for i, ln := range strings.Split(string(data), "\n") {
		c := splitRow(ln)
		if len(c) < 1 || !idRe.MatchString(c[0]) {
			if len(c) > 1 && strings.HasPrefix(strings.TrimLeft(c[0], "`*"), "TB-") {
				t.Errorf("INDEX.md:%d: first cell %q is not a plain TB-<SHORT>-<NNN> ID", i+1, c[0])
			}
			continue
		}
		if len(c) != 8 {
			t.Errorf("INDEX.md:%d: %d cells, want 8", i+1, len(c))
			continue
		}
		rows = append(rows, indexRow{i + 1, c[0], c[1], c[2], c[3], c[4], c[5], c[6], c[7]})
	}
	return rows
}

// report fails the test with every offender on its own line.
func report(t *testing.T, what string, bad []string) {
	t.Helper()
	if len(bad) > 0 {
		t.Errorf("%s (%d):\n  %s", what, len(bad), strings.Join(bad, "\n  "))
	}
}

// Every exported declaration has an ID in the documented shape, or is covered by one (calcbench D-071); the
// labels are present and in order; the root package exports nothing.
func TestIndex_docCommentShape(t *testing.T) {
	tr := readTree(t)
	report(t, "parse errors", tr.readErrors)
	report(t, "exported declarations with no ID", tr.uncovered)
	report(t, "exports in the root package, which holds none", tr.rootExport)
	rank := map[string]int{}
	for i, l := range labelOrder {
		rank[l] = i
	}
	var bad []string
	for _, e := range tr.entries {
		id := e.id()
		m := idRe.FindStringSubmatch(id)
		switch {
		case m == nil:
			bad = append(bad, e.where()+": ID "+id+" not TB-<SHORT>-<NNN>")
			continue
		case m[1] != tr.short[e.pkg]:
			bad = append(bad, e.where()+": SHORT "+m[1]+", doc.go says "+tr.short[e.pkg])
		}
		for _, l := range always {
			if len(e.fields[l]) == 0 || e.fields[l][0] == "" {
				bad = append(bad, e.where()+": missing "+l+":")
			}
		}
		for _, l := range labelOrder {
			for _, v := range e.fields[l] {
				if v == "" {
					bad = append(bad, e.where()+": empty "+l+":")
				}
			}
		}
		for i := 1; i < len(e.order); i++ {
			if a, b := e.order[i-1], e.order[i]; rank[b] < rank[a] {
				bad = append(bad, e.where()+": "+b+": after "+a+":")
			}
		}
		for _, l := range e.unknown {
			bad = append(bad, e.where()+": unknown label "+l+":")
		}
		for _, a := range e.fields["Authority"] {
			if strings.Contains(strings.ToLower(a), "code-heim") {
				bad = append(bad, e.where()+": Authority cites code-heim, a reading list only (sources.md)")
			}
		}
		for _, l := range []string{"ID", "Pattern", "Tags"} {
			if len(e.fields[l]) > 1 {
				bad = append(bad, e.where()+": more than one "+l+": line")
			}
		}
	}
	report(t, "doc comments out of shape", bad)
}

// No ID appears twice in code.
func TestIndex_noDuplicateIDs(t *testing.T) {
	tr := readTree(t)
	seen := map[string]string{}
	var bad []string
	for _, e := range tr.entries {
		if w, ok := seen[e.id()]; ok {
			bad = append(bad, e.id()+": "+w+" and "+e.where())
		}
		seen[e.id()] = e.where()
	}
	report(t, "IDs used twice in code", bad)
}

// Every ID is in INDEX.md exactly once, sorted, and the row matches the doc comment and doc.go word for word.
func TestIndex_indexAgreesWithCode(t *testing.T) {
	tr := readTree(t)
	rows := readIndex(t)
	byID := map[string][]indexRow{}
	for _, r := range rows {
		byID[r.id] = append(byID[r.id], r)
	}
	code := map[string]entry{}
	for _, e := range tr.entries {
		code[e.id()] = e
	}
	var missing, dup, stale, differ, unsorted []string
	for _, e := range tr.entries {
		switch n := len(byID[e.id()]); {
		case n == 0:
			missing = append(missing, e.id()+" "+e.where())
		case n > 1:
			dup = append(dup, e.id())
		}
	}
	for i, r := range rows {
		if i > 0 && rows[i-1].id >= r.id {
			unsorted = append(unsorted, fmt.Sprintf("INDEX.md:%d %s after %s", r.line, r.id, rows[i-1].id))
		}
		e, ok := code[r.id]
		if !ok {
			if !r.removed() {
				stale = append(stale, fmt.Sprintf("INDEX.md:%d %s (no code, not REMOVED)", r.line, r.id))
			}
			continue
		}
		if r.removed() {
			stale = append(stale, fmt.Sprintf("INDEX.md:%d %s is marked REMOVED but exists in code", r.line, r.id))
		}
		for _, c := range []struct{ col, got, want string }{
			{"Name", r.name, e.name}, {"Package", r.pkg, e.pkg}, {"Pattern", r.pattern, e.cell("Pattern")},
			{"Does", r.does, tr.docLine[r.id]}, {"Goroutines", r.goroutines, e.cell("Goroutines")},
			{"Authority", r.authority, e.cell("Authority")}, {"Tags", r.tags, e.cell("Tags")},
		} {
			if c.got != c.want {
				differ = append(differ, fmt.Sprintf("INDEX.md:%d %s %s: index %q, code %q", r.line, r.id, c.col, c.got, c.want))
			}
		}
	}
	report(t, "IDs missing from INDEX.md", missing)
	report(t, "IDs in INDEX.md more than once", dup)
	report(t, "INDEX.md rows with no code", stale)
	report(t, "INDEX.md cells that differ from the doc comment or doc.go", differ)
	report(t, "INDEX.md not sorted by ID", unsorted)
}

// Every ID is listed exactly once in its own package's doc.go, under the same name, and every package has a
// SHORT.
func TestIndex_docGoAgreesWithCode(t *testing.T) {
	tr := readTree(t)
	rows := readIndex(t)
	removed := map[string]bool{}
	for _, r := range rows {
		if r.removed() {
			removed[r.id] = true
		}
	}
	code := map[string]entry{}
	for _, e := range tr.entries {
		code[e.id()] = e
	}
	var bad []string
	for _, e := range tr.entries {
		n := 0
		for _, id := range tr.docIDs[e.pkg] {
			if id == e.id() {
				n++
			}
		}
		if n != 1 {
			bad = append(bad, fmt.Sprintf("%s listed %d times in %s/doc.go", e.id(), n, e.pkg))
		}
		if tr.docLine[e.id()] == "" {
			bad = append(bad, fmt.Sprintf("%s: empty Does text in %s/doc.go", e.id(), e.pkg))
		}
		if tr.docName[e.id()] != e.name {
			bad = append(bad, fmt.Sprintf("%s: doc.go names %q, code %q", e.id(), tr.docName[e.id()], e.name))
		}
	}
	for pkg, ids := range tr.docIDs {
		for _, id := range ids {
			if e, ok := code[id]; ok && e.pkg != pkg {
				bad = append(bad, fmt.Sprintf("%s listed in %s/doc.go but lives in %s", id, pkg, e.pkg))
			}
			if _, ok := code[id]; !ok && !removed[id] {
				bad = append(bad, fmt.Sprintf("%s listed in %s/doc.go: no code and no REMOVED row", id, pkg))
			}
		}
	}
	for pkg := range tr.pkgs {
		if tr.short[pkg] == "" {
			bad = append(bad, pkg+"/doc.go: no \"ID short form:\" line")
		}
	}
	slices.Sort(bad)
	report(t, "doc.go lists out of step with the code", bad)
}

// taxonomyRows reads doc/taxonomy.md: package → SHORT and Status, from the 7-column package table.
func taxonomyRows(t *testing.T) map[string][2]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "doc", "taxonomy.md"))
	if err != nil {
		t.Fatal(err)
	}
	pkgRe := regexp.MustCompile("^`?([a-z0-9][a-z0-9/]*)`?$")
	tax := map[string][2]string{}
	for _, ln := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(ln), "|") {
			continue
		}
		c := splitRow(ln)
		if len(c) != 7 {
			continue
		}
		m := pkgRe.FindStringSubmatch(c[0])
		if m == nil {
			continue
		}
		tax[m[1]] = [2]string{c[1], c[6]}
	}
	return tax
}

// Every package on disk is a built row of doc/taxonomy.md with the same SHORT, every built row exists, every
// status is built or planned, and no top-level folder exists without a row (calcbench D-072).
func TestIndex_taxonomyAgreesWithTree(t *testing.T) {
	tr := readTree(t)
	tax := taxonomyRows(t)
	var bad []string
	for pkg := range tr.pkgs {
		r, ok := tax[pkg]
		switch {
		case !ok:
			bad = append(bad, pkg+": on disk, not in doc/taxonomy.md")
		case r[1] != "built":
			bad = append(bad, pkg+": on disk, taxonomy status "+r[1])
		case r[0] != tr.short[pkg]:
			bad = append(bad, pkg+": taxonomy SHORT "+r[0]+", doc.go "+tr.short[pkg])
		}
	}
	for pkg, r := range tax {
		if r[1] == "built" && !tr.pkgs[pkg] {
			bad = append(bad, pkg+": taxonomy says built, no package on disk")
		}
		if r[1] != "built" && r[1] != "planned" {
			bad = append(bad, pkg+": taxonomy status "+r[1]+", want built or planned")
		}
	}
	// No folder without a row: every top-level folder (and every adapters/<driver>) that is not outside the
	// index is a taxonomy row, even before it holds Go files.
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() || skipDir(d.Name()) {
			continue
		}
		names := []string{d.Name()}
		if d.Name() == "adapters" {
			names = nil
			sub, err := os.ReadDir(filepath.Join(root, "adapters"))
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range sub {
				if s.IsDir() && !skipDir("adapters/"+s.Name()) {
					names = append(names, "adapters/"+s.Name())
				}
			}
		}
		for _, n := range names {
			if _, ok := tax[n]; !ok {
				bad = append(bad, n+"/: folder with no doc/taxonomy.md row")
			}
		}
	}
	slices.Sort(bad)
	report(t, "doc/taxonomy.md out of step with the tree", bad)
}

// TestIndex_counts logs the three counts the reports quote: exports | INDEX rows | doc.go entries.
func TestIndex_counts(t *testing.T) {
	tr := readTree(t)
	rows := readIndex(t)
	live := 0
	for _, r := range rows {
		if !r.removed() {
			live++
		}
	}
	docN := 0
	code := map[string]bool{}
	for _, e := range tr.entries {
		code[e.id()] = true
	}
	for _, ids := range tr.docIDs {
		for _, id := range ids {
			if code[id] {
				docN++
			}
		}
	}
	t.Logf("exports %d | INDEX rows %d (+%d REMOVED) | doc.go entries %d | packages %d",
		len(tr.entries), live, len(rows)-live, docN, len(tr.pkgs))
	if len(tr.entries) != live || live != docN {
		t.Errorf("counts differ: exports %d, INDEX %d, doc.go %d", len(tr.entries), live, docN)
	}
}
