// Package repo finds the repository a tool runs in, the same way for every
// tool: its root, git directories, HEAD, branch and default branch. It also
// turns paths typed on the command line into the repo-relative form that
// tools print, so a path printed by one tool is accepted by the others.
//
// Detect runs two git commands at the same time (three for a detached
// HEAD) and keeps nothing between runs, so what it reports is never stale.
// Anything slower (Dirty) is computed on demand.
package repo

import (
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Info is the repository a directory belongs to. Outside a git work tree
// only Root is set, to the directory itself.
type Info struct {
	Root      string // absolute root of the work tree, symlinks resolved (outside git: the directory)
	Git       bool   // inside a git work tree
	GitDir    string // this work tree's git dir: .git, or .git/worktrees/<name> in a linked worktree
	CommonDir string // the git dir all worktrees share
	Head      string // full commit hash of HEAD; "" on an unborn branch
	Branch    string // short branch name; "" when HEAD is detached

	// DefaultBranch is the branch work is compared against: the remote's
	// default (origin/HEAD), else main or master. "" if none is known.
	DefaultBranch string
	// DefaultRef is the full ref to compare with: the remote-tracking
	// branch when it exists (refs/remotes/origin/main), else the local one.
	DefaultRef string
}

// ErrOutside means a path lies outside the repository.
var ErrOutside = errors.New("outside the repository")

// Detect returns the repository containing dir. It only fails when dir
// itself can't be read; not being in a git repository (or git not being
// installed) is a normal result with Git false.
func Detect(dir string) (*Info, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, &os.PathError{Op: "detect", Path: abs, Err: errors.New("not a directory")}
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	// Two git calls, run at the same time: where the repository is, and its
	// branches. HEAD's branch is marked with '*' in the second.
	var refs string
	done := make(chan struct{})
	go func() {
		refs, _ = git(real, "for-each-ref", "--format=%(HEAD)%00%(refname)%00%(objectname)%00%(symref)",
			"refs/heads/", "refs/remotes/origin/HEAD", "refs/remotes/origin/main", "refs/remotes/origin/master")
		close(done)
	}()
	out, err := git(real, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	<-done
	lines := strings.Split(out, "\n")
	if err != nil || len(lines) != 3 {
		return &Info{Root: real}, nil
	}
	in := &Info{Root: lines[0], Git: true, GitDir: lines[1], CommonDir: lines[2]}
	have := map[string]bool{}
	originHead := ""
	for _, l := range strings.Split(refs, "\n") {
		f := strings.Split(l, "\x00")
		if len(f) != 4 {
			continue
		}
		have[f[1]] = true
		if f[0] == "*" {
			in.Branch, in.Head = strings.TrimPrefix(f[1], "refs/heads/"), f[2]
		}
		if f[1] == "refs/remotes/origin/HEAD" {
			originHead = f[3]
		}
	}
	if in.Branch == "" { // HEAD is detached, or on a branch with no commits yet
		in.Branch, _ = git(in.Root, "symbolic-ref", "-q", "--short", "HEAD")
		if in.Branch == "" {
			in.Head, _ = git(in.Root, "rev-parse", "-q", "--verify", "HEAD^{commit}")
		}
	}
	in.DefaultBranch, in.DefaultRef = defaultBranch(originHead, have)
	return in, nil
}

// defaultBranch picks the branch to compare against: origin/HEAD if the
// remote has one, else the first of origin/main, origin/master, main and
// master that exists.
func defaultBranch(originHead string, have map[string]bool) (name, ref string) {
	if originHead != "" {
		return strings.TrimPrefix(originHead, "refs/remotes/origin/"), originHead
	}
	for _, c := range []string{"refs/remotes/origin/main", "refs/remotes/origin/master", "refs/heads/main", "refs/heads/master"} {
		if have[c] {
			return path.Base(c), c
		}
	}
	return "", ""
}

// Dirty reports whether the work tree has uncommitted changes, untracked
// files included. It runs git status, so it's computed on demand rather
// than in Detect. Outside git it's always false.
func (in *Info) Dirty() (bool, error) {
	if !in.Git {
		return false, nil
	}
	out, err := git(in.Root, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// IsRev reports whether rev names a commit in this repository (a hash,
// branch, tag, HEAD~2, …). ref.Parse uses it to tell "main.go:run@b681ef1"
// from a name that contains '@'.
func (in *Info) IsRev(rev string) bool {
	if !in.Git || rev == "" || strings.HasPrefix(rev, "-") {
		return false
	}
	_, err := git(in.Root, "rev-parse", "-q", "--verify", "--end-of-options", rev+"^{commit}")
	return err == nil
}

// Abs returns the absolute path of a repo-relative path.
func (in *Info) Abs(rel string) string {
	return filepath.Join(in.Root, filepath.FromSlash(rel))
}

// Rel returns p (absolute, or relative to the process's working directory)
// relative to the root with forward slashes: the form tools print. The root
// itself is ".". ok is false when p lies outside the root.
func (in *Info) Rel(p string) (rel string, ok bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	if r, ok := inside(in.Root, abs); ok {
		return r, true
	}
	// a path through a symlink (the root is resolved, p may not be)
	if real, err := evalExisting(abs); err == nil {
		return inside(in.Root, real)
	}
	return "", false
}

func inside(root, abs string) (string, bool) {
	r, err := filepath.Rel(root, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(r), true
}

// evalExisting resolves symlinks in p, also when p itself doesn't exist
// (yet): the longest existing ancestor is resolved and the rest appended.
func evalExisting(p string) (string, error) {
	rest := ""
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(real, rest), nil
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// Readings returns the repo-relative paths that p, as typed on the command
// line, can mean, best first: relative to the root (the form tools print),
// then relative to cwd when that differs. An absolute path has one reading.
// Readings that leave the repository are dropped.
func (in *Info) Readings(p, cwd string) []string {
	if filepath.IsAbs(p) {
		if r, ok := in.Rel(p); ok {
			return []string{r}
		}
		return nil
	}
	var out []string
	if r, ok := inside(in.Root, filepath.Join(in.Root, p)); ok {
		out = append(out, r)
	}
	if cwd != "" {
		if !filepath.IsAbs(cwd) {
			if a, err := filepath.Abs(cwd); err == nil {
				cwd = a
			}
		}
		if r, ok := in.Rel(filepath.Join(cwd, p)); ok && (len(out) == 0 || out[0] != r) {
			out = append(out, r)
		}
	}
	return out
}

// Resolved is a path from the command line, made repo-relative.
type Resolved struct {
	Rel    string // repo-relative, forward slashes
	Exists bool
	// Shadowed is the cwd-relative reading when it also exists but lost to
	// the repo-relative one. Tools say so in their context line.
	Shadowed string
}

// Resolve turns a path typed on the command line into a repo-relative one.
// It accepts paths relative to the root, relative to cwd, and absolute
// ones. The first reading that exists wins (see Readings); if none exists,
// Rel is the best reading and Exists is false. A path that only leads
// outside the repository returns ErrOutside.
func (in *Info) Resolve(p, cwd string) (Resolved, error) {
	rs := in.Readings(p, cwd)
	if len(rs) == 0 {
		return Resolved{}, ErrOutside
	}
	for i, r := range rs {
		if !exists(in.Abs(r)) {
			continue
		}
		res := Resolved{Rel: r, Exists: true}
		if i == 0 && len(rs) > 1 && exists(in.Abs(rs[1])) {
			res.Shadowed = rs[1]
		}
		return res, nil
	}
	return Resolved{Rel: rs[0]}, nil
}

// Match finds which of files (repo-relative paths, existing or not, such
// as the files of a diff) p names: files equal to one of its readings, else
// files ending in it at a path boundary ("files.go" or "files/files.go" for
// "internal/files/files.go"). Several results mean p is ambiguous.
func (in *Info) Match(p, cwd string, files []string) []string {
	rs := in.Readings(p, cwd)
	for _, r := range rs {
		for _, f := range files {
			if f == r {
				return []string{f}
			}
		}
	}
	suffix := strings.TrimPrefix(path.Clean(filepath.ToSlash(p)), "./")
	if filepath.IsAbs(p) || suffix == "." || strings.HasPrefix(suffix, "../") {
		return nil
	}
	var out []string
	for _, f := range files {
		if f == suffix || strings.HasSuffix(f, "/"+suffix) {
			out = append(out, f)
		}
	}
	return out
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// git runs git in dir with English messages and without optional locks,
// so it never blocks (or is blocked by) another tool or agent working in
// the same repository.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}
