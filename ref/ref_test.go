package ref

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/outline"
)

func revs(known ...string) func(string) bool {
	return func(s string) bool {
		for _, k := range known {
			if s == k {
				return true
			}
		}
		return false
	}
}

func TestParse(t *testing.T) {
	isRev := revs("b681ef1", "HEAD~2", "main")
	for _, c := range []struct {
		in   string
		want Ref
	}{
		{"internal/files/files.go", Ref{Path: "internal/files/files.go"}},
		{"internal/files/files.go:List", Ref{Path: "internal/files/files.go", Name: "List", HasTarget: true}},
		{"files.go:globSet.match", Ref{Path: "files.go", Name: "globSet.match", HasTarget: true}},
		{"README.md:For coding agents", Ref{Path: "README.md", Name: "For coding agents", HasTarget: true}},
		{"notes.md:Step 1: setup", Ref{Path: "notes.md", Name: "Step 1: setup", HasTarget: true}},
		{"main.go:104", Ref{Path: "main.go", Line: 104, HasTarget: true}},
		{"main.go:104:12", Ref{Path: "main.go", Line: 104, Col: 12, HasTarget: true}},
		{"main.go:81-200", Ref{Path: "main.go", Line: 81, End: 200, HasTarget: true}},
		{"main.go:200-81", Ref{Path: "main.go", Line: 81, End: 200, HasTarget: true}},
		{"main.go:0", Ref{Path: "main.go", Name: "0", HasTarget: true}},
		{"main.go:run@b681ef1", Ref{Path: "main.go", Name: "run", Rev: "b681ef1", HasTarget: true}},
		{"main.go@HEAD~2", Ref{Path: "main.go", Rev: "HEAD~2"}},
		{"main.go:104@main", Ref{Path: "main.go", Line: 104, Rev: "main", HasTarget: true}},
		{"README.md:Contact @me", Ref{Path: "README.md", Name: "Contact @me", HasTarget: true}},
		{"node_modules/@types/x.d.ts:f", Ref{Path: "node_modules/@types/x.d.ts", Name: "f", HasTarget: true}},
		{"lib.cpp:parse#2", Ref{Path: "lib.cpp", Name: "parse", Ordinal: 2, HasTarget: true}},
		{"lib.cpp:parse#0", Ref{Path: "lib.cpp", Name: "parse#0", HasTarget: true}},
		{"notes.md:C# support", Ref{Path: "notes.md", Name: "C# support", HasTarget: true}},
		{"lib.cpp:Foo::bar", Ref{Path: "lib.cpp", Name: "Foo::bar", HasTarget: true}},
		{"List", Ref{Path: "List"}},
		{"files.go:", Ref{Path: "files.go", HasTarget: true}},
		{"  files.go:List  ", Ref{Path: "files.go", Name: "List", HasTarget: true}},
	} {
		got := Parse(c.in, isRev)
		got.body = ""
		if got != c.want {
			t.Errorf("Parse(%q)\n got  %+v\n want %+v", c.in, got, c.want)
		}
	}
	// without isRev, '@' is never a revision
	if r := Parse("main.go:run@b681ef1", nil); r.Name != "run@b681ef1" || r.Rev != "" {
		t.Errorf("Parse without isRev: %+v", r)
	}
}

func TestAsName(t *testing.T) {
	for _, c := range []struct {
		in, name string
		ordinal  int
		rev      string
	}{
		{"List", "List", 0, ""},
		{"Repo.Open", "Repo.Open", 0, ""},
		{"Foo::bar", "Foo::bar", 0, ""},
		{"parse#2", "parse", 2, ""},
		{"run@main", "run", 0, "main"},
	} {
		r := Parse(c.in, revs("main")).AsName()
		if r.Path != "" || r.Name != c.name || r.Ordinal != c.ordinal || r.Rev != c.rev || !r.HasTarget {
			t.Errorf("AsName(%q) = %+v", c.in, r)
		}
	}
}

func TestString(t *testing.T) {
	isRev := revs("abc123")
	for _, s := range []string{
		"a.go", "a.go:List", "a.go:Repo.Open", "a.go:12", "a.go:12:3", "a.go:12-40",
		"a.go:run@abc123", "a.go@abc123", "lib.cpp:parse#2", "README.md:Step 1: setup", "notes.md:C# support",
	} {
		if got := Parse(s, isRev).String(); got != s {
			t.Errorf("String(Parse(%q)) = %q", s, got)
		}
	}
	if got := Parse("List", nil).AsName().String(); got != "List" {
		t.Errorf("bare name String = %q", got)
	}
	for r, want := range map[Ref]string{
		File("a.go"):              "a.go",
		Symbol("a.go", "Open", 0): "a.go:Open",
		Symbol("a.go", "init", 2): "a.go:init#2",
		Location("a.go", 7, 0):    "a.go:7",
		Location("a.go", 7, 3):    "a.go:7:3",
	} {
		if got := r.String(); got != want {
			t.Errorf("%+v.String() = %q, want %q", r, got, want)
		}
	}
}

// targets for Match tests: two files, overloads, a Rust type with impls,
// nested symbols
var sample = []Target{
	{"a.go", "Repo", "type", 3, 5},            // 0
	{"a.go", "Repo.Open", "func", 7, 20},      // 1
	{"a.go", "File.Open", "func", 22, 30},     // 2
	{"a.go", "init", "func", 32, 32},          // 3
	{"a.go", "init", "func", 34, 34},          // 4
	{"b.go", "Repo.Open", "func", 1, 9},       // 5
	{"s.rs", "Circle", "struct", 1, 3},        // 6
	{"s.rs", "Circle", "impl", 5, 9},          // 7
	{"s.rs", "Circle.area", "fn", 6, 8},       // 8
	{"n.md", "Issue #12", "h2", 1, 2},         // 9
	{"n.md", "Usage", "h2", 3, 4},             // 10
	{"n.md", "Usage", "h2", 5, 6},             // 11
	{"p.py", "outer", "def", 1, 10},           // 12
	{"p.py", "outer.inner", "def", 2, 5},      // 13
	{"p.py", "outer.inner.deep", "def", 3, 4}, // 14
}

func TestMatch(t *testing.T) {
	for _, c := range []struct {
		ref   string
		files []string
		want  []int
	}{
		{"a.go:Repo.Open", []string{"a.go"}, []int{1}},
		{"Repo.Open", nil, []int{1, 5}},               // bare name in two files: ambiguous
		{"Open", []string{"a.go"}, []int{1, 2}},       // suffix: two different symbols
		{"Open", []string{"b.go"}, []int{5}},          // suffix, one file
		{"a.go:Repo", []string{"a.go"}, []int{0}},     // exact beats suffix
		{"a.go:init", []string{"a.go"}, []int{3, 4}},  // duplicates without ordinal
		{"a.go:init#2", []string{"a.go"}, []int{4}},   // ordinal
		{"a.go:init#3", []string{"a.go"}, nil},        // no third
		{"s.rs:Circle", []string{"s.rs"}, []int{6}},   // the type beats its impl
		{"s.rs:Circle#2", []string{"s.rs"}, []int{7}}, // the impl by ordinal
		{"s.rs:area", []string{"s.rs"}, []int{8}},
		{"n.md:Issue #12", []string{"n.md"}, []int{9}}, // "#12" is part of the name
		{"n.md:Usage#2", []string{"n.md"}, []int{11}},
		{"p.py:inner", []string{"p.py"}, []int{13}}, // suffix at a '.' boundary
		{"p.py:inner.deep", []string{"p.py"}, []int{14}},
		{"p.py:ner", []string{"p.py"}, nil},     // not at a boundary
		{"p.py:4", []string{"p.py"}, []int{14}}, // innermost containing line 4
		{"p.py:7", []string{"p.py"}, []int{12}},
		{"p.py:11", []string{"p.py"}, nil}, // outside every definition
		{"a.go:7", []string{"a.go"}, []int{1}},
		{"a.go:Repo.Open", []string{}, nil}, // no files resolved
		{"a.go", []string{"a.go"}, nil},     // a file names no symbol
	} {
		r := Parse(c.ref, nil)
		if !r.HasTarget {
			r = r.AsName()
			if strings.HasSuffix(c.ref, ".go") {
				r = Parse(c.ref, nil) // a file stays a file
			}
		}
		if got := Match(r, sample, c.files); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q, %v) = %v, want %v", c.ref, c.files, got, c.want)
		}
	}
}

func TestOrdinals(t *testing.T) {
	want := []int{0, 0, 0, 1, 2, 0, 0, 2, 0, 0, 1, 2, 0, 0, 0}
	if got := Ordinals(sample); !reflect.DeepEqual(got, want) {
		t.Errorf("Ordinals = %v\n want     %v", got, want)
	}
}

func TestInnermost(t *testing.T) {
	for line, want := range map[int]int{1: 12, 2: 13, 3: 14, 4: 14, 5: 13, 10: 12, 11: -1} {
		if got := Innermost(sample, "p.py", line); got != want {
			t.Errorf("Innermost(p.py, %d) = %d, want %d", line, got, want)
		}
	}
}

// TestRoundTrip checks the promise of docs/refs.md: every symbol outline
// finds has a ref that leads back to exactly that symbol. It runs over the
// fixtures, this module's own files, and the directories in PRISM_CORPUS
// (separated by ':'), for checking real repositories locally.
func TestRoundTrip(t *testing.T) {
	roots := []string{"testdata", ".."}
	if c := os.Getenv("PRISM_CORPUS"); c != "" {
		roots = append(roots, strings.Split(c, ":")...)
	}
	files, symbols := 0, 0
	for _, root := range roots {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".git", "node_modules", "target", "vendor", "dist", "build", ".godot":
					return filepath.SkipDir
				}
				return nil
			}
			if !outline.Supported(p) || strings.Contains(p, ":") {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			files++
			symbols += roundTrip(t, filepath.ToSlash(p), data)
			return nil
		})
	}
	if files == 0 || symbols == 0 {
		t.Fatal("round trip saw no symbols")
	}
	t.Logf("%d symbols in %d files", symbols, files)
}

func roundTrip(t *testing.T, path string, data []byte) int {
	t.Helper()
	ts := Targets(path, outline.Parse(path, data, outline.MaxSize))
	n := 0
	for i, r := range Refs(ts) {
		if ts[i].Name == "" || locationRe.MatchString(ts[i].Name) {
			continue // accepted gap: a heading made of digits reads as a line
		}
		n++
		s := r.String()
		if got := Match(Parse(s, nil), ts, []string{path}); len(got) != 1 || got[0] != i {
			t.Errorf("%s (%s, line %d) leads to %v, want [%d]", s, ts[i].Kind, ts[i].Line, got, i)
		}
	}
	return n
}
