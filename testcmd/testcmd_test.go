package testcmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Vaspyyy/prism/ref"
)

func write(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIsTest(t *testing.T) {
	rs := []byte("mod tests {\n    #[test]\n    fn parses() {}\n\n    /// doc\n    #[tokio::test]\n    #[ignore]\n    async fn later() {}\n\n    fn helper() {}\n}\n")
	for _, c := range []struct {
		t    ref.Target
		src  []byte
		want bool
	}{
		{ref.Target{Path: "a/files_test.go", Name: "TestList", Kind: "func"}, nil, true},
		{ref.Target{Path: "a/files_test.go", Name: "Test", Kind: "func"}, nil, true},
		{ref.Target{Path: "a/files_test.go", Name: "Test_list", Kind: "func"}, nil, true},
		{ref.Target{Path: "a/files_test.go", Name: "Testify", Kind: "func"}, nil, false},
		{ref.Target{Path: "a/files_test.go", Name: "S.TestM", Kind: "func"}, nil, false},
		{ref.Target{Path: "a/files_test.go", Name: "ExampleList", Kind: "func"}, nil, true},
		{ref.Target{Path: "a/files.go", Name: "TestList", Kind: "func"}, nil, false},
		{ref.Target{Path: "tests/test_shop.py", Name: "TestShop.TestInner.test_deep", Kind: "def"}, nil, true},
		{ref.Target{Path: "tests/shop_test.py", Name: "test_close", Kind: "def"}, nil, true},
		{ref.Target{Path: "tests/test_shop.py", Name: "Helper.test_x", Kind: "def"}, nil, false},
		{ref.Target{Path: "tests/test_shop.py", Name: "TestShop", Kind: "class"}, nil, false},
		{ref.Target{Path: "shop.py", Name: "test_close", Kind: "def"}, nil, false},
		{ref.Target{Path: "src/lib.rs", Name: "tests.parses", Kind: "fn", Line: 3}, rs, true},
		{ref.Target{Path: "src/lib.rs", Name: "tests.later", Kind: "fn", Line: 8}, rs, true},
		{ref.Target{Path: "src/lib.rs", Name: "tests.helper", Kind: "fn", Line: 10}, rs, false},
		{ref.Target{Path: "src/lib.rs", Name: "tests.helper", Kind: "fn", Line: 10}, nil, true}, // no source: module heuristic
		{ref.Target{Path: "src/lib.rs", Name: "parse", Kind: "fn", Line: 1}, nil, false},
		// attributes on the function's own line, and several on one line
		{ref.Target{Path: "src/lib.rs", Name: "a", Kind: "fn", Line: 1}, []byte("#[test] fn a() {}\n"), true},
		{ref.Target{Path: "src/lib.rs", Name: "a", Kind: "fn", Line: 1}, []byte("    #[cfg(test)] #[tokio::test(flavor = \"multi_thread\")] pub async fn a() {}\n"), true},
		{ref.Target{Path: "src/lib.rs", Name: "a", Kind: "fn", Line: 2}, []byte("#[ignore] #[test]\nfn a() {}\n"), true},
		{ref.Target{Path: "src/lib.rs", Name: "a", Kind: "fn", Line: 1}, []byte("#[inline] fn a() {} // #[test]\n"), false},
		{ref.Target{Path: "src/lib.rs", Name: "a", Kind: "fn", Line: 2}, []byte("#[cfg(test)] #[test_case(1)]\nfn a() {}\n"), false},
	} {
		if got := IsTest(c.t, c.src); got != c.want {
			t.Errorf("IsTest(%s:%s) = %v", c.t.Path, c.t.Name, got)
		}
	}
}

func TestGo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n")
	write(t, root, "internal/files/files_test.go", "package files\n\nfunc TestList(t *testing.T) {}\n\nfunc TestOther(t *testing.T) {}\n")
	write(t, root, "internal/group/group_test.go", "package group\n\nfunc TestGroup(t *testing.T) {}\n\nfunc TestList(t *testing.T) {}\n")
	write(t, root, "tools/go.mod", "module tools\n")
	write(t, root, "tools/gen/gen_test.go", "package gen\n\nfunc TestGen(t *testing.T) {}\n")
	list := ref.Target{Path: "internal/files/files_test.go", Name: "TestList", Kind: "func"}
	other := ref.Target{Path: "internal/files/files_test.go", Name: "TestOther", Kind: "func"}
	group := ref.Target{Path: "internal/group/group_test.go", Name: "TestGroup", Kind: "func"}
	gen := ref.Target{Path: "tools/gen/gen_test.go", Name: "TestGen", Kind: "func"}

	check := func(cwd string, ts []ref.Target, want ...string) []Cmd {
		t.Helper()
		cs := For(root, cwd, ts)
		var got []string
		for _, c := range cs {
			got = append(got, c.String())
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("For from %s:\n got %q\nwant %q", cwd, got, want)
		}
		return cs
	}
	cs := check(root, []ref.Target{list}, `go test -run '^TestList$' ./internal/files/`)
	if cs[0].Tests != 1 || cs[0].Caveat != "" {
		t.Errorf("one test: %+v", cs[0])
	}
	check(filepath.Join(root, "internal"), []ref.Target{list, other, list}, `go test -run '^(TestList|TestOther)$' ./files/`)
	check(filepath.Join(root, "internal", "group"), []ref.Target{list}, `go test -run '^TestList$' ../files/`)
	check(filepath.Join(root, "internal", "files"), []ref.Target{list}, `go test -run '^TestList$' .`)
	// group also has a TestList, which -run would pick up
	cs = check(root, []ref.Target{list, group}, `go test -run '^(TestGroup|TestList)$' ./internal/files/ ./internal/group/`)
	if cs[0].Tests != 2 || cs[0].Caveat == "" {
		t.Errorf("same name in another package not caught: %+v", cs[0])
	}
	if cs = For(root, root, []ref.Target{other, group}); cs[0].Caveat != "" {
		t.Errorf("no clash, but caveat %q", cs[0].Caveat)
	}
	// a nested module runs from its own directory
	check(root, []ref.Target{list, gen}, `go test -run '^TestList$' ./internal/files/`, `go -C tools test -run '^TestGen$' ./gen/`)
	check(filepath.Join(root, "tools", "gen"), []ref.Target{gen}, `go test -run '^TestGen$' .`)
	// outside any module: no command
	if cs := For(root, root, []ref.Target{{Path: "../elsewhere/a_test.go", Name: "TestA", Kind: "func"}}); len(cs) != 0 {
		t.Errorf("outside the repo: %q", cs)
	}
}

func TestPytest(t *testing.T) {
	root := t.TempDir()
	ts := []ref.Target{
		{Path: "tests/test_shop.py", Name: "TestShop.TestInner.test_deep", Kind: "def"},
		{Path: "tests/test_shop.py", Name: "test_close", Kind: "def"},
	}
	cs := For(root, root, ts)
	if len(cs) != 1 || cs[0].String() != "pytest tests/test_shop.py::TestShop::TestInner::test_deep tests/test_shop.py::test_close" || cs[0].Tests != 2 || cs[0].Caveat != "" {
		t.Errorf("pytest: %+v", cs)
	}
	if cs := For(root, filepath.Join(root, "tests"), ts[1:]); cs[0].String() != "pytest test_shop.py::test_close" {
		t.Errorf("from tests/: %q", cs[0].String())
	}
}

func TestCargo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/a\"]\n")
	write(t, root, "crates/a/Cargo.toml", "[package]\nname = \"a\"\n")
	write(t, root, "crates/a/src/lib.rs", "")
	ts := []ref.Target{
		{Path: "crates/a/src/lib.rs", Name: "tests.parses", Kind: "fn"},
		{Path: "crates/a/src/group/mod.rs", Name: "tests.groups", Kind: "fn"},
		{Path: "crates/a/src/group/span.rs", Name: "tests.spans", Kind: "fn"},
	}
	cs := For(root, root, ts)
	want := "cargo test --manifest-path crates/a/Cargo.toml -- --exact group::span::tests::spans group::tests::groups tests::parses"
	if len(cs) != 1 || cs[0].String() != want || cs[0].Tests != 3 || cs[0].Caveat != "" {
		t.Errorf("cargo:\n got %+v\nwant %q", cs, want)
	}
	// from inside the crate, an integration test target
	write(t, root, "crates/a/tests/cli/main.rs", "mod args;\n")
	it := []ref.Target{{Path: "crates/a/tests/cli/main.rs", Name: "runs", Kind: "fn"}, {Path: "crates/a/tests/cli/args.rs", Name: "parses", Kind: "fn"}}
	if cs := For(root, filepath.Join(root, "crates", "a", "src"), it); cs[0].String() != "cargo test --test cli -- --exact args::parses runs" {
		t.Errorf("integration: %q", cs[0].String())
	}
	// a library and a binary root can share test paths: a module of src/
	// may be in both, and so may a path
	write(t, root, "crates/a/src/main.rs", "fn main() {}\n")
	if cs := For(root, root, ts[:1]); cs[0].Caveat != "" {
		t.Errorf("lib + main without tests::parses: caveat %q", cs[0].Caveat)
	}
	if cs := For(root, root, ts[1:2]); cs[0].Caveat == "" {
		t.Error("src/group/mod.rs, lib + main: no caveat")
	}
	write(t, root, "crates/a/src/main.rs", "fn main() {}\n#[cfg(test)]\nmod tests {\n    #[test]\n    fn parses() {}\n}\n")
	if cs := For(root, root, ts[:1]); cs[0].Caveat == "" {
		t.Error("lib + main: no caveat")
	}
	c := &rustCrate{dir: filepath.Join(root, "crates", "a"), name: "a", main: "a", targets: []rustTarget{{"lib", "a"}, {"bin", "a"}}}
	write(t, root, "crates/a/src/bin/tool/main.rs", "")
	write(t, root, "crates/a/src/bin/gen.rs", "")
	write(t, root, "crates/a/tests/x/main.rs", "")
	write(t, root, "crates/a/tests/y.rs", "")
	for p, want := range map[string]string{
		"src/lib.rs": " lib/a", "src/main.rs": " bin/a", "src/a/lib.rs": "a::lib lib/a bin/a", "src/a/mod.rs": "a lib/a bin/a",
		"src/bin/tool/main.rs": " bin/tool", "src/bin/tool/opts.rs": "opts bin/tool", "src/bin/gen/opts.rs": "opts bin/gen",
		"src/bin/other/opts.rs": "other::opts ?", "tests/x.rs": " test/x", "tests/x/main.rs": " test/x", "tests/x/a/mod.rs": "a test/x",
		"tests/y/a.rs": "a test/y", "tests/common/mod.rs": "common ?", "benches/b.rs": " bench/b", "build.rs": " ?",
	} {
		mod, own, known := c.place(p)
		got := mod
		for _, t := range own {
			got += " " + t.kind + "/" + t.name
		}
		if !known {
			got += " ?"
		}
		if got != want {
			t.Errorf("place(%s) = %q, want %q", p, got, want)
		}
	}
}

// TestCargoNotRun: For gives no command for a test that cargo test, as
// For builds it, may not run.
func TestCargoNotRun(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[features]\ncli = []\n\n"+
		"[[bin]]\nname = \"tool\"\nrequired-features = [\"cli\"]\n\n[[test]]\nname = \"it\"\nrequired-features = [\"cli\"]\n\n"+
		"[[test]]\nname = \"custom\"\nharness = false\n")
	write(t, root, "src/lib.rs", rustTests("tests.lib_works"))
	write(t, root, "src/bin/tool.rs", "fn main() {}\n"+rustTests("tests.tool_works"))
	write(t, root, "tests/it.rs", rustTests("it_works"))
	write(t, root, "tests/custom.rs", "fn main() {}\n"+rustTests("checks"))
	lib := ref.Target{Path: "src/lib.rs", Name: "tests.lib_works", Kind: "fn"}
	tool := ref.Target{Path: "src/bin/tool.rs", Name: "tests.tool_works", Kind: "fn"}
	it := ref.Target{Path: "tests/it.rs", Name: "it_works", Kind: "fn"}
	custom := ref.Target{Path: "tests/custom.rs", Name: "checks", Kind: "fn"}
	// cargo skips a target without its features (or refuses --test it), and
	// a target without libtest runs its main, not its tests
	for _, ts := range [][]ref.Target{{tool}, {it}, {custom}, {tool, it, custom}} {
		if cs := For(root, root, ts); len(cs) != 0 {
			t.Errorf("For(%s): %+v", ts[0].Path, cs)
		}
	}
	cs := For(root, root, []ref.Target{lib, tool, it, custom})
	if len(cs) != 1 || cs[0].String() != "cargo test -- --exact tests::lib_works" || cs[0].Tests != 1 || cs[0].Caveat == "" {
		t.Errorf("For, with the library's test: %+v", cs) // custom's main runs too
	}
	// a rerun keeps the run's features
	rerun(t, root, root, "cargo test --features cli", []ref.Target{tool}, "cargo test --features cli -- --exact tests::tool_works")

	// [lib] test = false: a module of src/ may be the library's only
	nolib := t.TempDir()
	write(t, nolib, "Cargo.toml", "[package]\nname = \"q\"\n\n[lib]\ntest = false\n")
	write(t, nolib, "src/lib.rs", "pub mod parser;\n"+rustTests("root"))
	write(t, nolib, "src/parser.rs", rustTests("tests.parses"))
	write(t, nolib, "src/main.rs", "fn main() {}\n"+rustTests("main_works"))
	parses := ref.Target{Path: "src/parser.rs", Name: "tests.parses", Kind: "fn"}
	root1 := ref.Target{Path: "src/lib.rs", Name: "root", Kind: "fn"}
	main := ref.Target{Path: "src/main.rs", Name: "main_works", Kind: "fn"}
	if cs := For(nolib, nolib, []ref.Target{parses, root1}); len(cs) != 0 {
		t.Errorf("For, [lib] test = false: %+v", cs)
	}
	if cs := For(nolib, nolib, []ref.Target{parses, main}); len(cs) != 1 || cs[0].String() != "cargo test -- --exact main_works" {
		t.Errorf("For, the main binary's test: %+v", cs)
	}

	// a module moved by a cfg_attr wrapped over lines
	moved := t.TempDir()
	write(t, moved, "Cargo.toml", "[package]\nname = \"m\"\n")
	write(t, moved, "src/lib.rs", "pub mod parser;\n")
	write(t, moved, "src/parser.rs", "#[cfg(test)]\n#[cfg_attr(\n    all(),\n    path = \"parser_tests_long.rs\"\n)]\nmod tests;\n")
	write(t, moved, "src/parser_tests_long.rs", rustTests("parses"))
	mp := ref.Target{Path: "src/parser_tests_long.rs", Name: "parses", Kind: "fn"}
	if cs := For(moved, moved, []ref.Target{mp}); len(cs) != 0 {
		t.Errorf("For, cfg_attr path: %+v", cs)
	}
	rerun(t, moved, moved, "cargo test", []ref.Target{mp}, "")
	rerunF(t, moved, moved, "cargo test", []Failed{{Target: mp, Printed: "parser::tests::parses"}}, "cargo test -- --exact parser::tests::parses")
}

func TestNext(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n")
	ns := Next(root, root, []ref.Target{{Path: "internal/files/files_test.go", Name: "TestList", Kind: "func"}}, "mentions", "List")
	if len(ns) != 1 || ns[0].Cmd != `crux go test -run '^TestList$' ./internal/files/` || ns[0].Why != "runs the 1 test that mentions List" {
		t.Errorf("Next = %+v", ns)
	}
}
