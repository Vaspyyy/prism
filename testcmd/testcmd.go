// Package testcmd builds the command that runs a given set of tests: Go,
// pytest and cargo (docs/evidence.md, "Suggesting tests"). For builds it
// from scratch; Rerun reruns a run's failed tests with the run's own flags.
//
// It only builds commands it can make precise. Each one says how many of
// the tests it selects and, where the runner's filter may also select
// others, a caveat in words a reason line can use. A wrong suggestion is
// worse than none, so other ecosystems get no command.
package testcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/report"
)

// Cmd is a command that runs tests.
type Cmd struct {
	Args   []string // starting with the runner: "go", "pytest", "cargo"
	Tests  int      // how many of the given tests it runs
	Caveat string   // what else it may run; "" when it runs exactly those tests
}

// String is the command as a shell line.
func (c Cmd) String() string { return report.Cmd(c.Args...) }

// IsTest reports whether a symbol is a test its runner can select by name:
//
//   - Go: a TestXxx, FuzzXxx or ExampleXxx function (no receiver) in a
//     _test.go file
//   - Python: a test_* function or method, inside Test* classes only, in a
//     test_*.py or *_test.py file (pytest's default discovery)
//   - Rust: a function marked #[test] or #[<crate>::test]. src is the
//     file's content; without it, a function in a tests module or under
//     tests/ counts.
func IsTest(t ref.Target, src []byte) bool {
	base := filepath.Base(t.Path)
	switch {
	case strings.HasSuffix(t.Path, "_test.go"):
		return t.Kind == "func" && !strings.Contains(t.Name, ".") && goTestName(t.Name)
	case strings.HasSuffix(t.Path, ".py"):
		if t.Kind != "def" || !(strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) {
			return false
		}
		parts := strings.Split(t.Name, ".")
		for _, p := range parts[:len(parts)-1] {
			if !strings.HasPrefix(p, "Test") {
				return false
			}
		}
		return strings.HasPrefix(parts[len(parts)-1], "test")
	case strings.HasSuffix(t.Path, ".rs"):
		if t.Kind != "fn" {
			return false
		}
		if src == nil {
			return strings.HasPrefix(t.Name, "tests.") || strings.Contains(t.Name, ".tests.") || strings.HasPrefix(t.Path, "tests/") || strings.Contains(t.Path, "/tests/")
		}
		return rustTestAttr(src, t.Line)
	}
	return false
}

// goTestName is go test's rule: the prefix, then nothing or a character
// that isn't a lowercase letter.
func goTestName(name string) bool {
	for _, p := range []string{"Test", "Fuzz", "Example"} {
		if rest, ok := strings.CutPrefix(name, p); ok {
			return rest == "" || !(rest[0] >= 'a' && rest[0] <= 'z')
		}
	}
	return false
}

// rustTestAttr looks at the attributes and doc comments right above line
// (1-based) for #[test] or #[path::test].
func rustTestAttr(src []byte, line int) bool {
	lines := bytes.Split(src, []byte("\n"))
	for i := line - 2; i >= 0 && i < len(lines); i-- {
		l := strings.TrimSpace(string(lines[i]))
		switch {
		case l == "#[test]" || (strings.HasPrefix(l, "#[") && (strings.HasSuffix(l, "::test]") || strings.Contains(l, "::test("))):
			return true
		case strings.HasPrefix(l, "#[") || strings.HasPrefix(l, "///") || strings.HasPrefix(l, "//"):
			continue
		default:
			return false
		}
	}
	return false
}

// For builds the commands that run tests, one per Go module, Cargo
// package and one for pytest, in that order. root is the repository root
// (absolute); cwd is where the command will run, and paths in it are
// relative to that. Tests that aren't Go, Python or Rust are left out;
// pass only symbols IsTest accepts.
func For(root, cwd string, tests []ref.Target) []Cmd {
	var gomods, crates []string
	gos := map[string][]ref.Target{}
	rusts := map[string][]ref.Target{}
	var pys []ref.Target
	for _, t := range tests {
		abs := filepath.Join(root, filepath.FromSlash(t.Path))
		switch {
		case strings.HasSuffix(t.Path, ".go"):
			if m := up(root, filepath.Dir(abs), "go.mod", nil); m != "" {
				if gos[m] == nil {
					gomods = append(gomods, m)
				}
				gos[m] = append(gos[m], t)
			}
		case strings.HasSuffix(t.Path, ".py"):
			pys = append(pys, t)
		case strings.HasSuffix(t.Path, ".rs"):
			if c := up(root, filepath.Dir(abs), "Cargo.toml", []byte("[package]")); c != "" {
				if rusts[c] == nil {
					crates = append(crates, c)
				}
				rusts[c] = append(rusts[c], t)
			}
		}
	}
	var out []Cmd
	for _, m := range gomods {
		out = append(out, goCmd(root, cwd, m, gos[m]))
	}
	for _, c := range crates {
		out = append(out, cargoCmd(root, cwd, c, rusts[c]))
	}
	if len(pys) > 0 {
		out = append(out, pytestCmd(root, cwd, pys))
	}
	return out
}

// up finds the nearest directory from dir up to root that holds file
// (containing want, if given). "" if there's none.
func up(root, dir, file string, want []byte) string {
	for {
		if data, ok := outline.Read(filepath.Join(dir, file)); ok && (want == nil || bytes.Contains(data, want)) {
			return dir
		}
		if dir == root || !within(root, dir) {
			return ""
		}
		dir = filepath.Dir(dir)
	}
}

func within(base, p string) bool {
	r, err := filepath.Rel(base, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator))
}

// rel is p relative to base as a path a runner takes: ".", "./x/" or
// "../x/" (with dirSlash), else "x" or "../x".
func rel(base, p string, dirSlash bool) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		r = p
	}
	r = filepath.ToSlash(r)
	if !dirSlash {
		return r
	}
	switch {
	case r == ".":
		return "."
	case strings.HasPrefix(r, "../"), r == "..":
		return r + "/"
	}
	return "./" + r + "/"
}

// goCmd: go test -run '^(A|B)$' ./dir/ ... in the module. -run applies to
// every listed package, so with several packages a test of the same name
// in another one runs too; that's checked and said.
func goCmd(root, cwd, mod string, ts []ref.Target) Cmd {
	sel := goSelect(root, ts)
	var args []string
	base := cwd
	if within(mod, cwd) {
		args = []string{"go", "test", "-run", sel.re}
	} else {
		args = []string{"go", "-C", rel(cwd, mod, false), "test", "-run", sel.re}
		base = mod
	}
	for _, d := range sel.dirs {
		args = append(args, rel(base, d, true))
	}
	return Cmd{Args: args, Tests: sel.n, Caveat: sel.caveat}
}

// goSelection is what a -run filter selects for some Go tests: the regexp,
// the package directories (absolute, sorted), how many distinct tests, and
// the caveat when a test of the same name in another listed package runs
// too.
type goSelection struct {
	re     string
	dirs   []string
	n      int
	caveat string
}

func goSelect(root string, ts []ref.Target) goSelection {
	byDir := map[string]map[string]bool{}
	var dirs, names []string
	for _, t := range ts {
		d := filepath.Dir(filepath.Join(root, filepath.FromSlash(t.Path)))
		if byDir[d] == nil {
			byDir[d] = map[string]bool{}
			dirs = append(dirs, d)
		}
		if !byDir[d][t.Name] {
			byDir[d][t.Name] = true
			if !slices.Contains(names, t.Name) {
				names = append(names, t.Name)
			}
		}
	}
	slices.Sort(dirs)
	slices.Sort(names)
	sel := goSelection{re: "^" + names[0] + "$", dirs: dirs}
	if len(names) > 1 {
		sel.re = "^(" + strings.Join(names, "|") + ")$"
	}
	for _, set := range byDir {
		sel.n += len(set)
	}
	if len(dirs) > 1 {
		wanted := map[string]bool{}
		for _, nm := range names {
			wanted[nm] = true
		}
		for _, d := range dirs {
			for _, nm := range goTests(d) {
				if wanted[nm] && !byDir[d][nm] {
					sel.caveat = "also runs tests of the same name in the other packages"
				}
			}
		}
	}
	return sel
}

// goTests lists the test functions in a directory's _test.go files.
func goTests(dir string) []string {
	files, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	var out []string
	for _, f := range files {
		data, ok := outline.Read(f)
		if !ok {
			continue
		}
		name := filepath.Base(f)
		syms := outline.Parse(name, data, outline.MaxSize)
		for _, t := range ref.Targets(name, syms) {
			if IsTest(t, nil) {
				out = append(out, t.Name)
			}
		}
	}
	return out
}

// pytestCmd: pytest with node ids, exact.
func pytestCmd(root, cwd string, ts []ref.Target) Cmd {
	var ids []string
	for _, t := range ts {
		id := rel(cwd, filepath.Join(root, filepath.FromSlash(t.Path)), false) + "::" + strings.ReplaceAll(t.Name, ".", "::")
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return Cmd{Args: append([]string{"pytest"}, ids...), Tests: len(ids)}
}

// cargoCmd: cargo test [--manifest-path] [--test x] -- --exact path...
// Test paths are the module path from the file's place in the crate plus
// the qualified name. --exact makes each filter match one test path in
// each target; a crate with both a library and a binary root can have
// the same path in both.
func cargoCmd(root, cwd, crate string, ts []ref.Target) Cmd {
	var filters []string
	targets := map[string]bool{}
	for _, t := range ts {
		r := rel(crate, filepath.Join(root, filepath.FromSlash(t.Path)), false)
		mod, target := rustModule(r)
		targets[target] = true
		f := strings.ReplaceAll(t.Name, ".", "::")
		if mod != "" {
			f = mod + "::" + f
		}
		if !slices.Contains(filters, f) {
			filters = append(filters, f)
		}
	}
	slices.Sort(filters)
	args := []string{"cargo", "test"}
	if !within(crate, cwd) {
		args = append(args, "--manifest-path", rel(cwd, filepath.Join(crate, "Cargo.toml"), false))
	}
	c := Cmd{Tests: len(filters)}
	if len(targets) == 1 && !targets[""] {
		for t := range targets {
			args = append(args, "--test", t)
		}
	} else if exists(filepath.Join(crate, "src", "lib.rs")) && exists(filepath.Join(crate, "src", "main.rs")) {
		c.Caveat = "also runs tests with the same path in the crate's other targets"
	}
	c.Args = append(append(args, "--", "--exact"), filters...)
	return c
}

// rustModule turns a crate-relative file path into its module path, and
// the integration test target for files under tests/ ("" otherwise):
// src/lib.rs "", src/a/b.rs "a::b", src/a/mod.rs "a", tests/x.rs "" in x.
func rustModule(p string) (mod, target string) {
	p = strings.TrimSuffix(p, ".rs")
	parts := strings.Split(p, "/")
	switch {
	case len(parts) >= 2 && parts[0] == "tests":
		target = parts[1]
		parts = parts[2:]
	case len(parts) >= 1 && parts[0] == "src":
		parts = parts[1:]
		if len(parts) >= 2 && parts[0] == "bin" {
			parts = parts[2:] // a binary's root (src/bin/x.rs or src/bin/x/main.rs)
		}
	default:
		return "", ""
	}
	if n := len(parts); n > 0 && (parts[n-1] == "mod" || (n == 1 && (parts[0] == "main" || parts[0] == "lib"))) {
		parts = parts[:n-1] // a module's mod.rs, or a target's root file
	}
	return strings.Join(parts, "::"), target
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
