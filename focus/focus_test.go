package focus

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Vaspyyy/prism/repo"
)

func store(t *testing.T, branch string) (*Store, *repo.Info) {
	t.Helper()
	for _, v := range []string{"PRISM_SESSION", "CLAUDE_CODE_SESSION_ID", "T3CODE_THREAD_ID"} {
		t.Setenv(v, "")
	}
	dir := t.TempDir()
	info := &repo.Info{Root: dir, Git: true, GitDir: filepath.Join(dir, ".git"), Head: "abc", Branch: branch}
	return Open(info), info
}

func TestRoundTrip(t *testing.T) {
	s, _ := store(t, "main")
	if s.Focus() != nil || s.LastRun() != nil {
		t.Fatal("empty store returned records")
	}
	syms := make([]string, MaxSymbols+5)
	for i := range syms {
		syms[i] = fmt.Sprintf("a.go:F%d", i)
	}
	if err := s.SetFocus("recap", Focus{Range: "b681ef1", Old: "o", New: "n", Symbols: syms}); err != nil {
		t.Fatal(err)
	}
	f := s.Focus()
	if f == nil || f.Range != "b681ef1" || len(f.Symbols) != MaxSymbols || f.Total != MaxSymbols+5 || f.By != "recap" || f.Head != "abc" || f.Branch != "main" {
		t.Fatalf("focus = %+v", f)
	}
	if !f.Has("a.go:F0") || f.Has("a.go:F504") {
		t.Error("Has")
	}
	if err := s.SetLastRun("crux", LastRun{Cmd: "go test ./...", Exit: 1, Tests: []string{"a_test.go:TestF", "a_test.go:TestF"}, In: []string{"a_test.go:TestF", "a.go:G"}}); err != nil {
		t.Fatal(err)
	}
	r := s.LastRun()
	if r == nil || !r.Failing("a_test.go:TestF") || r.Failing("a.go:F0") || len(r.Tests) != 1 || len(r.In) != 1 || r.In[0] != "a.go:G" {
		t.Fatalf("last run = %+v", r)
	}
	if f.Name() != "commit n" || (&Focus{Range: "main...feature"}).Name() != "main...feature" || (&Focus{New: "b681ef1cccc"}).Name() != "commit b681ef1" {
		t.Errorf("Name = %q", f.Name())
	}
	if _, err := os.Stat(filepath.Join(s.dir, "focus.json")); err != nil {
		t.Error("no session: focus.json expected")
	}
	var none *Store
	if none.Focus() != nil || none.SetFocus("recap", Focus{}) != nil {
		t.Error("nil store")
	}
	if Open(&repo.Info{Root: "/x"}) != nil {
		t.Error("store outside git")
	}
}

func TestExpiry(t *testing.T) {
	s, info := store(t, "main")
	now := time.Now()
	s.now = func() time.Time { return now }
	s.SetFocus("recap", Focus{Range: "x"})
	// a read within the TTL pushes it back
	s.now = func() time.Time { return now.Add(50 * time.Minute) }
	if s.Focus() == nil {
		t.Fatal("expired too early")
	}
	s.now = func() time.Time { return now.Add(100 * time.Minute) }
	if s.Focus() == nil {
		t.Fatal("a read should have renewed it")
	}
	s.now = func() time.Time { return now.Add(161 * time.Minute) }
	if s.Focus() != nil {
		t.Fatal("not expired after TTL")
	}
	if _, err := os.Stat(s.file("focus")); !os.IsNotExist(err) {
		t.Error("expired record not deleted")
	}
	// a branch change ends it
	s.now = time.Now
	s.SetFocus("recap", Focus{Range: "y"})
	info.Branch = "feature"
	if Open(info).Focus() != nil {
		t.Error("focus survived a branch change")
	}
	// so does a broken file
	info.Branch = "main"
	s2 := Open(info)
	os.WriteFile(s2.file("focus"), []byte("{"), 0o644)
	if s2.Focus() != nil {
		t.Error("broken record read")
	}
}

func TestSessions(t *testing.T) {
	_, info := store(t, "main")
	t.Setenv("T3CODE_THREAD_ID", "thread-1")
	a := Open(info)
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude/1 with spaces")
	b := Open(info)
	if Session() != "claude/1 with spaces" {
		t.Errorf("Session() = %q: CLAUDE_CODE_SESSION_ID should win over T3CODE_THREAD_ID", Session())
	}
	t.Setenv("PRISM_SESSION", "override")
	if Session() != "override" {
		t.Error("PRISM_SESSION should win")
	}
	a.SetFocus("recap", Focus{Range: "a"})
	b.SetFocus("recap", Focus{Range: "b"})
	if a.Focus().Range != "a" || b.Focus().Range != "b" {
		t.Error("sessions see each other's focus")
	}
	if filepath.Dir(b.file("focus")) != a.dir || filepath.Base(b.file("focus")) == "focus.json" {
		t.Errorf("session file %q", b.file("focus"))
	}
	t.Setenv("PRISM_SESSION", "")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "")
	t.Setenv("T3CODE_THREAD_ID", "")
	if Open(info).Focus() != nil {
		t.Error("the shared record shouldn't see a session's focus")
	}
}

func TestConcurrentWriters(t *testing.T) {
	s, _ := store(t, "main")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.SetFocus("recap", Focus{Range: fmt.Sprint(i), Symbols: []string{"a.go:A"}})
		}()
		go func() {
			defer wg.Done()
			if f := s.Focus(); f != nil && len(f.Symbols) != 1 {
				t.Errorf("torn read: %+v", f)
			}
		}()
	}
	wg.Wait()
	if s.Focus() == nil {
		t.Error("no focus after concurrent writes")
	}
	left, _ := filepath.Glob(filepath.Join(s.dir, ".*.tmp"))
	if len(left) > 0 {
		t.Errorf("temp files left: %q", left)
	}
}
