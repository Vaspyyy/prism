// Package focus keeps the two pieces of context that can't be derived from
// git: the focus (the commit or range recap last showed explicitly) and
// crux's last run (docs/context.md, "Remembered").
//
// Records live in the worktree's git dir, under prism/, so they're never
// committed and fix work in another worktree has its own. Each agent
// session gets its own files, so parallel agents don't steer each other.
// Writes are atomic; concurrent writers mean the last one wins, which is
// harmless because context only ranks, marks and fills. A record expires
// TTL after it was last written or read, or when the branch changes.
package focus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/repo"
)

// TTL is how long a record lives after it was last written or read.
const TTL = 60 * time.Minute

// MaxSymbols caps the symbol refs a focus record keeps.
const MaxSymbols = 500

// Meta is what every record carries.
type Meta struct {
	By      string    `json:"by"`                // the tool that wrote it
	At      time.Time `json:"at"`                // when
	Head    string    `json:"head"`              // HEAD when it was written
	Branch  string    `json:"branch"`            // the branch when it was written; "" if detached
	Session string    `json:"session,omitempty"` // the agent session that wrote it
}

// Focus is the commit or range recap last showed explicitly.
type Focus struct {
	Meta
	// Range names what was shown: a commit ("b681ef1", a short hash rather
	// than a name like HEAD that moves), or a range as given ("A..B",
	// "main...feature"; two separate revisions are stored as "A..B").
	Range   string   `json:"range"`
	Old     string   `json:"old"`     // full hash of the old side (the empty tree for a root commit)
	New     string   `json:"new"`     // full hash of the new side; Symbols are as of this commit
	Symbols []string `json:"symbols"` // refs of the changed symbols, at most MaxSymbols
	Total   int      `json:"total"`   // changed symbols before the cap
}

// Name is how reports name the focus: "commit b681ef1" for one commit, or
// the range as stored with the commits it resolved to, "HEAD~1..HEAD
// (1f0e2d3..b681ef1)", since names like HEAD move and a report must say
// what to repeat it with.
func (f *Focus) Name() string {
	if f == nil {
		return ""
	}
	if strings.Contains(f.Range, "..") {
		resolved := short(f.Old) + ".." + short(f.New)
		if f.Old == "" || f.New == "" || f.Range == resolved {
			return f.Range
		}
		return f.Range + " (" + resolved + ")"
	}
	h := short(f.New)
	if h == "" {
		h = f.Range
	}
	return "commit " + h
}

func short(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// Has reports whether a symbol ref is in the focus.
func (f *Focus) Has(ref string) bool {
	if f == nil {
		return false
	}
	for _, s := range f.Symbols {
		if s == ref {
			return true
		}
	}
	return false
}

// LastRun is crux's last run in this worktree.
type LastRun struct {
	Meta
	Cmd   string   `json:"cmd"`   // the command crux ran, as a shell line
	Exit  int      `json:"exit"`  // its exit code
	Tests []string `json:"tests"` // refs of the tests that failed
	In    []string `json:"in"`    // refs of the symbols containing an error or failure location
}

// Failing reports whether a symbol failed in the run: a failing test, or a
// symbol containing an error or failure location.
func (r *LastRun) Failing(ref string) bool {
	if r == nil {
		return false
	}
	for _, list := range [][]string{r.Tests, r.In} {
		for _, s := range list {
			if s == ref {
				return true
			}
		}
	}
	return false
}

// Session returns the current agent session, first match wins:
// PRISM_SESSION, CLAUDE_CODE_SESSION_ID, T3CODE_THREAD_ID. "" when none is
// set; records are then shared per worktree.
func Session() string {
	for _, v := range []string{"PRISM_SESSION", "CLAUDE_CODE_SESSION_ID", "T3CODE_THREAD_ID"} {
		if s := os.Getenv(v); s != "" {
			return s
		}
	}
	return ""
}

// Store reads and writes the records of one worktree and session.
type Store struct {
	dir     string
	session string
	branch  string
	head    string
	now     func() time.Time
}

// Open returns the store for a repository and the current session. It's
// nil outside git; a nil store reads nothing and writes nothing.
func Open(info *repo.Info) *Store {
	if info == nil || !info.Git || info.GitDir == "" {
		return nil
	}
	return &Store{dir: filepath.Join(info.GitDir, "prism"), session: Session(), branch: info.Branch, head: info.Head, now: time.Now}
}

// file is a record's path: focus.json, or focus-<hash of session>.json.
// Hashed so any session string makes a safe, short name.
func (s *Store) file(kind string) string {
	if s.session == "" {
		return filepath.Join(s.dir, kind+".json")
	}
	h := sha256.Sum256([]byte(s.session))
	return filepath.Join(s.dir, kind+"-"+hex.EncodeToString(h[:6])+".json")
}

// Focus returns the focus, or nil if there's none or it expired.
func (s *Store) Focus() *Focus {
	var f Focus
	if !s.read("focus", &f, &f.Meta) {
		return nil
	}
	return &f
}

// LastRun returns crux's last run, or nil if there's none or it expired.
func (s *Store) LastRun() *LastRun {
	var r LastRun
	if !s.read("last-run", &r, &r.Meta) {
		return nil
	}
	return &r
}

// SetFocus records a focus. by is the writing tool; the rest of Meta is
// filled in. Symbols beyond MaxSymbols are dropped and Total keeps the
// count.
func (s *Store) SetFocus(by string, f Focus) error {
	if s == nil {
		return nil
	}
	if f.Total < len(f.Symbols) {
		f.Total = len(f.Symbols)
	}
	if len(f.Symbols) > MaxSymbols {
		f.Symbols = f.Symbols[:MaxSymbols]
	}
	if f.Symbols == nil {
		f.Symbols = []string{}
	}
	f.Meta = s.meta(by)
	return s.write("focus", f)
}

// SetLastRun records crux's last run. Refs in In that are also in Tests
// are dropped (a test failing inside itself counts once), and each list
// keeps at most MaxSymbols refs.
func (s *Store) SetLastRun(by string, r LastRun) error {
	if s == nil {
		return nil
	}
	r.Tests = capped(r.Tests, nil)
	r.In = capped(r.In, r.Tests)
	r.Meta = s.meta(by)
	return s.write("last-run", r)
}

// capped returns refs without duplicates and without those in skip, at
// most MaxSymbols of them, never nil.
func capped(refs, skip []string) []string {
	out := []string{}
	for _, r := range refs {
		if len(out) == MaxSymbols {
			break
		}
		if !slices.Contains(out, r) && !slices.Contains(skip, r) {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) meta(by string) Meta {
	return Meta{By: by, At: s.now().Truncate(time.Second), Head: s.head, Branch: s.branch, Session: s.session}
}

// read loads a record into v. An expired, unreadable or foreign record is
// deleted and reported as missing. A good one gets its expiry pushed back.
func (s *Store) read(kind string, v any, m *Meta) bool {
	if s == nil {
		return false
	}
	p := s.file(kind)
	st, err := os.Stat(p)
	if err != nil {
		return false
	}
	now := s.now()
	data, ok := outline.Read(p) // a regular file of sane size, never a planted symlink
	if !ok || json.Unmarshal(data, v) != nil || now.Sub(st.ModTime()) > TTL || m.Branch != s.branch || m.Session != s.session {
		os.Remove(p)
		return false
	}
	os.Chtimes(p, now, now) // read counts as use: the record lives on
	return true
}

// write stores a record atomically: a temp file in the same directory,
// renamed over the old one.
func (s *Store) write(kind string, v any) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "."+kind+"-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.file(kind)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	now := s.now()
	return os.Chtimes(s.file(kind), now, now)
}
