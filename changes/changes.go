// Package changes finds the symbols the current work changed: uncommitted
// changes (working tree and index against HEAD, untracked files included)
// and, on a branch other than the default one, what the branch committed
// since it split off. It's derived from git on every run and stores
// nothing, so it can't go stale or collide between agents (docs/context.md).
//
// It's deliberately light: changed lines are mapped to the innermost
// symbol containing them. Renames, moves and signature changes stay
// recap's job; both use the same outline, so the names agree.
package changes

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/repo"
)

// Marks.
const (
	Uncommitted = "changed" // changed in the working tree or index
	Branch      = "branch"  // committed on this branch since it left the default branch
)

// MaxFiles is how many changed files get symbol detail; beyond it the set
// only lists files and says so in Skipped (docs/context.md, "Stay fast").
const MaxFiles = 200

// emptyTree is git's empty tree, the base on an unborn branch.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// Set is the changed symbols of the current work.
type Set struct {
	Base       string   // what uncommitted changes are compared with: "HEAD"
	BranchBase string   // the merge base with the default branch, when on another branch
	Files      []string // changed files, repo-relative, sorted
	Symbols    []Symbol // changed symbols, by file and line
	Skipped    string   // why symbol detail was skipped ("812 changed files"); "" if it wasn't

	marks map[string]string // ref -> mark
	files map[string]string // path -> mark
}

// Symbol is a changed symbol.
type Symbol struct {
	Ref       string // with "#N" where the name is duplicated in its file
	Path      string
	Name      string
	Kind      string
	Line, End int
	Mark      string // Uncommitted or Branch
}

// Mark returns the mark of the symbol with this ref ("" if it didn't
// change).
func (s *Set) Mark(r string) string {
	if s == nil {
		return ""
	}
	return s.marks[r]
}

// Changed reports whether a file changed.
func (s *Set) Changed(path string) bool { return s.FileMark(path) != "" }

// FileMark returns the mark of a changed file: Uncommitted when it has
// uncommitted changes, else Branch; "" if it didn't change.
func (s *Set) FileMark(path string) string {
	if s == nil {
		return ""
	}
	return s.files[path]
}

// Count returns the number of changed symbols with each mark.
func (s *Set) Count() (uncommitted, branch int) {
	if s == nil {
		return 0, 0
	}
	for _, sym := range s.Symbols {
		if sym.Mark == Uncommitted {
			uncommitted++
		} else {
			branch++
		}
	}
	return
}

// Compute finds the current changes. Outside git it returns nil.
func Compute(info *repo.Info) (*Set, error) {
	if info == nil || !info.Git {
		return nil, nil
	}
	s := &Set{Base: "HEAD", marks: map[string]string{}, files: map[string]string{}}
	head := "HEAD"
	if info.Head == "" {
		head = emptyTree
	}
	if info.Branch != "" && info.DefaultRef != "" && info.Branch != info.DefaultBranch && info.Head != "" {
		if mb, err := git(info.Root, "merge-base", "HEAD", info.DefaultRef); err == nil && mb != "" && mb != info.Head {
			s.BranchBase = mb
		}
	}

	// which files changed: cheap, and decides whether symbol detail fits.
	//
	// git status rather than git diff: it lists untracked files in the same
	// pass, and, like any foreground git status, it writes back a refreshed
	// index when it can. With GIT_OPTIONAL_LOCKS=0 a stale index stays stale
	// and every run re-reads every file whose stat data changed (580 ms
	// instead of 13 ms in a 30,000-file repo). The lock is optional: if
	// another git command holds it, status skips the write.
	var wg sync.WaitGroup
	var status, branchNames string
	var errU, errB error
	wg.Add(1)
	go func() {
		defer wg.Done()
		status, errU = gitRun(info.Root, true, "status", "--porcelain", "-z", "--no-renames", "--untracked-files=all", "--ignore-submodules=dirty")
	}()
	if s.BranchBase != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			branchNames, errB = git(info.Root, "diff", "--name-only", "--no-renames", "-z", s.BranchBase, "HEAD")
		}()
	}
	wg.Wait()
	if errU != nil {
		return nil, errU
	}
	if errB != nil {
		s.BranchBase = "" // can't tell; keep the uncommitted part
	}
	var uncommittedFiles, untrackedFiles []string
	for _, e := range split0(status) {
		if len(e) < 4 {
			continue
		}
		if e[:2] == "??" {
			untrackedFiles = append(untrackedFiles, e[3:])
		} else {
			uncommittedFiles = append(uncommittedFiles, e[3:])
		}
	}
	branchFiles := split0(branchNames)
	for _, f := range branchFiles {
		s.files[f] = Branch
	}
	for _, group := range [][]string{uncommittedFiles, untrackedFiles} {
		for _, f := range group {
			s.files[f] = Uncommitted
		}
	}
	for f := range s.files {
		s.Files = append(s.Files, f)
	}
	slices.Sort(s.Files)
	if len(s.Files) == 0 {
		return s, nil
	}
	if len(s.Files) > MaxFiles {
		s.Skipped = strconv.Itoa(len(s.Files)) + " changed files"
		return s, nil
	}

	// changed lines, in working-tree line numbers: against HEAD for the
	// uncommitted part, against the merge base for everything on the branch.
	// Limited to the changed files, so git stats only those.
	var hunksU, hunksB map[string][][2]int
	if len(uncommittedFiles) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hunksU = hunks(info.Root, head, uncommittedFiles)
		}()
	}
	if s.BranchBase != "" && len(branchFiles) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hunksB = hunks(info.Root, s.BranchBase, branchFiles)
		}()
	}
	wg.Wait()
	if hunksU == nil {
		hunksU = map[string][][2]int{}
	}
	for _, f := range untrackedFiles {
		hunksU[f] = [][2]int{{1, 1 << 30}}
	}

	for _, f := range s.Files {
		if !outline.Supported(f) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(info.Root, filepath.FromSlash(f)))
		if err != nil {
			continue // deleted in the working tree
		}
		ts := ref.Targets(f, outline.Parse(f, data, outline.MaxSize))
		refs := ref.Refs(ts)
		mark := map[int]string{}
		for _, h := range hunksB[f] {
			for _, i := range touched(ts, f, h) {
				mark[i] = Branch
			}
		}
		for _, h := range hunksU[f] {
			for _, i := range touched(ts, f, h) {
				mark[i] = Uncommitted
			}
		}
		for i, t := range ts {
			if m := mark[i]; m != "" {
				r := refs[i].String()
				s.Symbols = append(s.Symbols, Symbol{Ref: r, Path: f, Name: t.Name, Kind: t.Kind, Line: t.Line, End: t.End, Mark: m})
				s.marks[r] = m
			}
		}
	}
	return s, nil
}

// touched returns the innermost symbols containing the lines of a hunk.
// An untracked file's hunk covers everything, so every symbol counts.
func touched(ts []ref.Target, path string, h [2]int) []int {
	if h[1] >= 1<<30 {
		out := make([]int, len(ts))
		for i := range ts {
			out[i] = i
		}
		return out
	}
	seen := map[int]bool{}
	var out []int
	for line := h[0]; line <= h[1]; line++ {
		if i := ref.Innermost(ts, path, line); i >= 0 && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	return out
}

// hunks returns, per file, the working-tree line ranges of files that
// differ from base. A pure deletion counts as the line it happened at.
func hunks(root, base string, files []string) map[string][][2]int {
	out := map[string][][2]int{}
	args := append([]string{"diff", "-U0", "--no-color", "--no-ext-diff", "--no-renames", base, "--"}, files...)
	data, err := gitRun(root, false, args...)
	if err != nil {
		return out
	}
	file := ""
	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		l := sc.Text()
		switch {
		case strings.HasPrefix(l, "+++ "):
			file = ""
			if p, ok := strings.CutPrefix(unquote(l[4:]), "b/"); ok {
				file = p
			}
		case strings.HasPrefix(l, "@@ ") && file != "":
			if start, n, ok := newSide(l); ok {
				end := start + n - 1
				if n == 0 {
					start, end = max(start, 1), max(start, 1)
				}
				out[file] = append(out[file], [2]int{start, end})
			}
		}
	}
	return out
}

// newSide reads "+c,d" from a hunk header "@@ -a,b +c,d @@".
func newSide(header string) (start, n int, ok bool) {
	f := strings.Fields(header)
	if len(f) < 3 || !strings.HasPrefix(f[2], "+") {
		return 0, 0, false
	}
	a, b, has := strings.Cut(f[2][1:], ",")
	start, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	n = 1
	if has {
		if n, err = strconv.Atoi(b); err != nil {
			return 0, 0, false
		}
	}
	return start, n, true
}

// unquote undoes git's quoting of unusual paths ("\"b/tab\\there.go\"").
func unquote(p string) string {
	if !strings.HasPrefix(p, `"`) {
		return p
	}
	if s, err := strconv.Unquote(p); err == nil {
		return s
	}
	return p
}

func split0(s string) []string {
	var out []string
	for _, f := range strings.Split(s, "\x00") {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// gitRun runs git in dir. Paths are literal, never globs. Only a command that
// may refresh the index (status) gets optional locks.
func gitRun(dir string, refresh bool, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_LITERAL_PATHSPECS=1")
	if !refresh {
		cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0")
	}
	out, err := cmd.Output()
	return string(out), err
}

func git(dir string, args ...string) (string, error) {
	out, err := gitRun(dir, false, args...)
	return strings.TrimRight(out, "\n"), err
}
