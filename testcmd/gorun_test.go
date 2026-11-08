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
// filter that runs those subtests and their parents, and no other.
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
	root := t.TempDir()
	write(t, root, "go.mod", "module sub\n\ngo "+strings.Fields(v)[0]+"\n")
	write(t, root, "sub_test.go", `package sub

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
}
`)
	argv := []string{"go", "test", "-count=1", "-v", "./..."}
	_, failed := goRun(t, root, argv)
	want := []string{"TestA/a+b(c)[1]|x.*", "TestA/bad", "TestA/bad#01", "TestA/outer/inner", "TestA/p/q", "TestA/with_space", "TestB/bad"}
	if !slices.Equal(failed, want) {
		t.Fatalf("go test printed failures %q, want %q", failed, want)
	}

	for _, c := range []struct {
		printed []string
		ran     []string // besides the printed ones
	}{
		{[]string{"TestA/bad"}, []string{"TestA"}},
		{[]string{"TestA/bad#01", "TestA/with_space", "TestA/a+b(c)[1]|x.*"}, []string{"TestA"}},
		{[]string{"TestA/p/q"}, []string{"TestA"}},
		{[]string{"TestA/outer/inner"}, []string{"TestA", "TestA/outer"}},
		// the parent tests, and so their passing subtests
		{[]string{"TestA/outer/inner", "TestB/bad"}, []string{"TestA", "TestB", "TestB/ok", "TestA/ok", "TestA/outer", "TestA/outer/fine",
			"TestA/bad", "TestA/bad#01", "TestA/with_space", "TestA/a+b(c)[1]|x.*", "TestA/p/q"}},
	} {
		var fs []Failed
		for _, p := range c.printed {
			top, _, _ := strings.Cut(p, "/")
			fs = append(fs, Failed{Target: ref.Target{Path: "sub_test.go", Name: top, Kind: "func"}, Printed: p})
		}
		cmd, ok := Rerun(root, root, argv, fs)
		if !ok {
			t.Errorf("%q: no rerun", c.printed)
			continue
		}
		ran, failed := goRun(t, root, cmd.Args)
		wantRan := append(slices.Clone(c.printed), c.ran...)
		slices.Sort(wantRan)
		wantFailed := slices.Sorted(slices.Values(c.printed))
		if cmd.Caveat != "" {
			wantFailed = want
		}
		if !slices.Equal(ran, wantRan) || !slices.Equal(failed, wantFailed) {
			t.Errorf("%s\n ran %q\nwant %q\n failed %q\n  want %q", cmd, ran, wantRan, failed, wantFailed)
		}
		if cmd.Tests != len(c.printed) {
			t.Errorf("%s: %d tests, want %d", cmd, cmd.Tests, len(c.printed))
		}
	}
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
