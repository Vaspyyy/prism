package report

import (
	"strings"
	"unicode/utf8"
)

// Context is what context a run had and what it did with it
// (docs/context.md). It's nil when context is off (--no-context or
// PRISM_NO_CONTEXT=1). prism/work builds it; tools don't fill it by hand.
type Context struct {
	// Applied is what context changed in this run: any of "rank", "mark",
	// "fill", "suggest" (a next: line only context gave a reason for).
	// Empty when context was available but changed nothing.
	Applied []string        `json:"applied"`
	Changed *ContextChanged `json:"changed,omitempty"`  // derived: what changed in this worktree
	Focus   *ContextFocus   `json:"focus,omitempty"`    // remembered: what recap last showed
	LastRun *ContextLastRun `json:"last_run,omitempty"` // remembered: crux's last run
	// Notes say what the run did with context beyond marking, in the same
	// words as the text report's context: line ("nothing matches in the
	// working tree; showing focus commit b681ef1").
	Notes []string `json:"notes,omitempty"`
}

// ContextChanged is the derived context: uncommitted changes and, on a
// branch other than the default one, the branch's own commits.
type ContextChanged struct {
	Base        string `json:"base"`                  // what uncommitted changes are against: "HEAD"
	BranchBase  string `json:"branch_base,omitempty"` // the merge base with the default branch
	Files       int    `json:"files"`                 // changed files
	Uncommitted int    `json:"uncommitted"`           // changed symbols marked "changed"
	Branch      int    `json:"branch"`                // changed symbols marked "branch"
	Skipped     string `json:"skipped,omitempty"`     // why symbol detail was skipped ("812 changed files")
}

// ContextFocus is the remembered focus: the commit or range recap last
// showed explicitly.
type ContextFocus struct {
	Range   string `json:"range"`   // as recap was given it: "b681ef1", "main...feature"
	Old     string `json:"old"`     // full hash of the old side
	New     string `json:"new"`     // full hash of the new side; symbols are as of this commit
	Symbols int    `json:"symbols"` // changed symbols in it
	By      string `json:"by"`      // the tool that wrote it
	At      string `json:"at"`      // RFC 3339
}

// ContextLastRun is crux's last run in this worktree.
type ContextLastRun struct {
	Cmd     string `json:"cmd"`
	Exit    int    `json:"exit"`
	Failing int    `json:"failing"` // failing tests plus symbols containing an error
	By      string `json:"by"`
	At      string `json:"at"` // RFC 3339
}

// Quote returns a shell word for s that POSIX shells, bash and zsh read
// back as s: s itself when it's made of plain characters, else s in single
// quotes. fish reads it the same unless s holds a backslash.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	plain := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("_-./:=@%+,", r):
		default:
			plain = false
		}
	}
	if plain {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Cmd joins a command's words, quoting each as needed, so it can be run as
// printed: Cmd("go", "test", "-run", "^TestList$", "./internal/files/")
// is "go test -run '^TestList$' ./internal/files/".
func Cmd(args ...string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = Quote(a)
	}
	return strings.Join(q, " ")
}

// MaxNext is how many next: lines a report shows (docs/output.md).
const MaxNext = 3

// NextLines formats suggestions as the text report's next: lines, at most
// MaxNext, with the reasons lined up:
//
//	next: seek internal/files/files.go:List                   definition and callers
//	next: crux go test -run '^TestList$' ./internal/files/   runs the 1 test that mentions List
func NextLines(next []Next) []string {
	next = next[:min(len(next), MaxNext)]
	width := 0
	for _, n := range next {
		if w := utf8.RuneCountInString(n.Cmd); w <= 60 {
			width = max(width, w)
		}
	}
	out := make([]string, len(next))
	for i, n := range next {
		pad := max(width-utf8.RuneCountInString(n.Cmd), 0) + 3
		out[i] = "next: " + n.Cmd
		if n.Why != "" {
			out[i] += strings.Repeat(" ", pad) + n.Why
		}
	}
	return out
}
