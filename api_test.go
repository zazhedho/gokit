package gokit

import (
	"flag"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var updateAPI = flag.Bool("update", false, "rewrite testdata/public_api.golden from the current exported API")

const apiGolden = "testdata/public_api.golden"

// Module-internal packages print by bare type name, so moving or renaming an
// internal package does not change the snapshot; only the public shape does.
func apiQualifier(p *types.Package) string {
	if p.Path() == "github.com/zazhedho/gokit" || strings.HasPrefix(p.Path(), "github.com/zazhedho/gokit/internal/") {
		return ""
	}
	return p.Path()
}

func apiSnapshot(t *testing.T) string {
	t.Helper()
	bp, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("list root package files: %v", err)
	}
	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(bp.GoFiles))
	for _, name := range bp.GoFiles {
		f, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("github.com/zazhedho/gokit", fset, files, nil)
	if err != nil {
		t.Fatalf("type-check root package: %v", err)
	}

	var b strings.Builder
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		switch o := obj.(type) {
		case *types.Func:
			b.WriteString("func " + name + strings.TrimPrefix(types.TypeString(o.Type(), apiQualifier), "func") + "\n")
		case *types.Const:
			b.WriteString("const " + name + " " + types.TypeString(o.Type(), apiQualifier) + " = " + o.Val().ExactString() + "\n")
		case *types.Var:
			b.WriteString("var " + name + " " + types.TypeString(o.Type(), apiQualifier) + "\n")
		case *types.TypeName:
			writeAPIType(&b, o)
		}
	}
	writeReachableTypes(&b, pkg)
	return b.String()
}

func isModuleInternal(p *types.Package) bool {
	return p != nil && strings.HasPrefix(p.Path(), "github.com/zazhedho/gokit/internal/")
}

// reachableTypes returns the named types of the module's internal packages that
// a caller can reach from the exported API without naming them: through
// signatures, struct fields, interface methods and the methods of those types.
// Types the root package declares or aliases are printed with the root API
// already, so they are left out. Types of other modules are not followed.
func reachableTypes(pkg *types.Package) []*types.TypeName {
	printed := map[*types.TypeName]bool{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		if tn, ok := scope.Lookup(name).(*types.TypeName); ok && tn.Exported() {
			if named, ok := types.Unalias(tn.Type()).(*types.Named); ok {
				printed[named.Obj()] = true
			}
		}
	}

	seen := map[types.Type]bool{}
	found := map[*types.TypeName]bool{}
	var visit func(t types.Type)
	visitSig := func(sig *types.Signature) {
		for i := 0; i < sig.Params().Len(); i++ {
			visit(sig.Params().At(i).Type())
		}
		for i := 0; i < sig.Results().Len(); i++ {
			visit(sig.Results().At(i).Type())
		}
	}
	visit = func(t types.Type) {
		t = types.Unalias(t)
		if seen[t] {
			return
		}
		seen[t] = true
		switch u := t.(type) {
		case *types.Pointer:
			visit(u.Elem())
		case *types.Slice:
			visit(u.Elem())
		case *types.Array:
			visit(u.Elem())
		case *types.Chan:
			visit(u.Elem())
		case *types.Map:
			visit(u.Key())
			visit(u.Elem())
		case *types.Signature:
			visitSig(u)
		case *types.Struct:
			for i := 0; i < u.NumFields(); i++ {
				if f := u.Field(i); f.Exported() {
					visit(f.Type())
				}
			}
		case *types.Interface:
			for i := 0; i < u.NumMethods(); i++ {
				if m := u.Method(i); m.Exported() {
					visit(m.Type())
				}
			}
		case *types.Named:
			if op := u.Obj().Pkg(); op == nil || (!isModuleInternal(op) && op != pkg) {
				return
			}
			if isModuleInternal(u.Obj().Pkg()) && !printed[u.Obj()] {
				found[u.Obj()] = true
			}
			visit(u.Underlying())
			ms := types.NewMethodSet(types.NewPointer(u))
			for i := 0; i < ms.Len(); i++ {
				if m := ms.At(i).Obj(); m.Exported() {
					visit(m.Type())
				}
			}
		}
	}
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj.Exported() {
			visit(obj.Type())
		}
	}

	out := make([]*types.TypeName, 0, len(found))
	for tn := range found {
		out = append(out, tn)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name() != out[j].Name() {
			return out[i].Name() < out[j].Name()
		}
		return out[i].Pkg().Path() < out[j].Pkg().Path()
	})
	return out
}

func writeReachableTypes(b *strings.Builder, pkg *types.Package) {
	for _, tn := range reachableTypes(pkg) {
		if tn.Exported() {
			writeAPITypeAs(b, tn, "reachable type")
		}
	}
}

func writeAPIType(b *strings.Builder, o *types.TypeName) {
	writeAPITypeAs(b, o, "type")
}

func writeAPITypeAs(b *strings.Builder, o *types.TypeName, keyword string) {
	typ := o.Type()
	header := keyword + " " + o.Name()
	if o.IsAlias() {
		header += " alias"
	}
	under := typ.Underlying()
	switch u := under.(type) {
	case *types.Struct:
		b.WriteString(header + " struct\n")
		for i := 0; i < u.NumFields(); i++ {
			if f := u.Field(i); f.Exported() {
				b.WriteString("\tfield " + f.Name() + " " + types.TypeString(f.Type(), apiQualifier) + "\n")
				// A separate line keeps the field lines stable; a JSON-shape
				// change shows up as a changed tag line.
				if tag := u.Tag(i); tag != "" {
					b.WriteString("\ttag " + f.Name() + " " + tag + "\n")
				}
			}
		}
	case *types.Interface:
		b.WriteString(header + " interface\n")
	default:
		b.WriteString(header + " " + types.TypeString(under, apiQualifier) + "\n")
	}

	// The pointer method set is a superset of the value one, so mark pointer-only
	// methods to catch a receiver change.
	_, isIface := under.(*types.Interface)
	var lines []string
	if isIface {
		it := under.(*types.Interface)
		for i := 0; i < it.NumMethods(); i++ {
			if m := it.Method(i); m.Exported() {
				lines = append(lines, "\tmethod "+m.Name()+strings.TrimPrefix(types.TypeString(m.Type(), apiQualifier), "func"))
			}
		}
	} else {
		val := types.NewMethodSet(typ)
		ptr := types.NewMethodSet(types.NewPointer(typ))
		for i := 0; i < ptr.Len(); i++ {
			m := ptr.At(i).Obj()
			if !m.Exported() {
				continue
			}
			recv := "(T)"
			if val.Lookup(m.Pkg(), m.Name()) == nil {
				recv = "(*T)"
			}
			lines = append(lines, "\tmethod "+recv+" "+m.Name()+strings.TrimPrefix(types.TypeString(m.Type(), apiQualifier), "func"))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
}

// TestPublicAPI pins every exported identifier of the root package. Run
// `go test -run TestPublicAPI -update .` to accept a deliberate change.
func TestPublicAPI(t *testing.T) {
	got := apiSnapshot(t)
	if *updateAPI {
		if err := os.MkdirAll(filepath.Dir(apiGolden), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(apiGolden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(apiGolden)
	if err != nil {
		t.Fatalf("read golden (create it with -update): %v", err)
	}
	if string(want) != got {
		t.Errorf("public API changed; if intended, regenerate with -update.\n%s", apiDiff(string(want), got))
	}
}

// apiDiff returns the lines removed (-) and added (+) between want and got,
// in order, so a failure names the changed identifiers instead of dumping both.
func apiDiff(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out.WriteString("- " + a[i] + "\n")
			i++
		default:
			out.WriteString("+ " + b[j] + "\n")
			j++
		}
	}
	for ; i < len(a); i++ {
		out.WriteString("- " + a[i] + "\n")
	}
	for ; j < len(b); j++ {
		out.WriteString("+ " + b[j] + "\n")
	}
	return out.String()
}
