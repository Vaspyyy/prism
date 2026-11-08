package testcmd

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/ref"
)

// TestRerunGoSubtestsReal checks subtest reruns against go test itself:
// the names it prints for failed subtests (with_space for "with space",
// bad#01 for a second "bad", p/q for a name with a slash) give a -run
// filter that runs those subtests and their parents, and no other, also
// for subtests of several tests at several depths. A test that failed on
// its own reruns whole, as does each parent in a module for a go before
// 1.18, which the caveat says.
func TestRerunGoSubtestsReal(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go isn't installed")
	}
	v, ok := strings.CutPrefix(runtime.Version(), "go")
	if !ok {
		t.Skip("a development go: no version for go.mod")
	}
	const tests = `package sub

import "testing"

func fail(t *testing.T) { t.Fail() }

func pass(t *testing.T) {}

func TestA(t *testing.T) {
	t.Run("ok", pass)
	t.Run("bad", fail)
	t.Run("bad", fail)
	t.Run("with space", fail)
	t.Run("a+b(c)[1]|x.*", fail)
	t.Run("p/q", fail)
	t.Run("outer", func(t *testing.T) {
		t.Run("inner", fail)
		t.Run("fine", pass)
	})
}

func TestB(t *testing.T) {
	t.Run("ok", pass)
	t.Run("bad", fail)
	t.Run("a+", func(t *testing.T) { t.Run("in", fail) })
}

func TestC(t *testing.T) {
	t.Run("ok", pass)
	t.Run("bad", fail)
	t.Fail()
}
`
	argv := []string{"go", "test", "-count=1", "-v", "./..."}
	want := []string{"TestA/a+b(c)[1]|x.*", "TestA/bad", "TestA/bad#01", "TestA/outer/inner", "TestA/p/q", "TestA/with_space", "TestB/a+/in", "TestB/bad", "TestC/bad"}
	allA := []string{"TestA", "TestA/ok", "TestA/bad", "TestA/bad#01", "TestA/with_space", "TestA/a+b(c)[1]|x.*", "TestA/p/q", "TestA/outer", "TestA/outer/inner", "TestA/outer/fine"}
	for _, m := range []struct {
		goVersion string
		cases     []struct {
			printed []string
			ran     []string // besides the printed ones
		}
	}{
		{strings.Fields(v)[0], []struct {
			printed []string
			ran     []string
		}{
			{[]string{"TestA/bad"}, []string{"TestA"}},
			{[]string{"TestA/bad#01", "TestA/with_space", "TestA/a+b(c)[1]|x.*"}, []string{"TestA"}},
			{[]string{"TestA/p/q"}, []string{"TestA"}},
			{[]string{"TestA/outer/inner"}, []string{"TestA", "TestA/outer"}},
			// several parents and depths: alternatives
			{[]string{"TestA/outer/inner", "TestB/bad"}, []string{"TestA", "TestA/outer", "TestB"}},
			{[]string{"TestA/a+b(c)[1]|x.*", "TestA/outer/inner", "TestA/p/q", "TestB/bad"}, []string{"TestA", "TestA/outer", "TestB"}},
			{[]string{"TestB/a+/in"}, []string{"TestB", "TestB/a+"}},
			{want, []string{"TestA", "TestA/outer", "TestB", "TestB/a+", "TestC"}},
			// failed on its own: the whole test
			{[]string{"TestC", "TestC/bad", "TestB/bad"}, []string{"TestC/ok", "TestB"}},
		}},
		{"1.17", []struct {
			printed []string
			ran     []string
		}{
			{[]string{"TestA/bad", "TestA/with_space"}, []string{"TestA"}},
			// several parents: the parent tests
			{[]string{"TestA/outer/inner", "TestB/bad"}, append([]string{"TestB", "TestB/ok", "TestB/a+", "TestB/a+/in"}, allA...)},
		}},
	} {
		root := t.TempDir()
		write(t, root, "go.mod", "module sub\n\ngo "+m.goVersion+"\n")
		write(t, root, "sub_test.go", tests)
		_, failed := goRun(t, root, argv)
		if !slices.Equal(failed, want) {
			t.Fatalf("go %s: go test printed failures %q, want %q", m.goVersion, failed, want)
		}
		for _, c := range m.cases {
			var fs []Failed
			for _, p := range c.printed {
				top, _, _ := strings.Cut(p, "/")
				fs = append(fs, Failed{Target: ref.Target{Path: "sub_test.go", Name: top, Kind: "func"}, Printed: p})
			}
			cmd, ok := Rerun(root, root, argv, fs)
			if !ok {
				t.Errorf("go %s, %q: no rerun", m.goVersion, c.printed)
				continue
			}
			ran, failed := goRun(t, root, cmd.Args)
			wantRan := slices.Compact(slices.Sorted(slices.Values(append(slices.Clone(c.printed), c.ran...))))
			var wantFailed []string // what fails of what runs
			for _, f := range want {
				if slices.Contains(wantRan, f) {
					wantFailed = append(wantFailed, f)
				}
			}
			if !slices.Equal(ran, wantRan) || !slices.Equal(failed, wantFailed) {
				t.Errorf("go %s: %s\n ran %q\nwant %q\n failed %q\n  want %q", m.goVersion, cmd, ran, wantRan, failed, wantFailed)
			}
			if cmd.Tests != len(c.printed) {
				t.Errorf("%s: %d tests, want %d", cmd, cmd.Tests, len(c.printed))
			}
			exact := append(ancestors(c.printed), c.printed...)
			wide := slices.ContainsFunc(ran, func(r string) bool { return !slices.Contains(exact, r) })
			if wide != (cmd.Caveat != "") {
				t.Errorf("%s: caveat %q, runs more than the failed tests and their parents: %v", cmd, cmd.Caveat, wide)
			}
		}
	}
}

// ancestors lists the parents of go test names: TestA and TestA/outer for
// TestA/outer/inner.
func ancestors(names []string) []string {
	var out []string
	for _, n := range names {
		for i := range len(n) {
			if n[i] == '/' {
				out = append(out, n[:i])
			}
		}
	}
	return out
}

var (
	goRunRe  = regexp.MustCompile(`^=== RUN\s+(\S+)`)
	goFailRe = regexp.MustCompile(`^\s*--- FAIL: (\S+)`)
)

// goRun runs a go test command in dir and returns the tests that ran and
// the failed ones without a failed subtest, as go test -v printed them,
// sorted.
func goRun(t *testing.T, dir string, argv []string) (ran, failed []string) {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	out, _ := cmd.CombinedOutput()
	var fails []string
	for _, l := range strings.Split(string(out), "\n") {
		if m := goRunRe.FindStringSubmatch(l); m != nil {
			ran = append(ran, m[1])
		}
		if m := goFailRe.FindStringSubmatch(l); m != nil {
			fails = append(fails, m[1])
		}
	}
	if len(ran) == 0 {
		t.Fatalf("%s ran no tests:\n%s", strings.Join(argv, " "), out)
	}
	for _, f := range fails {
		if !slices.ContainsFunc(fails, func(g string) bool { return strings.HasPrefix(g, f+"/") }) {
			failed = append(failed, f)
		}
	}
	slices.Sort(ran)
	slices.Sort(failed)
	return ran, failed
}
