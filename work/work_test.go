package work

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Vaspyyy/prism/changes"
	"github.com/Vaspyyy/prism/focus"
	"github.com/Vaspyyy/prism/repo"
)

func newRepo(t *testing.T) (string, *repo.Info) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CEILING_DIRECTORIES": base,
		"GIT_AUTHOR_NAME": "T", "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "T", "GIT_COMMITTER_EMAIL": "t@example.com",
		"PRISM_SESSION": "", "CLAUDE_CODE_SESSION_ID": "", "T3CODE_THREAD_ID": "", "PRISM_NO_CONTEXT": "",
	} {
		t.Setenv(k, v)
	}
	dir := filepath.Join(base, "r")
	os.MkdirAll(dir, 0o755)
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() {}\n\nfunc B() {}\n"), 0o644)
	git("add", ".")
	git("commit", "-q", "-m", "first")
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() { _ = 1 }\n\nfunc B() {}\n"), 0o644)
	info, err := repo.Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, info
}

func TestDisabled(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "1": true, "yes": true} {
		t.Setenv("PRISM_NO_CONTEXT", v)
		if Disabled(false) != want {
			t.Errorf("PRISM_NO_CONTEXT=%q: %v", v, !want)
		}
	}
	t.Setenv("PRISM_NO_CONTEXT", "")
	if !Disabled(true) {
		t.Error("--no-context ignored")
	}
}

func TestOff(t *testing.T) {
	_, info := newRepo(t)
	s := Load(info, Changes|Focus|LastRun, true)
	s.Use(Fill)
	s.Note("filled")
	if s.Marks("a.go:A") != nil || s.FileMarks("a.go") != nil || s.Line() != "" || s.Context() != nil || s.Store != nil || len(s.Applied()) != 0 {
		t.Errorf("context off still did something: %+v", s)
	}
	var none *State
	if none.Marks("a.go:A") != nil || none.Line() != "" || none.Context() != nil {
		t.Error("nil state")
	}
	if s := Load(&repo.Info{Root: t.TempDir()}, Changes, false); s.Line() != "" || s.Store != nil {
		t.Error("outside git")
	}
}

func TestChanges(t *testing.T) {
	_, info := newRepo(t)
	s := Load(info, Changes, false)
	if s.Line() != "" || !reflect.DeepEqual(s.Applied(), []string{}) {
		t.Errorf("nothing marked yet, but line %q, applied %q", s.Line(), s.Applied())
	}
	if c := s.Context(); c == nil || c.Changed == nil || c.Changed.Uncommitted != 1 || c.Changed.Files != 1 || c.Focus != nil {
		t.Errorf("context before marks: %+v", c)
	}
	if got := s.Marks("a.go:A"); !reflect.DeepEqual(got, []string{MarkChanged}) || s.Marks("a.go:B") != nil {
		t.Errorf("marks = %q", got)
	}
	if got := s.FileMarks("a.go"); !reflect.DeepEqual(got, []string{MarkChanged}) {
		t.Errorf("file marks = %q", got)
	}
	if s.Line() != "context: 1 changed symbol vs HEAD" {
		t.Errorf("line = %q", s.Line())
	}
	s.Use(Rank)
	if got := s.Context().Applied; !reflect.DeepEqual(got, []string{Rank, Mark}) {
		t.Errorf("applied = %q", got)
	}
	if Suffix(s.Marks("a.go:A")) != " [changed]" || Suffix(nil) != "" {
		t.Error("Suffix")
	}
}

func TestRemembered(t *testing.T) {
	_, info := newRepo(t)
	st := focus.Open(info)
	st.SetFocus("recap", focus.Focus{Range: "HEAD~1", Old: "1111111aaaa", New: "b681ef1cccc", Symbols: []string{"a.go:B"}})
	st.SetLastRun("crux", focus.LastRun{Cmd: "go test ./...", Exit: 1, In: []string{"a.go:A"}})

	// a layer that isn't loaded marks nothing
	if got := Load(info, Changes, false).Marks("a.go:B"); got != nil {
		t.Errorf("focus not loaded, but %q", got)
	}
	s := Load(info, Changes|Focus|LastRun, false)
	if got := s.Marks("a.go:A"); !reflect.DeepEqual(got, []string{MarkChanged, MarkFailing}) {
		t.Errorf("A = %q", got)
	}
	if got := s.Marks("a.go:B"); !reflect.DeepEqual(got, []string{MarkFocus}) {
		t.Errorf("B = %q", got)
	}
	s.Use(Fill)
	s.Note("nothing matches in the working tree; showing focus commit b681ef1")
	want := "context: 1 changed symbol vs HEAD; focus: commit b681ef1 (recap, just now); last run: crux go test ./... failed (just now); nothing matches in the working tree; showing focus commit b681ef1"
	if s.Line() != want {
		t.Errorf("line\n got %q\nwant %q", s.Line(), want)
	}
	c := s.Context()
	if !reflect.DeepEqual(c.Applied, []string{Mark, Fill}) || c.Focus == nil || c.Focus.Symbols != 1 || c.LastRun == nil || c.LastRun.Failing != 1 || len(c.Notes) != 1 {
		t.Errorf("context = %+v", c)
	}
	if _, err := time.Parse(time.RFC3339, c.Focus.At); err != nil {
		t.Errorf("focus at: %v", err)
	}
	s.now = func() time.Time { return time.Now().Add(4 * time.Minute) }
	if !strings.Contains(s.Line(), "(recap, 4 min ago)") {
		t.Errorf("age: %q", s.Line())
	}
	// a range is named as given
	st.SetFocus("recap", focus.Focus{Range: "main...feature", New: "c", Symbols: []string{"a.go:B"}})
	s = Load(info, Focus, false)
	s.Marks("a.go:B")
	if !strings.HasPrefix(s.Line(), "context: focus: main...feature (recap,") { // no old side stored: the range as given
		t.Errorf("range line: %q", s.Line())
	}
}

func TestPeekAndUsed(t *testing.T) {
	_, info := newRepo(t)
	s := Load(info, Changes, false)
	if got := s.Peek("a.go:A"); !reflect.DeepEqual(got, []string{MarkChanged}) || !reflect.DeepEqual(s.PeekFile("a.go"), []string{MarkChanged}) {
		t.Errorf("peek = %q", got)
	}
	if s.Line() != "" || len(s.Applied()) != 0 {
		t.Errorf("peeking counted as shown: %q", s.Line())
	}
	// ranking by a peeked mark: the line names the layer, nothing was marked
	s.Use(Rank)
	s.Used(s.Peek("a.go:A")...)
	if s.Line() != "context: 1 changed symbol vs HEAD" || !reflect.DeepEqual(s.Applied(), []string{Rank}) {
		t.Errorf("after Used: %q %q", s.Line(), s.Applied())
	}
	// a suggestion only context gave a reason for
	s.Use(Suggest)
	if !reflect.DeepEqual(s.Applied(), []string{Rank, Suggest}) {
		t.Errorf("suggest: %q", s.Applied())
	}
	// printing a peeked mark
	s.Show(s.PeekFile("a.go")...)
	if !reflect.DeepEqual(s.Context().Applied, []string{Rank, Mark, Suggest}) {
		t.Errorf("after Show: %q", s.Context().Applied)
	}
}

func TestLoadWith(t *testing.T) {
	dir, info := newRepo(t)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package a\n\nfunc C() {}\n"), 0o644)
	s := LoadWith(info, Options{Layers: Changes, Changes: changes.Options{Paths: []string{"b.go"}}})
	if s.Peek("a.go:A") != nil || s.Peek("b.go:C") == nil || len(s.Changes.Files) != 1 {
		t.Errorf("paths: %+v", s.Changes)
	}
	s = LoadWith(info, Options{Layers: Changes, Changes: changes.Options{FilesOnly: true, NoLocks: true}})
	if s.Peek("a.go:A") != nil || s.FileMarks("a.go") == nil || s.Line() != "context: 2 changed files" {
		t.Errorf("files only: line %q", s.Line())
	}
	if s := LoadWith(info, Options{Layers: Changes, Off: true}); s.Changes != nil || s.Store != nil {
		t.Error("off")
	}
}

func TestLongCommand(t *testing.T) {
	_, info := newRepo(t)
	focus.Open(info).SetLastRun("crux", focus.LastRun{Cmd: "sh -c 'go build ./...\n  && go test ./... -run '^(TestAlpha|TestBeta|TestGamma|TestDelta)$''", Exit: 2, In: []string{"a.go:B"}})
	s := Load(info, LastRun, false)
	s.Marks("a.go:B")
	l := s.Line()
	if strings.Contains(l, "\n") || !strings.Contains(l, "…") || len([]rune(l)) > 120 {
		t.Errorf("line = %q", l)
	}
	if oneLine("go  test\n./...", 60) != "go test ./..." {
		t.Error("oneLine")
	}
}
