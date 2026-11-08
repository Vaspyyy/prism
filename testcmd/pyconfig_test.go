package testcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/ref"
)

// TestRerunPytestReal checks Rerun's config pins against pytest itself,
// where it's installed: a run and its rerun report the same rootdir and
// config file.
func TestRerunPytestReal(t *testing.T) {
	if testing.Short() {
		t.Skip("runs pytest")
	}
	if _, err := exec.LookPath("pytest"); err != nil {
		t.Skip("pytest isn't installed")
	}
	cov := exec.Command("pytest", "-p", "pytest_cov.plugin", "--version")
	cov.Env = pytestEnv()
	withCov := cov.Run() == nil

	test := "def test_a():\n    assert False\n"
	nested := t.TempDir()
	write(t, nested, "pyproject.toml", "[tool.pytest.ini_options]\naddopts = \"-p no:cacheprovider\"\n")
	write(t, nested, "packages/foo/pyproject.toml", "[project]\nname = \"foo\"\n\n[tool.pytest.ini_options]\naddopts = \"--strict-markers\"\n")
	write(t, nested, "packages/foo/tests/test_x.py", test)
	write(t, nested, "packages/bar/tests/test_y.py", test)
	bare := t.TempDir()
	write(t, bare, "a/setup.py", "")
	write(t, bare, "a/test_x.py", test)
	write(t, bare, "b/test_y.py", test)
	write(t, bare, "c/pytest.ini", "[pytest]\n")
	write(t, bare, "c/test_z.py", test)
	ini := t.TempDir()
	write(t, ini, "tox.ini", "[tox]\n\n[pytest]\naddopts = -rA\n")
	write(t, ini, "sub/setup.cfg", "[metadata]\nname = sub\n\n[tool:pytest]\naddopts = -v\n")
	write(t, ini, "sub/test_x.py", test)
	plain := t.TempDir()
	write(t, plain, "pyproject.toml", "[project]\nname = \"top\"\n")
	write(t, plain, "sub/pyproject.toml", "[project]\nname = \"sub\"\n")
	write(t, plain, "sub/test_x.py", test)

	foo := ref.Target{Path: "packages/foo/tests/test_x.py", Name: "test_a", Kind: "def"}
	x := ref.Target{Path: "sub/test_x.py", Name: "test_a", Kind: "def"}
	for _, c := range []struct {
		root, cwd, argv string
		failed          ref.Target
		pin             string // what the rerun adds
	}{
		{nested, ".", "pytest -x", foo, "-c pyproject.toml"},
		{nested, ".", "pytest packages", foo, "-c pyproject.toml"},
		{nested, "packages", "pytest", foo, "-c ../pyproject.toml"},
		{nested, ".", "pytest packages/foo/tests", foo, ""},
		{nested, "packages/foo", "pytest", foo, ""},
		{nested, ".", "pytest --rootdir=. packages", foo, "-c pyproject.toml"},
		{nested, ".", "pytest -p pytest_cov.plugin --cov packages/foo", foo, ""},
		{nested, ".", "pytest -p pytest_cov.plugin --cov=packages/foo", foo, "-c pyproject.toml"},
		{bare, ".", "pytest a b", ref.Target{Path: "a/test_x.py", Name: "test_a", Kind: "def"}, "--rootdir ."},
		{bare, ".", "pytest b c", ref.Target{Path: "b/test_y.py", Name: "test_a", Kind: "def"}, "-c c/pytest.ini"},
		{bare, ".", "pytest b c", ref.Target{Path: "c/test_z.py", Name: "test_a", Kind: "def"}, ""},
		{ini, ".", "pytest", x, "-c tox.ini"},
		{plain, ".", "pytest", x, "-c pyproject.toml"},
	} {
		t.Run(c.cwd+" "+c.argv+" "+c.failed.Path, func(t *testing.T) {
			if strings.Contains(c.argv, "pytest_cov") && !withCov {
				t.Skip("pytest-cov isn't installed")
			}
			t.Parallel()
			cwd := filepath.Join(c.root, c.cwd)
			argv := strings.Fields(c.argv)
			cmd, ok := Rerun(c.root, cwd, argv, []Failed{{Target: c.failed}})
			if !ok {
				t.Fatal("no rerun")
			}
			if !strings.Contains(cmd.String(), " "+c.pin+" ") && c.pin != "" || c.pin == "" && strings.Contains(cmd.String(), "-c ") {
				t.Errorf("rerun %q, want it to pin %q", cmd.String(), c.pin)
			}
			if run, re := pytestHeader(t, cwd, argv[1:]), pytestHeader(t, cwd, cmd.Args[1:]); run != re {
				t.Errorf("run:   %s\nrerun: %s (%s)", run, re, cmd.String())
			}
		})
	}
}

func pytestEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PYTEST_ADDOPTS=") {
			env = append(env, e)
		}
	}
	return append(env, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1", "PYTHONDONTWRITEBYTECODE=1")
}

// pytestHeader is the rootdir and configfile lines pytest prints for args.
func pytestHeader(t *testing.T, dir string, args []string) string {
	t.Helper()
	cmd := exec.Command("pytest", append(append([]string{}, args...), "--co", "-p", "no:cacheprovider")...)
	cmd.Dir = dir
	cmd.Env = pytestEnv()
	out, _ := cmd.CombinedOutput()
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(l, "rootdir: ") || strings.HasPrefix(l, "configfile: ") {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		t.Fatalf("pytest %s: no rootdir\n%s", strings.Join(args, " "), out)
	}
	return strings.Join(lines, ", ")
}
