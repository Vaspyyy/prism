package testcmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/ref"
)

// rerun checks Rerun from cwd: want "" means no command.
func rerun(t *testing.T, root, cwd, argv string, failed []ref.Target, want string) Cmd {
	t.Helper()
	c, ok := Rerun(root, cwd, strings.Fields(argv), failed)
	got := ""
	if ok {
		got = c.String()
	}
	if got != want {
		t.Errorf("Rerun(%q)\n got %q\nwant %q", argv, got, want)
	}
	return c
}

func TestRerunGo(t *testing.T) {
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
	one := []ref.Target{list}

	c := rerun(t, root, root, "go test ./...", one, `go test -run '^TestList$' ./internal/files/`)
	if c.Tests != 1 || c.Caveat != "" {
		t.Errorf("one test: %+v", c)
	}
	// flags stay, in order; values that look like packages are values
	rerun(t, root, root, "go test -race -count=1 -tags integration -timeout 30s -v ./...", one,
		`go test -race -count=1 -tags integration -timeout 30s -v -run '^TestList$' ./internal/files/`)
	rerun(t, root, root, "go test -p 1 -coverprofile c.out -covermode atomic -cover ./internal/files", one,
		`go test -p 1 -coverprofile c.out -covermode atomic -cover -run '^TestList$' ./internal/files/`)
	// -run is replaced, benchmarks dropped; flags after the packages count too
	rerun(t, root, root, "go test -run TestX -bench . -benchtime=2s -benchmem ./internal/...", one,
		`go test -benchmem -run '^TestList$' ./internal/files/`)
	rerun(t, root, root, "go test ./... -run=Foo -count 1 --race -skip TestSlow", one,
		`go test -count 1 --race -skip TestSlow -run '^TestList$' ./internal/files/`)
	// what follows -args goes to the test binary as is
	rerun(t, root, root, "go test ./... -args -update -v", one, `go test -run '^TestList$' ./internal/files/ -args -update -v`)
	// several tests and packages, with the same-name caveat
	c = rerun(t, root, root, "go test -v ./...", []ref.Target{list, other, group},
		`go test -v -run '^(TestGroup|TestList|TestOther)$' ./internal/files/ ./internal/group/`)
	if c.Tests != 3 || c.Caveat == "" {
		t.Errorf("group's TestList runs too, unsaid: %+v", c)
	}
	// from a subdirectory, and with -C (kept where it was)
	rerun(t, root, filepath.Join(root, "internal"), "go test ./...", one, `go test -run '^TestList$' ./files/`)
	rerun(t, root, root, "go -C internal test -race ./...", one, `go -C internal test -race -run '^TestList$' ./files/`)
	rerun(t, root, root, "go test -C internal ./...", one, `go test -C internal -run '^TestList$' ./files/`)
	// another module: only reachable from here in a workspace
	rerun(t, root, filepath.Join(root, "tools"), "go test ./...", []ref.Target{gen}, `go test -run '^TestGen$' ./gen/`)
	rerun(t, root, root, "go test ./...", []ref.Target{gen}, "")
	write(t, root, "go.work", "go 1.22\n\nuse (\n\t.\n\t./tools\n)\n")
	rerun(t, root, root, "go test ./...", []ref.Target{gen}, `go test -run '^TestGen$' ./tools/gen/`)

	// no command when it can't be exact
	for _, argv := range []string{
		"go test -fuzz FuzzList ./internal/files", // fuzzing, not running tests
		"go test -list . ./...",
		"go test -c ./internal/files",
		"go test -o files.test ./internal/files",
		"go test -n ./...",
		"go test -frobnicate x ./...",          // unknown: is x its value or a package?
		"go test -run TestList/empty ./...",    // a subtest filter: the whole test would run more
		"go test internal/files/files_test.go", // files, not packages
		"go test ./... -args -test.run=TestX",  // the binary's own filter wins
		"go test -test.v ./...",
		"go test -timeout", // a flag without its value
		"go vet ./...",
		"go build ./...",
		"go",
		"gotest ./...",
	} {
		rerun(t, root, root, argv, one, "")
	}
	rerun(t, root, root, "go test ./...", []ref.Target{list, gen}, "") // two modules
	rerun(t, root, root, "go test ./...", []ref.Target{{Path: "tests/test_x.py", Name: "test_a", Kind: "def"}}, "")
	rerun(t, root, root, "go test ./...", nil, "")
}

func TestRerunPytest(t *testing.T) {
	root := t.TempDir()
	deep := ref.Target{Path: "tests/test_shop.py", Name: "TestShop.TestInner.test_deep", Kind: "def"}
	closeT := ref.Target{Path: "tests/test_shop.py", Name: "test_close", Kind: "def"}
	one := []ref.Target{closeT}

	c := rerun(t, root, root, "pytest -x -k smoke --tb=short tests/", one, "pytest -x -k smoke --tb=short tests/test_shop.py::test_close")
	if c.Tests != 1 || c.Caveat != "" {
		t.Errorf("one test: %+v", c)
	}
	c = rerun(t, root, root, "python3 -m pytest -q -n 4 tests/test_shop.py", []ref.Target{deep, closeT, deep},
		"python3 -m pytest -q -n 4 tests/test_shop.py::TestShop::TestInner::test_deep tests/test_shop.py::test_close")
	if c.Tests != 2 {
		t.Errorf("two tests: %+v", c)
	}
	// short options clustered or with the value attached; values aren't paths
	rerun(t, root, root, "pytest -xvs -rA -kfoo -p no:cacheprovider tests", one,
		"pytest -xvs -rA -kfoo -p no:cacheprovider tests/test_shop.py::test_close")
	rerun(t, root, root, "pytest -vk smoke tests", one, "pytest -vk smoke tests/test_shop.py::test_close")
	rerun(t, root, root, "pytest --maxfail 2 --deselect tests/test_shop.py::test_slow tests", one,
		"pytest --maxfail 2 --deselect tests/test_shop.py::test_slow tests/test_shop.py::test_close")
	// --cov takes the next word unless it's an option
	rerun(t, root, root, "pytest --cov src tests", one, "pytest --cov src tests/test_shop.py::test_close")
	rerun(t, root, root, "pytest --cov --tb=line tests", one, "pytest --cov --tb=line tests/test_shop.py::test_close")
	rerun(t, root, root, "pytest -v -- tests/", one, "pytest -v tests/test_shop.py::test_close")
	rerun(t, root, filepath.Join(root, "tests"), "py.test", one, "py.test test_shop.py::test_close")
	rerun(t, root, root, "/usr/bin/python3.12 -m pytest", one, "/usr/bin/python3.12 -m pytest tests/test_shop.py::test_close")

	for _, argv := range []string{
		"pytest --co tests",      // collects, doesn't run
		"pytest --collect-only",  //
		"pytest --sw tests",      // stepwise skips to the last failure
		"pytest --pyargs shop",   // packages, not paths
		"pytest -h",              //
		"pytest --frob x tests",  // unknown: is x its value or a path?
		"pytest -z tests",        //
		"pytest -xz tests",       //
		"pytest tests --maxfail", // no value
		"pytest --lf=1 tests",    // a flag with a value
		"python -X dev -m pytest",
		"python -m unittest",
		"python3 -m pytest_cov",
	} {
		rerun(t, root, root, argv, one, "")
	}
	rerun(t, root, root, "pytest", []ref.Target{{Path: "a/a_test.go", Name: "TestA", Kind: "func"}}, "")
}

func TestRerunCargo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/a\", \"crates/b\"]\n")
	write(t, root, "crates/a/Cargo.toml", "[package]\nname = \"a\"\n")
	write(t, root, "crates/a/src/lib.rs", "")
	write(t, root, "crates/b/Cargo.toml", "[package]\nname = \"b\"\n")
	write(t, root, "crates/b/src/lib.rs", "")
	parses := ref.Target{Path: "crates/a/src/lib.rs", Name: "tests.parses", Kind: "fn"}
	groups := ref.Target{Path: "crates/a/src/group/mod.rs", Name: "tests.groups", Kind: "fn"}
	bTest := ref.Target{Path: "crates/b/src/lib.rs", Name: "tests.parses", Kind: "fn"}
	cli := []ref.Target{{Path: "crates/a/tests/cli/main.rs", Name: "runs", Kind: "fn"}, {Path: "crates/a/tests/cli/args.rs", Name: "parses", Kind: "fn"}}
	a := filepath.Join(root, "crates", "a")

	c := rerun(t, root, a, "cargo test", []ref.Target{parses, groups}, "cargo test -- --exact group::tests::groups tests::parses")
	if c.Tests != 2 || c.Caveat != "" {
		t.Errorf("two tests: %+v", c)
	}
	// cargo's and libtest's options stay, values aren't filters
	rerun(t, root, root, "cargo +nightly test --release -p a --features x -j 4 -- --test-threads 1 --nocapture", []ref.Target{groups},
		"cargo +nightly test --release -p a --features x -j 4 -- --test-threads 1 --nocapture --exact group::tests::groups")
	// filters are replaced, --exact given once
	rerun(t, root, a, "cargo test parses -- --exact other --skip slow", []ref.Target{parses},
		"cargo test -- --skip slow --exact tests::parses")
	// target selection stays
	c = rerun(t, root, a, "cargo test --test cli", cli, "cargo test --test cli -- --exact args::parses runs")
	if c.Caveat != "" {
		t.Errorf("one target: %+v", c)
	}
	// a test with the same path may run in another crate or target
	if c := rerun(t, root, root, "cargo test --workspace", []ref.Target{parses, bTest}, "cargo test --workspace -- --exact tests::parses"); c.Caveat == "" || c.Tests != 1 {
		t.Errorf("two crates: %+v", c)
	}
	if c := rerun(t, root, a, "cargo test", append([]ref.Target{parses}, cli...), "cargo test -- --exact args::parses runs tests::parses"); c.Caveat == "" {
		t.Errorf("two targets: %+v", c)
	}
	write(t, root, "crates/a/src/main.rs", "")
	if c := rerun(t, root, a, "cargo test", []ref.Target{parses}, "cargo test -- --exact tests::parses"); c.Caveat == "" {
		t.Error("lib + main: no caveat")
	}
	if c := rerun(t, root, a, "cargo test --lib", []ref.Target{parses}, "cargo test --lib -- --exact tests::parses"); c.Caveat != "" {
		t.Errorf("--lib: caveat %q", c.Caveat)
	}

	for _, argv := range []string{
		"cargo test --doc", // doctests only
		"cargo test --no-run",
		"cargo test -- --list",
		"cargo test -- --bench",
		"cargo test --frob x", // unknown: is x its value or the filter?
		"cargo test -- --frob",
		"cargo test -p",
		"cargo nextest run",
		"cargo build",
		"cargo",
	} {
		rerun(t, root, a, argv, []ref.Target{parses}, "")
	}
	rerun(t, root, a, "cargo test", []ref.Target{{Path: "x/a_test.go", Name: "TestA", Kind: "func"}}, "")
}

func TestRerunNext(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n")
	n, ok := RerunNext(root, root, []string{"go", "test", "-race", "./..."}, []ref.Target{{Path: "a/a_test.go", Name: "TestA", Kind: "func"}})
	if !ok || n.Cmd != `crux go test -race -run '^TestA$' ./a/` || n.Why != "reruns the 1 failed test" {
		t.Errorf("RerunNext = %+v, %v", n, ok)
	}
	if _, ok := RerunNext(root, root, []string{"make", "test"}, []ref.Target{{Path: "a/a_test.go", Name: "TestA", Kind: "func"}}); ok {
		t.Error("make: a suggestion")
	}
}
