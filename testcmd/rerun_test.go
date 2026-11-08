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
	fs := make([]Failed, len(failed))
	for i, f := range failed {
		fs[i] = Failed{Target: f}
	}
	return rerunF(t, root, cwd, argv, fs, want)
}

// rerunF is rerun with the names the runner printed.
func rerunF(t *testing.T, root, cwd, argv string, failed []Failed, want string) Cmd {
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
	rerun(t, root, root, "go test ./... -- -update", one, `go test -run '^TestList$' ./internal/files/ -- -update`)
	// go test's rule: a word after the packages and a flag goes to the test
	// binary with all that follows, so it isn't a package
	rerun(t, root, root, "go test ./internal/group -v ./internal/files -race", one,
		`go test -v -run '^TestList$' ./internal/files/ -args ./internal/files -race`)
	rerun(t, root, root, "go test ./internal/group -count 1 ./internal/files", one,
		`go test -count 1 -run '^TestList$' ./internal/files/ -args ./internal/files`)
	rerun(t, root, root, "go test -v ./internal/group ./internal/files", one, `go test -v -run '^TestList$' ./internal/files/`)
	// the runner as given
	rerun(t, root, root, "/usr/lib/go-1.21/bin/go test -race ./...", one, `/usr/lib/go-1.21/bin/go test -race -run '^TestList$' ./internal/files/`)
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

func TestRerunGoSubtests(t *testing.T) {
	// a module whose go.mod has goLine ("" for none)
	fixture := func(goLine string) string {
		root := t.TempDir()
		write(t, root, "go.mod", "module x\n"+goLine)
		write(t, root, "a/a_test.go", "package a\n\nfunc TestA(t *testing.T) {}\n\nfunc TestB(t *testing.T) {}\n")
		write(t, root, "b/b_test.go", "package b\n\nfunc TestA(t *testing.T) {}\n\nfunc TestC(t *testing.T) {}\n")
		return root
	}
	a := ref.Target{Path: "a/a_test.go", Name: "TestA", Kind: "func"}
	b := ref.Target{Path: "a/a_test.go", Name: "TestB", Kind: "func"}
	bA := ref.Target{Path: "b/b_test.go", Name: "TestA", Kind: "func"}
	bC := ref.Target{Path: "b/b_test.go", Name: "TestC", Kind: "func"}
	f := func(t ref.Target, printed string) Failed { return Failed{Target: t, Printed: printed} }
	root := fixture("\ngo 1.22\n")
	check := func(want string, n int, caveat string, failed ...Failed) {
		t.Helper()
		c := rerunF(t, root, root, "go test -v ./...", failed, want)
		if want != "" && (c.Tests != n || c.Caveat != caveat) {
			t.Errorf("%s: %d tests, caveat %q; want %d, %q", want, c.Tests, c.Caveat, n, caveat)
		}
	}

	// failed subtests as go test printed them, each level quoted
	check(`go test -v -run '^TestA$/^bad$' ./a/`, 1, "", f(a, "TestA/bad"))
	check(`go test -v -run '^TestA$/^a\+b\(c\)\[1\]\|x\.\*$' ./a/`, 1, "", f(a, "TestA/a+b(c)[1]|x.*"))
	check(`go test -v -run '^TestA$/^(bad|bad#01|with_space)$' ./a/`, 3, "",
		f(a, "TestA/with_space"), f(a, "TestA/bad"), f(a, "TestA/bad#01"), f(a, "TestA/bad"))
	check(`go test -v -run '^TestA$/^outer$/^(deep|inner)$' ./a/`, 2, "", f(a, "TestA/outer/inner"), f(a, "TestA/outer/deep"))
	check(`go test -v -run '^TestA$/^a\+b$/^in$' ./a/`, 1, "", f(a, "TestA/a+b/in"))
	check(`go test -v -run '^TestA$/^bad$' ./a/ ./b/`, 2, "", f(a, "TestA/bad"), f(bA, "TestA/bad"))
	// tests: as before, with or without the printed name
	check(`go test -v -run '^TestA$' ./a/`, 1, "", f(a, ""))
	check(`go test -v -run '^(TestA|TestB)$' ./a/`, 2, "", f(a, "TestA"), f(b, "TestB"))
	// other parents and depths: alternatives
	check(`go test -v -run '^TestA$/^bad$|^TestB$/^bad$' ./a/`, 2, "", f(a, "TestA/bad"), f(b, "TestB/bad"))
	check(`go test -v -run '^TestA$/^bad$|^TestB$/^x$/^y$' ./a/`, 2, "", f(b, "TestB/x/y"), f(a, "TestA/bad"))
	check(`go test -v -run '^TestA$/^bad$|^TestA$/^outer$/^inner$' ./a/`, 2, "", f(a, "TestA/bad"), f(a, "TestA/outer/inner"))
	check(`go test -v -run '^TestA$/^x$/^inner$|^TestA$/^y$/^inner$' ./a/`, 2, "", f(a, "TestA/x/inner"), f(a, "TestA/y/inner"))
	check(`go test -v -run '^TestA$/^bad$|^TestB$' ./a/`, 2, "", f(a, "TestA/bad"), f(b, "TestB"))
	// a test that failed on its own as well as through a subtest reruns whole
	passA := "also reruns passing subtests of TestA"
	check(`go test -v -run '^TestA$' ./a/`, 2, passA, f(a, "TestA"), f(a, "TestA/bad"))
	check(`go test -v -run '^TestA$/^outer$' ./a/`, 2, "also reruns passing subtests of TestA/outer", f(a, "TestA/outer"), f(a, "TestA/outer/inner"))
	check(`go test -v -run '^TestA$|^TestB$/^x$' ./a/`, 3, passA, f(a, "TestA/bad"), f(b, "TestB/x"), f(a, "TestA"))
	// -run applies to every package: another package's test of the same name
	sameName := "also runs tests of the same name in the other packages"
	check(`go test -v -run '^TestA$/^(bad|worse)$' ./a/ ./b/`, 2, sameName, f(a, "TestA/bad"), f(bA, "TestA/worse"))
	check(`go test -v -run '^TestA$/^bad$|^TestC$/^bad$' ./a/ ./b/`, 2, sameName, f(a, "TestA/bad"), f(bC, "TestC/bad"))
	check(`go test -v -run '^TestA$|^TestC$/^bad$' ./a/ ./b/`, 3, passA+"; "+sameName, f(a, "TestA"), f(a, "TestA/bad"), f(bC, "TestC/bad"))
	check(`go test -v -run '^TestA$/^x$' ./a/ ./b/`, 2, "also reruns passing subtests of TestA/x", f(a, "TestA/x/y"), f(bA, "TestA/x"))

	// a printed name that isn't the test or one of its subtests
	for _, p := range []string{"TestB/bad", "TestAB/bad", "TestA/", "TestA//bad", "/TestA"} {
		check("", 0, "", f(a, p))
	}

	// a module that allows a go before 1.18, whose -run doesn't split on |:
	// one group as above, else the parent tests, which the caveat says
	for _, goLine := range []string{"", "\ngo 1.17\n", "go 1.16 // old\n"} {
		root = fixture(goLine)
		check(`go test -v -run '^TestA$/^(bad|with_space)$' ./a/`, 2, "", f(a, "TestA/bad"), f(a, "TestA/with_space"))
		check(`go test -v -run '^TestA$/^outer$' ./a/`, 2, "also reruns passing subtests of TestA/outer", f(a, "TestA/outer"), f(a, "TestA/outer/inner"))
		check(`go test -v -run '^(TestA|TestB)$' ./a/`, 2, "also reruns passing subtests of TestA and TestB", f(a, "TestA/bad"), f(b, "TestB/bad"))
		check(`go test -v -run '^(TestA|TestB)$' ./a/`, 2, passA, f(a, "TestA/bad"), f(b, "TestB"))
		check(`go test -v -run '^TestA$' ./a/`, 2, passA, f(a, "TestA/bad"), f(a, "TestA/outer/inner"))
		check(`go test -v -run '^(TestA|TestB)$' ./a/`, 3, "also reruns passing subtests of TestA and TestB", f(a, "TestA"), f(a, "TestA/outer/x"), f(b, "TestB/y"))
		check(`go test -v -run '^(TestA|TestC)$' ./a/ ./b/`, 2, "also reruns passing subtests of TestA and TestC; "+sameName, f(a, "TestA/bad"), f(bC, "TestC/bad"))
	}
}

func TestGoAlternates(t *testing.T) {
	for _, c := range []struct {
		gomod string
		want  bool
	}{
		{"module x\n\ngo 1.18\n", true},
		{"module x\n\ngo 1.21.0\n\ntoolchain go1.22.1\n", true},
		{"module x\ngo 1.26rc1 // next\n", true},
		{"module x\n\ngo 1.17\n", false},
		{"module x\n\ngo 1.9\n", false},
		{"module x\n", false},
		{"module x\n// go 1.22\n", false},
		{"module x\ngo 2.0\n", false},
	} {
		dir := t.TempDir()
		write(t, dir, "go.mod", c.gomod)
		if got := goAlternates(dir); got != c.want {
			t.Errorf("goAlternates(%q) = %v", c.gomod, got)
		}
	}
	if goAlternates(t.TempDir()) {
		t.Error("no go.mod: alternates")
	}
}

func TestRerunPytest(t *testing.T) {
	root := t.TempDir()
	deep := ref.Target{Path: "tests/test_shop.py", Name: "TestShop.TestInner.test_deep", Kind: "def"}
	closeT := ref.Target{Path: "tests/test_shop.py", Name: "test_close", Kind: "def"}
	one := []ref.Target{closeT}

	write(t, root, "tests/test_shop.py", "def test_close():\n    pass\n")
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
		"pytest --co tests",                           // collects, doesn't run
		"pytest --collect-only",                       //
		"pytest --sw tests",                           // stepwise skips to the last failure
		"pytest --pyargs shop",                        // packages, not paths
		"pytest -h",                                   //
		"pytest --frob x tests",                       // unknown: is x its value or a path?
		"pytest -z tests",                             //
		"pytest -xz tests",                            //
		"pytest tests --maxfail",                      // no value
		"pytest --lf=1 tests",                         // a flag with a value
		"pytest -q tests/test_shop.py::test_close[2]", // one parametrization: the rerun would run them all
		"pytest -- tests/test_shop.py::test_close[2]",
		"python -X dev -m pytest",
		"python -m unittest",
		"python3 -m pytest_cov",
	} {
		rerun(t, root, root, argv, one, "")
	}
	rerun(t, root, root, "pytest", []ref.Target{{Path: "a/a_test.go", Name: "TestA", Kind: "func"}}, "")

	// the parametrization pytest printed is the one rerun
	p2 := Failed{Target: closeT, Printed: "tests/test_shop.py::test_close[2]"}
	p3 := Failed{Target: closeT, Printed: "test_shop.py::test_close[3-a::b]"}
	if c := rerunF(t, root, root, "pytest tests", []Failed{p2, p3}, "pytest 'tests/test_shop.py::test_close[2]' 'tests/test_shop.py::test_close[3-a::b]'"); c.Tests != 2 {
		t.Errorf("two parametrizations: %+v", c)
	}
	rerunF(t, root, root, "pytest tests", []Failed{{Target: closeT, Printed: "tests/test_shop.py::test_close"}}, "pytest tests/test_shop.py::test_close")
	for _, printed := range []string{"tests/test_shop.py::TestShop::test_close", "tests/test_shop.py::test_closed", "tests/test_shop.py::test_close[2"} {
		rerunF(t, root, root, "pytest tests", []Failed{{Target: closeT, Printed: printed}}, "") // another test's id
	}
}

// TestRerunPytestConfig: the rerun settles on the run's config file and
// rootdir, which pytest finds from the paths it's given.
func TestRerunPytestConfig(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pyproject.toml", "[tool.pytest.ini_options]\naddopts = \"-p no:cacheprovider\"\n")
	write(t, root, "packages/foo/pyproject.toml", "[project]\nname = \"foo\"\n\n[tool.pytest.ini_options]\naddopts = \"--strict-markers\"\n")
	write(t, root, "packages/foo/tests/test_x.py", "def test_a():\n    assert False\n")
	write(t, root, "packages/bar/tests/test_y.py", "def test_b():\n    assert False\n")
	a := []ref.Target{{Path: "packages/foo/tests/test_x.py", Name: "test_a", Kind: "def"}}
	foo := filepath.Join(root, "packages", "foo")

	// from the root without paths the run used the root's config; the node
	// id alone would make pytest use packages/foo's
	rerun(t, root, root, "pytest -x", a, "pytest -x -c pyproject.toml packages/foo/tests/test_x.py::test_a")
	rerun(t, root, root, "pytest packages", a, "pytest -c pyproject.toml packages/foo/tests/test_x.py::test_a")
	rerun(t, root, filepath.Join(root, "packages"), "pytest", a, "pytest -c ../pyproject.toml foo/tests/test_x.py::test_a")
	// the same config either way: nothing to pin
	rerun(t, root, root, "pytest packages/foo/tests", a, "pytest packages/foo/tests/test_x.py::test_a")
	rerun(t, root, foo, "pytest", a, "pytest tests/test_x.py::test_a")
	rerun(t, root, root, "pytest -c pyproject.toml", a, "pytest -c pyproject.toml packages/foo/tests/test_x.py::test_a")
	rerun(t, root, root, "pytest --rootdir=. packages", a, "pytest --rootdir=. -c pyproject.toml packages/foo/tests/test_x.py::test_a")
	// a plugin's option value counts as a path when pytest picks its config
	rerun(t, root, root, "pytest --cov packages/foo", a, "pytest --cov packages/foo packages/foo/tests/test_x.py::test_a")
	rerun(t, root, root, "pytest --cov=packages/foo", a, "pytest --cov=packages/foo -c pyproject.toml packages/foo/tests/test_x.py::test_a")

	// without config files, a setup.py marks the rootdir
	bare := t.TempDir()
	write(t, bare, "a/setup.py", "")
	write(t, bare, "a/test_x.py", "def test_a():\n    assert False\n")
	write(t, bare, "b/test_y.py", "def test_b():\n    assert False\n")
	rerun(t, bare, bare, "pytest a b", []ref.Target{{Path: "a/test_x.py", Name: "test_a", Kind: "def"}}, "pytest --rootdir . a/test_x.py::test_a")
	// the run had no config file and the rerun would: no way to pin that
	write(t, bare, "b/pytest.ini", "[pytest]\n")
	rerun(t, bare, bare, "pytest", []ref.Target{{Path: "b/test_y.py", Name: "test_b", Kind: "def"}}, "")
	// (with both paths given, pytest searched b too and found it)
	rerun(t, bare, bare, "pytest a b", []ref.Target{{Path: "b/test_y.py", Name: "test_b", Kind: "def"}}, "pytest b/test_y.py::test_b")
}

func TestPyprojectConfig(t *testing.T) {
	for src, want := range map[string][2]bool{
		"[tool.pytest.ini_options]\n":                         {true, true},
		"[tool.pytest]\nminversion = \"9\"\n":                 {true, true},
		"[ tool . \"pytest\" . ini_options ] # x\n":           {true, true},
		"[tool.pytest]\n\n[tool.ruff]\nx = 1\n":               {false, true}, // empty
		"[tool.pytest-watcher]\nnow = true\n":                 {false, true},
		"[project]\ndependencies = [\"pytest\"]\n":            {false, true},
		"x = \"\"\"\n[tool.pytest.ini_options]\n\"\"\"\n":     {false, true}, // in a string
		"# [tool.pytest.ini_options]\n":                       {false, true},
		"tool.pytest.ini_options.addopts = \"-x\"\n":          {false, false},
		"[tool]\npytest.ini_options = { addopts = \"-x\" }\n": {false, false},
	} {
		if has, ok := pyprojectConfig([]byte(src)); has != want[0] || ok != want[1] {
			t.Errorf("pyprojectConfig(%q) = %v, %v", src, has, ok)
		}
	}
}

// rustTests is a Rust file with #[test] functions, at the top or in a
// tests module ("tests.parses").
func rustTests(names ...string) string {
	var top, mod strings.Builder
	for _, n := range names {
		if m, ok := strings.CutPrefix(n, "tests."); ok {
			mod.WriteString("    #[test]\n    fn " + m + "() {}\n")
		} else {
			top.WriteString("#[test]\nfn " + n + "() {}\n")
		}
	}
	return top.String() + "#[cfg(test)]\nmod tests {\n" + mod.String() + "}\n"
}

func TestRerunCargo(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[workspace]\nmembers = [\"crates/a\", \"crates/b\"]\n")
	write(t, root, "crates/a/Cargo.toml", "[package]\nname = \"a\"\n")
	write(t, root, "crates/a/src/lib.rs", "mod group;\n"+rustTests("tests.parses"))
	write(t, root, "crates/a/src/group/mod.rs", rustTests("tests.groups"))
	write(t, root, "crates/a/tests/cli/main.rs", "mod args;\n"+rustTests("runs"))
	write(t, root, "crates/a/tests/cli/args.rs", rustTests("parses"))
	write(t, root, "crates/b/Cargo.toml", "[package]\nname = \"b\"\n")
	write(t, root, "crates/b/src/lib.rs", rustTests("tests.parses"))
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
	// the runner as given
	rerun(t, root, a, "/home/u/.cargo/bin/cargo test", []ref.Target{parses}, "/home/u/.cargo/bin/cargo test -- --exact tests::parses")

	// the same path failing in two crates is two tests, and nothing else runs
	if c := rerun(t, root, root, "cargo test --workspace", []ref.Target{parses, bTest}, "cargo test --workspace -- --exact tests::parses"); c.Caveat != "" || c.Tests != 2 {
		t.Errorf("two crates: %+v", c)
	}
	// but where only one failed, the other crate's runs too: with
	// --workspace, from the virtual root, not with -p
	for _, argv := range []string{"cargo test --workspace", "cargo test", "cargo test --all --lib"} {
		if c := rerun(t, root, root, argv, []ref.Target{parses}, argv+" -- --exact tests::parses"); c.Caveat == "" {
			t.Errorf("%s: b's tests::parses runs too, unsaid", argv)
		}
	}
	if c := rerun(t, root, root, "cargo test -p a", []ref.Target{parses}, "cargo test -p a -- --exact tests::parses"); c.Caveat != "" {
		t.Errorf("-p a: caveat %q", c.Caveat)
	}
	if c := rerun(t, root, root, "cargo test --workspace", []ref.Target{groups}, "cargo test --workspace -- --exact group::tests::groups"); c.Caveat != "" {
		t.Errorf("a path only a has: caveat %q", c.Caveat)
	}
	// a library and integration tests: a caveat only where a path repeats
	if c := rerun(t, root, a, "cargo test", append([]ref.Target{parses}, cli...), "cargo test -- --exact args::parses runs tests::parses"); c.Caveat != "" {
		t.Errorf("lib + cli: caveat %q", c.Caveat)
	}
	write(t, root, "crates/a/tests/cli/tests.rs", rustTests("parses"))
	if c := rerun(t, root, a, "cargo test", []ref.Target{parses}, "cargo test -- --exact tests::parses"); c.Caveat == "" {
		t.Error("cli's tests::parses runs too, unsaid")
	}
	if c := rerun(t, root, a, "cargo test --lib", []ref.Target{parses}, "cargo test --lib -- --exact tests::parses"); c.Caveat != "" {
		t.Errorf("--lib: caveat %q", c.Caveat)
	}
	// a target the command doesn't build: the run couldn't have run the test
	rerun(t, root, a, "cargo test --lib", cli[:1], "")
	rerun(t, root, a, "cargo test --test other", []ref.Target{parses}, "")
	rerun(t, root, a, "cargo test -p b", []ref.Target{parses}, "")

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

// TestRerunCargoTargets: a library and binaries can hold the same path, so
// only a selection of one of them is exact.
func TestRerunCargoTargets(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[package]\nname = \"app\"\n")
	write(t, root, "src/lib.rs", "mod util;\n"+rustTests("tests.works"))
	write(t, root, "src/util.rs", rustTests("tests.helps"))
	write(t, root, "src/main.rs", "fn main() {}\n"+rustTests("tests.works"))
	write(t, root, "src/bin/tool.rs", "fn main() {}\n"+rustTests("tests.works", "tests.tool"))
	lib := ref.Target{Path: "src/lib.rs", Name: "tests.works", Kind: "fn"}
	util := ref.Target{Path: "src/util.rs", Name: "tests.helps", Kind: "fn"}
	tool := ref.Target{Path: "src/bin/tool.rs", Name: "tests.works", Kind: "fn"}
	toolOnly := ref.Target{Path: "src/bin/tool.rs", Name: "tests.tool", Kind: "fn"}

	for argv, caveat := range map[string]bool{
		"cargo test":                     true, // main and tool have tests::works too
		"cargo test --tests":             true, // still the library and the binaries
		"cargo test --lib --bins":        true,
		"cargo test --lib --bin app":     true,
		"cargo test --all-targets":       true,
		"cargo test --lib":               false,
		"cargo test --lib --test x":      false,
		"cargo test --lib --bin toolbox": false,
	} {
		want := argv + " -- --exact tests::works"
		if c := rerun(t, root, root, argv, []ref.Target{lib}, want); (c.Caveat != "") != caveat {
			t.Errorf("%s: caveat %q", argv, c.Caveat)
		}
	}
	if c := rerun(t, root, root, "cargo test --lib --bin a*", []ref.Target{lib}, "cargo test --lib --bin 'a*' -- --exact tests::works"); c.Caveat == "" {
		t.Error("--bin a*: no caveat")
	}
	// src/bin/tool.rs is the tool binary's, and only it has tests::tool
	if c := rerun(t, root, root, "cargo test --bin tool", []ref.Target{tool}, "cargo test --bin tool -- --exact tests::works"); c.Caveat != "" {
		t.Errorf("--bin tool: caveat %q", c.Caveat)
	}
	if c := rerun(t, root, root, "cargo test", []ref.Target{toolOnly}, "cargo test -- --exact tests::tool"); c.Caveat != "" {
		t.Errorf("tests::tool: caveat %q", c.Caveat)
	}
	rerun(t, root, root, "cargo test --lib", []ref.Target{tool}, "")
	// a module of src/ may be in the library and the main binary both
	if c := rerun(t, root, root, "cargo test", []ref.Target{util}, "cargo test -- --exact util::tests::helps"); c.Caveat == "" {
		t.Error("src/util.rs: no caveat")
	}
	if c := rerun(t, root, root, "cargo test --lib", []ref.Target{util}, "cargo test --lib -- --exact util::tests::helps"); c.Caveat != "" {
		t.Errorf("src/util.rs, --lib: caveat %q", c.Caveat)
	}

	// a library and src/bin/tool.rs alone
	write(t, root, "src/main.rs", "fn main() {}\n")
	if c := rerun(t, root, root, "cargo test", []ref.Target{lib}, "cargo test -- --exact tests::works"); c.Caveat == "" {
		t.Error("tool's tests::works runs too, unsaid")
	}
	if c := rerun(t, root, root, "cargo test --bins", []ref.Target{tool}, "cargo test --bins -- --exact tests::works"); c.Caveat != "" {
		t.Errorf("--bins, only tool has tests::works: caveat %q", c.Caveat)
	}
}

// TestRerunCargoSelection: which targets each selection builds is
// cargo's: --benches builds the library and binaries too, and the
// manifest's test, bench and harness settings count.
func TestRerunCargoSelection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n")
	write(t, root, "src/lib.rs", rustTests("works"))
	write(t, root, "benches/b.rs", rustTests("works", "bench_only"))
	write(t, root, "examples/e.rs", "fn main() {}\n"+rustTests("works"))
	lib := ref.Target{Path: "src/lib.rs", Name: "works", Kind: "fn"}
	bench := ref.Target{Path: "benches/b.rs", Name: "works", Kind: "fn"}
	benchOnly := ref.Target{Path: "benches/b.rs", Name: "bench_only", Kind: "fn"}
	for _, c := range []struct {
		argv   string
		failed ref.Target
		caveat bool // "": no command
	}{
		{"cargo test --benches", bench, true}, // the library's works runs too
		{"cargo test --benches", lib, true},   // the library is built with --benches
		{"cargo test --benches", benchOnly, false},
		{"cargo test --bench b", bench, false},
		{"cargo test --examples", lib, false}, // no command: --examples builds examples only
		{"cargo test --example e", lib, false},
		{"cargo test --all-targets", bench, true},
		{"cargo test", lib, false}, // benches and examples aren't tested by default
	} {
		want := c.argv + " -- --exact works"
		if c.failed == benchOnly {
			want = c.argv + " -- --exact bench_only"
		}
		if strings.Contains(c.argv, "example") {
			want = ""
		}
		if cmd := rerun(t, root, root, c.argv, []ref.Target{c.failed}, want); want != "" && (cmd.Caveat != "") != c.caveat {
			t.Errorf("%s, %s: caveat %q", c.argv, c.failed.Path, cmd.Caveat)
		}
	}

	// the manifest's settings: what --benches and the default select, and
	// a target without libtest, which runs what it likes
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[lib]\nbench = false\n\n[[example]]\nname = \"e\"\ntest = true\n")
	if c := rerun(t, root, root, "cargo test --benches", []ref.Target{bench}, "cargo test --benches -- --exact works"); c.Caveat != "" {
		t.Errorf("[lib] bench = false: caveat %q", c.Caveat)
	}
	if c := rerun(t, root, root, "cargo test", []ref.Target{lib}, "cargo test -- --exact works"); c.Caveat == "" {
		t.Error("example e with test = true: its works runs too, unsaid")
	}
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[[bench]]\nname = \"b\"\nharness = false\n")
	write(t, root, "benches/b.rs", "fn main() {}\n") // no works of its own: its main decides
	if c := rerun(t, root, root, "cargo test --benches", []ref.Target{lib}, "cargo test --benches -- --exact works"); c.Caveat == "" {
		t.Error("bench b without libtest: no caveat")
	}
	if cs := For(root, root, []ref.Target{lib}); len(cs) != 1 || cs[0].Caveat != "" {
		t.Errorf("For, a bench without libtest (not built by default): %+v", cs)
	}
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[lib]\ntest = false\n")
	rerun(t, root, root, "cargo test", []ref.Target{lib}, "")
	if cs := For(root, root, []ref.Target{lib}); len(cs) != 0 {
		t.Errorf("For, [lib] test = false: %+v", cs)
	}

	// a [[bin]] at src/main.rs, under its own name
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[[bin]]\nname = \"tool\"\npath = \"src/main.rs\"\n")
	write(t, root, "src/main.rs", "fn main() {}\n"+rustTests("main_works"))
	main := ref.Target{Path: "src/main.rs", Name: "main_works", Kind: "fn"}
	rerun(t, root, root, "cargo test --bin tool", []ref.Target{main}, "cargo test --bin tool -- --exact main_works")
	rerun(t, root, root, "cargo test --bin p", []ref.Target{main}, "")
	if cs := For(root, root, []ref.Target{main}); len(cs) != 1 {
		t.Errorf("For, [[bin]] path = src/main.rs: %+v", cs)
	}
	for _, m := range []string{"[lib]\npath = \"src/lib.rs\"\n", "[[bench]]\nname = \"b\"\npath = \"benches/b.rs\"\n"} {
		write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n"+m)
		if cs := For(root, root, []ref.Target{lib}); len(cs) != 1 {
			t.Errorf("For, %q (the path cargo finds anyway): %+v", m, cs)
		}
	}
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n\n[[bench]]\nname = \"b\"\npath = \"perf/b.rs\"\n")
	if cs := For(root, root, []ref.Target{lib}); len(cs) != 0 {
		t.Errorf("For, a bench elsewhere: %+v", cs)
	}
}

// TestRerunCargoOneLine: a test whose attributes share its line (or its
// module's) is still found in another package.
func TestRerunCargoOneLine(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[workspace]\nmembers = [\"a\", \"b\"]\n")
	write(t, root, "a/Cargo.toml", "[package]\nname = \"a\"\n")
	write(t, root, "a/src/lib.rs", rustTests("tests.parses"))
	write(t, root, "b/Cargo.toml", "[package]\nname = \"b\"\n")
	write(t, root, "b/src/lib.rs", "#[cfg(test)] mod tests {\n    #[test] fn parses() {}\n}\n")
	parses := []ref.Target{{Path: "a/src/lib.rs", Name: "tests.parses", Kind: "fn"}}
	if c := rerun(t, root, root, "cargo test --workspace", parses, "cargo test --workspace -- --exact tests::parses"); c.Caveat == "" {
		t.Error("b's one-line tests::parses runs too, unsaid")
	}
	// --exclude leaves a package out
	if c := rerun(t, root, root, "cargo test --workspace --exclude b", parses, "cargo test --workspace --exclude b -- --exact tests::parses"); c.Caveat != "" {
		t.Errorf("--exclude b: caveat %q", c.Caveat)
	}
	rerun(t, root, root, "cargo test --workspace --exclude a", parses, "")
}

func TestRustOpaqueSrc(t *testing.T) {
	for src, want := range map[string]bool{
		"#[path = \"imp.rs\"]\nmod imp;\n":                     true,
		"    #[cfg(unix)] #[path = \"unix.rs\"] mod sys;\n":    true,
		"#[cfg_attr(unix, path = \"unix.rs\")]\nmod sys;\n":    true,
		"mod a; #[path = \"b.rs\"] mod b;\n":                   true,
		"mod gen { include!(\"gen.rs\"); }\n":                  true,
		"include!(r\"gen.rs\");\n":                             true,
		"// see #[path = \"x.rs\"]\n":                          false,
		"/// include!(\"x.rs\")\n":                             false,
		"let s = \"#[path = \\\"x.rs\\\"]\";\n":                false,
		"assert!(out.contains(\"include!(\\\"a\\\")\"));\n":    false,
		"include!(concat!(env!(\"OUT_DIR\"), \"/gen.rs\"));\n": false,
		"#[serde(with = \"path\")]\nfield: u8,\n":              false,
		// cfg_attr wrapped over lines, as rustfmt does with long ones
		"#[cfg(test)]\n#[cfg_attr(\n    all(),\n    path = \"parser_tests.rs\"\n)]\nmod tests;\n":      true,
		"#[cfg_attr(feature = \"a)\", path = \"x.rs\")]\nmod x;\n":                                     true,
		"#[cfg_attr(\n    feature = \"serde\",\n    derive(Serialize)\n)]\nstruct S;\nlet path = 1;\n": false,
	} {
		if got := rustMoves([]byte(src)); got != want {
			t.Errorf("rustMoves(%q) = %v", src, got)
		}
	}
}

// TestRerunCargoPrinted: where the files don't tell a test's path, only
// the path cargo printed can rerun it.
func TestRerunCargoPrinted(t *testing.T) {
	root := t.TempDir()
	write(t, root, "Cargo.toml", "[package]\nname = \"p\"\n")
	write(t, root, "src/lib.rs", "#[path = \"parser/imp.rs\"]\nmod parser;\n")
	write(t, root, "src/parser/imp.rs", rustTests("tests.works"))
	works := ref.Target{Path: "src/parser/imp.rs", Name: "tests.works", Kind: "fn"}

	rerun(t, root, root, "cargo test", []ref.Target{works}, "") // by file it'd be parser::imp::tests::works
	c := rerunF(t, root, root, "cargo test", []Failed{{Target: works, Printed: "parser::tests::works"}}, "cargo test -- --exact parser::tests::works")
	if c.Tests != 1 || c.Caveat != "" {
		t.Errorf("printed: %+v", c)
	}
	rerunF(t, root, root, "cargo test", []Failed{{Target: works, Printed: "parser::tests::other"}}, "") // another test's
	if cs := For(root, root, []ref.Target{works}); len(cs) != 0 {
		t.Errorf("For with #[path]: %+v", cs)
	}

	// a target with its own path
	custom := t.TempDir()
	write(t, custom, "Cargo.toml", "[package]\nname = \"c\"\n\n[lib]\npath = \"lib/mod.rs\"\n")
	write(t, custom, "lib/mod.rs", rustTests("tests.works"))
	write(t, custom, "src/lib.rs", rustTests("tests.works"))
	lw := ref.Target{Path: "src/lib.rs", Name: "tests.works", Kind: "fn"}
	rerun(t, custom, custom, "cargo test", []ref.Target{lw}, "")
	if cs := For(custom, custom, []ref.Target{lw}); len(cs) != 0 {
		t.Errorf("For with [lib] path: %+v", cs)
	}

	// a module of tests/ no target owns: each test that includes it has it
	shared := t.TempDir()
	write(t, shared, "Cargo.toml", "[package]\nname = \"s\"\n")
	write(t, shared, "tests/api.rs", "mod common;\n"+rustTests("api"))
	write(t, shared, "tests/cli.rs", "mod common;\n"+rustTests("cli"))
	write(t, shared, "tests/common/mod.rs", rustTests("helper"))
	helper := ref.Target{Path: "tests/common/mod.rs", Name: "helper", Kind: "fn"}
	rerun(t, shared, shared, "cargo test", []ref.Target{helper}, "")
	if c := rerunF(t, shared, shared, "cargo test", []Failed{{Target: helper, Printed: "common::helper"}}, "cargo test -- --exact common::helper"); c.Caveat == "" {
		t.Error("common::helper: no caveat")
	}
	rerunF(t, shared, shared, "cargo test --test api", []Failed{{Target: helper, Printed: "common::helper"}}, "cargo test --test api -- --exact common::helper")
	api := ref.Target{Path: "tests/api.rs", Name: "api", Kind: "fn"}
	if cs := For(shared, shared, []ref.Target{helper, api}); len(cs) != 1 || cs[0].String() != "cargo test --test api -- --exact api" || cs[0].Tests != 1 {
		t.Errorf("For with tests/common: %+v", cs)
	}
}

func TestRerunNext(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n")
	a := []Failed{{Target: ref.Target{Path: "a/a_test.go", Name: "TestA", Kind: "func"}}}
	n, ok := RerunNext(root, root, []string{"go", "test", "-race", "./..."}, a)
	if !ok || n.Cmd != `crux go test -race -run '^TestA$' ./a/` || n.Why != "reruns the 1 failed test" {
		t.Errorf("RerunNext = %+v, %v", n, ok)
	}
	if _, ok := RerunNext(root, root, []string{"make", "test"}, a); ok {
		t.Error("make: a suggestion")
	}
}
