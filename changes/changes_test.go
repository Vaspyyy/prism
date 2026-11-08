package changes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Vaspyyy/prism/repo"
)

func isolate(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CEILING_DIRECTORIES": base,
		"GIT_AUTHOR_NAME": "T", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "T", "GIT_COMMITTER_EMAIL": "t@example.com",
	} {
		t.Setenv(k, v)
	}
	return base
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, p, content string) {
	t.Helper()
	full := filepath.Join(dir, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const shop = `package shop

func init() {}

func init() {}

type Repo struct{}

func (r *Repo) Open() error {
	x := 1
	y := 2
	return nil
}

func Close() {}
`

func newRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(isolate(t), "r")
	os.MkdirAll(dir, 0o755)
	run(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "shop/shop.go", shop)
	write(t, dir, "README.md", "# Shop\n\n## Usage\n\nRun it.\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func compute(t *testing.T, dir string) *Set {
	t.Helper()
	info, err := repo.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Compute(info)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func refs(s *Set) []string {
	var out []string
	for _, sym := range s.Symbols {
		out = append(out, sym.Ref+" "+sym.Mark)
	}
	return out
}

func TestClean(t *testing.T) {
	s := compute(t, newRepo(t))
	if len(s.Files) != 0 || len(s.Symbols) != 0 || s.Skipped != "" || s.BranchBase != "" {
		t.Errorf("clean repo: %+v", s)
	}
}

func TestUncommitted(t *testing.T) {
	dir := newRepo(t)
	write(t, dir, "shop/shop.go", strings.Replace(shop, "y := 2", "y := 3", 1))
	write(t, dir, "notes.md", "# Notes\n\n## Todo\n")
	s := compute(t, dir)
	want := []string{"notes.md:Notes changed", "notes.md:Todo changed", "shop/shop.go:Repo.Open changed"}
	if got := refs(s); !reflect.DeepEqual(got, want) {
		t.Errorf("symbols = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(s.Files, []string{"notes.md", "shop/shop.go"}) {
		t.Errorf("files = %q", s.Files)
	}
	if s.Mark("shop/shop.go:Repo.Open") != Uncommitted || s.Mark("shop/shop.go:Close") != "" || !s.Changed("notes.md") || s.Changed("README.md") {
		t.Error("Mark/Changed disagree with Symbols")
	}
	if u, b := s.Count(); u != 3 || b != 0 {
		t.Errorf("Count = %d, %d", u, b)
	}
}

func TestStagedDeletionAndOrdinal(t *testing.T) {
	dir := newRepo(t)
	// delete a line inside Open, and change the second init
	src := strings.Replace(shop, "\ty := 2\n", "", 1)
	src = strings.Replace(src, "func init() {}\n\ntype", "func init() { _ = 1 }\n\ntype", 1)
	write(t, dir, "shop/shop.go", src)
	run(t, dir, "add", ".")
	want := []string{"shop/shop.go:init#2 changed", "shop/shop.go:Repo.Open changed"}
	if got := refs(compute(t, dir)); !reflect.DeepEqual(got, want) {
		t.Errorf("symbols = %q, want %q", got, want)
	}
}

func TestBranch(t *testing.T) {
	dir := newRepo(t)
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "shop/shop.go", strings.Replace(shop, "func Close() {}", "func Close() { _ = 0 }", 1))
	run(t, dir, "commit", "-q", "-am", "close")
	write(t, dir, "shop/shop.go", strings.Replace(strings.Replace(shop, "func Close() {}", "func Close() { _ = 0 }", 1), "x := 1", "x := 9", 1))
	s := compute(t, dir)
	want := []string{"shop/shop.go:Repo.Open changed", "shop/shop.go:Close branch"}
	if got := refs(s); !reflect.DeepEqual(got, want) {
		t.Errorf("symbols = %q, want %q", got, want)
	}
	if s.BranchBase == "" {
		t.Error("no branch base on a feature branch")
	}
	if s.FileMark("shop/shop.go") != Uncommitted {
		t.Errorf("a file with uncommitted changes is %q", s.FileMark("shop/shop.go"))
	}
	if u, b := s.Count(); u != 1 || b != 1 {
		t.Errorf("Count = %d, %d", u, b)
	}
	// on the default branch itself, only uncommitted changes count
	run(t, dir, "stash", "-q")
	if s := compute(t, dir); s.FileMark("shop/shop.go") != Branch || s.FileMark("README.md") != "" {
		t.Errorf("committed on the branch only: %q", s.FileMark("shop/shop.go"))
	}
	run(t, dir, "checkout", "-q", "main")
	if s := compute(t, dir); s.BranchBase != "" || len(s.Symbols) != 0 {
		t.Errorf("on main: %+v", s)
	}
}

func TestUnbornAndOutsideGit(t *testing.T) {
	base := isolate(t)
	dir := filepath.Join(base, "u")
	os.MkdirAll(dir, 0o755)
	run(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "a.go", "package a\n\nfunc A() {}\n")
	want := []string{"a.go:A changed"}
	if got := refs(compute(t, dir)); !reflect.DeepEqual(got, want) {
		t.Errorf("unborn: %q, want %q", got, want)
	}
	plain := filepath.Join(base, "plain")
	os.MkdirAll(plain, 0o755)
	if s := compute(t, plain); s != nil {
		t.Errorf("outside git: %+v", s)
	}
	var none *Set
	if none.Mark("a.go:A") != "" || none.Changed("a.go") {
		t.Error("a nil set should mark nothing")
	}
}

func TestTooManyFiles(t *testing.T) {
	dir := newRepo(t)
	for i := 0; i <= MaxFiles; i++ {
		write(t, dir, fmt.Sprintf("gen/f%03d.go", i), "package gen\n\nfunc F() {}\n")
	}
	s := compute(t, dir)
	if s.Skipped != fmt.Sprintf("%d changed files", MaxFiles+1) || len(s.Symbols) != 0 || len(s.Files) != MaxFiles+1 {
		t.Errorf("too many files: skipped %q, %d symbols, %d files", s.Skipped, len(s.Symbols), len(s.Files))
	}
}

func TestNewSide(t *testing.T) {
	for h, want := range map[string][3]int{
		"@@ -1,2 +3,4 @@ func": {3, 4, 1},
		"@@ -5 +5 @@":          {5, 1, 1},
		"@@ -5,2 +4,0 @@":      {4, 0, 1},
		"@@ -0,0 +1,10 @@":     {1, 10, 1},
		"@@ broken":            {0, 0, 0},
	} {
		s, n, ok := newSide(h)
		if s != want[0] || n != want[1] || ok != (want[2] == 1) {
			t.Errorf("newSide(%q) = %d, %d, %v", h, s, n, ok)
		}
	}
	if got := unquote(`"b/tab\there.go"`); got != "b/tab\there.go" {
		t.Errorf("unquote = %q", got)
	}
}

func TestOptions(t *testing.T) {
	dir := newRepo(t)
	run(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "README.md", "# Shop\n\n## Usage\n\nRun it twice.\n")
	run(t, dir, "commit", "-q", "-am", "readme")
	write(t, dir, "shop/shop.go", strings.Replace(shop, "x := 1", "x := 9", 1))
	write(t, dir, "shop/new.go", "package shop\n\nfunc New() {}\n")
	info, err := repo.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	with := func(o Options) *Set {
		t.Helper()
		s, err := ComputeWith(info, o)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	// a path limits files, symbols and the branch part
	s := with(Options{Paths: []string{"shop/shop.go"}})
	if !reflect.DeepEqual(s.Files, []string{"shop/shop.go"}) || !reflect.DeepEqual(refs(s), []string{"shop/shop.go:Repo.Open changed"}) {
		t.Errorf("one path: files %q, symbols %q", s.Files, refs(s))
	}
	if s := with(Options{Paths: []string{"shop"}}); !reflect.DeepEqual(s.Files, []string{"shop/new.go", "shop/shop.go"}) {
		t.Errorf("a directory: %q", s.Files)
	}
	if s := with(Options{Paths: []string{"README.md"}}); s.FileMark("README.md") != Branch || s.BranchBase == "" || len(s.Files) != 1 {
		t.Errorf("branch file under a path: %+v", s)
	}
	// files only: marks for files, no symbols
	s = with(Options{FilesOnly: true, NoLocks: true})
	if len(s.Symbols) != 0 || s.FileMark("README.md") != Branch || s.FileMark("shop/new.go") != Uncommitted || len(s.Files) != 3 {
		t.Errorf("files only: files %q, symbols %q", s.Files, refs(s))
	}
	// the whole set, for comparison
	want := []string{"README.md:Usage branch", "shop/new.go:New changed", "shop/shop.go:Repo.Open changed"}
	if got := refs(with(Options{})); !reflect.DeepEqual(got, want) {
		t.Errorf("all: %q, want %q", got, want)
	}
}

func TestSymlinkNotRead(t *testing.T) {
	dir := newRepo(t)
	// an untracked symlink to an endless device, and one to a file outside
	if err := os.Symlink("/dev/zero", filepath.Join(dir, "zero.go")); err != nil {
		t.Skip("no symlinks:", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.go")
	os.WriteFile(outside, []byte("package secret\n\nfunc Secret() {}\n"), 0o644)
	os.Symlink(outside, filepath.Join(dir, "secret.go"))
	done := make(chan *Set)
	go func() { done <- compute(t, dir) }()
	select {
	case s := <-done:
		if len(s.Symbols) != 0 || !s.Changed("zero.go") || !s.Changed("secret.go") {
			t.Errorf("symlinks: files %q, symbols %q", s.Files, refs(s))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Compute hangs on a symlink to /dev/zero")
	}
}
