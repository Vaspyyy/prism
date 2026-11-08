package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// isolate keeps the user's git configuration (signing, hooks, default
// branch) out of the tests and stops git from finding a repository above
// the temp dirs.
func isolate(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", base)
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	return base
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository on branch b with one commit: a.go and
// internal/files/files.go.
func newRepo(t *testing.T, dir, b string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "init", "-q", "-b", b)
	write(t, filepath.Join(dir, "a.go"), "package a\n")
	write(t, filepath.Join(dir, "internal/files/files.go"), "package files\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func detect(t *testing.T, dir string) *Info {
	t.Helper()
	in, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestDetectRepo(t *testing.T) {
	root := newRepo(t, filepath.Join(isolate(t), "r"), "main")
	head := run(t, root, "rev-parse", "HEAD")
	in := detect(t, filepath.Join(root, "internal", "files"))
	want := &Info{Root: root, Git: true, GitDir: filepath.Join(root, ".git"), CommonDir: filepath.Join(root, ".git"),
		Head: head, Branch: "main", DefaultBranch: "main", DefaultRef: "refs/heads/main"}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("got  %+v\nwant %+v", in, want)
	}
}

func TestDetectUnbornAndDetached(t *testing.T) {
	base := isolate(t)
	unborn := filepath.Join(base, "u")
	if err := os.MkdirAll(unborn, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, unborn, "init", "-q", "-b", "trunk")
	in := detect(t, unborn)
	if !in.Git || in.Head != "" || in.Branch != "trunk" || in.DefaultBranch != "" {
		t.Errorf("unborn: %+v", in)
	}

	root := newRepo(t, filepath.Join(base, "d"), "main")
	run(t, root, "checkout", "-q", "--detach")
	in = detect(t, root)
	if in.Branch != "" || len(in.Head) != 40 || in.DefaultBranch != "main" {
		t.Errorf("detached: %+v", in)
	}
}

func TestDetectWorktree(t *testing.T) {
	base := isolate(t)
	root := newRepo(t, filepath.Join(base, "main"), "main")
	wt := filepath.Join(base, "wt")
	run(t, root, "worktree", "add", "-q", "-b", "feature", wt)
	in := detect(t, wt)
	if in.Root != wt || in.Branch != "feature" || in.DefaultBranch != "main" {
		t.Errorf("worktree: %+v", in)
	}
	if in.GitDir != filepath.Join(root, ".git", "worktrees", "wt") || in.CommonDir != filepath.Join(root, ".git") {
		t.Errorf("worktree git dirs: %q, %q", in.GitDir, in.CommonDir)
	}
}

func TestDefaultBranchFromRemote(t *testing.T) {
	base := isolate(t)
	origin := newRepo(t, filepath.Join(base, "origin"), "trunk")
	run(t, base, "clone", "-q", origin, "clone")
	clone := filepath.Join(base, "clone")
	run(t, clone, "checkout", "-q", "-b", "feature")
	in := detect(t, clone)
	if in.DefaultBranch != "trunk" || in.DefaultRef != "refs/remotes/origin/trunk" {
		t.Errorf("with origin/HEAD: %q %q", in.DefaultBranch, in.DefaultRef)
	}

	// a remote added by hand usually has no origin/HEAD: fall back to
	// origin/main before the local branch
	origin2 := newRepo(t, filepath.Join(base, "origin2"), "main")
	run(t, base, "clone", "-q", origin2, "clone2")
	clone2 := filepath.Join(base, "clone2")
	run(t, clone2, "remote", "set-head", "origin", "-d")
	in = detect(t, clone2)
	if in.DefaultBranch != "main" || in.DefaultRef != "refs/remotes/origin/main" {
		t.Errorf("without origin/HEAD: %q %q", in.DefaultBranch, in.DefaultRef)
	}
}

func TestDetectOutsideGit(t *testing.T) {
	dir := filepath.Join(isolate(t), "plain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	in := detect(t, dir)
	if !reflect.DeepEqual(in, &Info{Root: dir}) {
		t.Errorf("outside git: %+v", in)
	}
	if d, err := in.Dirty(); d || err != nil {
		t.Errorf("Dirty outside git: %v %v", d, err)
	}
	if _, err := Detect(filepath.Join(dir, "missing")); err == nil {
		t.Error("Detect of a missing directory should fail")
	}
}

func TestDirty(t *testing.T) {
	root := newRepo(t, filepath.Join(isolate(t), "r"), "main")
	in := detect(t, root)
	check := func(want bool, when string) {
		t.Helper()
		if d, err := in.Dirty(); err != nil || d != want {
			t.Errorf("%s: Dirty = %v, %v; want %v", when, d, err, want)
		}
	}
	check(false, "clean")
	write(t, filepath.Join(root, "new.go"), "package a\n")
	check(true, "untracked file")
	run(t, root, "add", ".")
	run(t, root, "commit", "-q", "-m", "second")
	check(false, "committed")
	write(t, filepath.Join(root, "a.go"), "package a // changed\n")
	check(true, "modified file")
}

func TestRel(t *testing.T) {
	base := isolate(t)
	root := newRepo(t, filepath.Join(base, "r"), "main")
	link := filepath.Join(base, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	in := detect(t, link)
	if in.Root != root {
		t.Fatalf("Root through a symlink: %q, want %q", in.Root, root)
	}
	for _, c := range []struct {
		p, want string
		ok      bool
	}{
		{filepath.Join(root, "internal/files/files.go"), "internal/files/files.go", true},
		{filepath.Join(link, "internal/files/files.go"), "internal/files/files.go", true},
		{filepath.Join(link, "internal/new/not-yet.go"), "internal/new/not-yet.go", true},
		{root, ".", true},
		{base, "", false},
		{filepath.Join(base, "rx/a.go"), "", false},
	} {
		got, ok := in.Rel(c.p)
		if got != c.want || ok != c.ok {
			t.Errorf("Rel(%q) = %q, %v; want %q, %v", c.p, got, ok, c.want, c.ok)
		}
	}
	if got := in.Abs("internal/files/files.go"); got != filepath.Join(root, "internal", "files", "files.go") {
		t.Errorf("Abs: %q", got)
	}
}

func TestResolve(t *testing.T) {
	root := newRepo(t, filepath.Join(isolate(t), "r"), "main")
	write(t, filepath.Join(root, "internal/a.go"), "package internal\n")    // shadows a.go from internal/
	write(t, filepath.Join(root, "internal/only.go"), "package internal\n") // exists only cwd-relative
	in := detect(t, root)
	cwd := filepath.Join(root, "internal")
	for _, c := range []struct {
		p    string
		want Resolved
		err  error
	}{
		{"a.go", Resolved{Rel: "a.go", Exists: true, Shadowed: "internal/a.go"}, nil},
		{"only.go", Resolved{Rel: "internal/only.go", Exists: true}, nil},
		{"internal/files/files.go", Resolved{Rel: "internal/files/files.go", Exists: true}, nil},
		{"files/files.go", Resolved{Rel: "internal/files/files.go", Exists: true}, nil},
		{"./files/../only.go", Resolved{Rel: "internal/only.go", Exists: true}, nil},
		{"gone.go", Resolved{Rel: "gone.go"}, nil},
		{filepath.Join(root, "a.go"), Resolved{Rel: "a.go", Exists: true}, nil},
		{"../../elsewhere.go", Resolved{}, ErrOutside},
		{"/etc/passwd", Resolved{}, ErrOutside},
	} {
		got, err := in.Resolve(c.p, cwd)
		if got != c.want || err != c.err {
			t.Errorf("Resolve(%q) = %+v, %v; want %+v, %v", c.p, got, err, c.want, c.err)
		}
	}
}

func TestMatch(t *testing.T) {
	root := newRepo(t, filepath.Join(isolate(t), "r"), "main")
	in := detect(t, root)
	// a diff's files, including ones that no longer exist
	files := []string{"a.go", "internal/files/files.go", "cmd/files/files.go", "internal/old/removed.go"}
	cwd := filepath.Join(root, "internal")
	for _, c := range []struct {
		p    string
		want []string
	}{
		{"internal/files/files.go", []string{"internal/files/files.go"}},
		{"files/files.go", []string{"internal/files/files.go"}}, // cwd reading beats the suffix rule
		{"old/removed.go", []string{"internal/old/removed.go"}}, // deleted, named from cwd
		{"removed.go", []string{"internal/old/removed.go"}},
		{"files.go", []string{"internal/files/files.go", "cmd/files/files.go"}}, // ambiguous
		{"a.go", []string{"a.go"}},
		{"iles.go", nil}, // only at a path boundary
		{"../../x.go", nil},
	} {
		if got := in.Match(c.p, cwd, files); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Match(%q) = %q, want %q", c.p, got, c.want)
		}
	}
}
