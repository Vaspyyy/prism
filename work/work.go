// Package work is the context a tool has in one run: what changed
// (prism/changes), the focus and crux's last run (prism/focus), loaded
// together and turned into marks, the context: line and the JSON context
// (docs/context.md).
//
// A tool loads only the layers its row of the table in docs/context.md
// reads, asks for marks as it prints, records what context did (rank,
// mark, fill) and prints Line() at the top of its report. Context never
// breaks a run: anything that fails to load is just missing.
package work

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Vaspyyy/prism/changes"
	"github.com/Vaspyyy/prism/focus"
	"github.com/Vaspyyy/prism/repo"
	"github.com/Vaspyyy/prism/report"
)

// Layer is a part of the context a tool reads.
type Layer int

// Layers.
const (
	Changes Layer = 1 << iota // uncommitted and branch changes
	Focus                     // what recap last showed
	LastRun                   // crux's last run
)

// What context did in a run.
const (
	Rank = "rank" // reordered results
	Mark = "mark" // marked results
	Fill = "fill" // filled a result that would have been empty
)

// Marks, as printed in brackets after a name. Changed and Branch come from
// prism/changes.
const (
	MarkChanged = changes.Uncommitted // "changed"
	MarkBranch  = changes.Branch      // "branch"
	MarkFocus   = "focus"
	MarkFailing = "failing"
)

// Disabled reports whether context is off: the tool's --no-context flag,
// or PRISM_NO_CONTEXT set to anything but "" or "0".
func Disabled(flag bool) bool {
	v := os.Getenv("PRISM_NO_CONTEXT")
	return flag || (v != "" && v != "0")
}

// State is a run's context.
type State struct {
	Off     bool           // context is off: nothing is loaded, marked or written
	Changes *changes.Set   // nil if not loaded, or outside git
	Focus   *focus.Focus   // nil if not loaded, or none
	LastRun *focus.LastRun // nil if not loaded, or none
	Store   *focus.Store   // where a tool writes focus or the last run; nil when off or outside git

	layers  Layer
	applied []string
	used    map[string]bool // marks handed out
	notes   []string
	now     func() time.Time
}

// Options say what to load.
type Options struct {
	Layers Layer
	Off    bool // context is off (see Disabled): load nothing
	// Changes narrows the Changes layer: to some paths, to files only, or
	// without taking git's optional index lock (see changes.Options).
	Changes changes.Options
}

// Load reads the given layers. With off (see Disabled) it loads nothing and
// the state marks nothing, says nothing and has no Store.
func Load(info *repo.Info, layers Layer, off bool) *State {
	return LoadWith(info, Options{Layers: layers, Off: off})
}

// LoadWith is Load with options.
func LoadWith(info *repo.Info, o Options) *State {
	s := &State{Off: o.Off, used: map[string]bool{}, now: time.Now}
	if o.Off || info == nil || !info.Git {
		return s
	}
	s.layers = o.Layers
	s.Store = focus.Open(info)
	if o.Layers&Changes != 0 {
		s.Changes, _ = changes.ComputeWith(info, o.Changes)
	}
	layers := o.Layers
	if layers&Focus != 0 {
		s.Focus = s.Store.Focus()
	}
	if layers&LastRun != 0 {
		s.LastRun = s.Store.LastRun()
	}
	return s
}

// Marks returns a symbol's marks, in a fixed order: changed or branch,
// focus, failing. A mark handed out here counts as shown: the context:
// line names its layer. To look without that (for ranking, or before
// deciding what to print), use Peek, then Used for what was printed.
func (s *State) Marks(ref string) []string {
	out := s.Peek(ref)
	s.Used(out...)
	return out
}

// Peek returns a symbol's marks like Marks, without counting them as shown.
func (s *State) Peek(ref string) []string {
	if s == nil || s.Off {
		return nil
	}
	var out []string
	if m := s.Changes.Mark(ref); m != "" {
		out = append(out, m)
	}
	if s.Focus.Has(ref) {
		out = append(out, MarkFocus)
	}
	if s.LastRun.Failing(ref) {
		out = append(out, MarkFailing)
	}
	return out
}

// FileMarks returns a file's marks (changed or branch) and counts them as
// shown.
func (s *State) FileMarks(path string) []string {
	out := s.PeekFile(path)
	s.Used(out...)
	return out
}

// PeekFile returns a file's marks without counting them as shown.
func (s *State) PeekFile(path string) []string {
	if s == nil || s.Off {
		return nil
	}
	if m := s.Changes.FileMark(path); m != "" {
		return []string{m}
	}
	return nil
}

// Used counts marks as shown, or as the reason results were reordered:
// the context: line then names their layers. Marks and FileMarks call it.
func (s *State) Used(marks ...string) {
	if s == nil || s.Off {
		return
	}
	for _, m := range marks {
		s.used[m] = true
	}
}

// Suffix formats marks as printed after a name: " [changed] [focus]", or
// "" for none.
func Suffix(marks []string) string {
	var b strings.Builder
	for _, m := range marks {
		b.WriteString(" [" + m + "]")
	}
	return b.String()
}

// Use records what context did: Rank, Mark or Fill. Recording Mark isn't
// needed after handing out marks; Marks and FileMarks do that.
func (s *State) Use(what string) {
	if s == nil || s.Off || slices.Contains(s.applied, what) {
		return
	}
	s.applied = append(s.applied, what)
}

// Note adds a sentence to the context: line, such as why a result was
// filled: "nothing matches in the working tree; showing focus commit
// b681ef1".
func (s *State) Note(note string) {
	if s == nil || s.Off || note == "" {
		return
	}
	s.notes = append(s.notes, note)
}

// Applied returns what context did, in a fixed order.
func (s *State) Applied() []string {
	out := []string{}
	if s == nil || s.Off {
		return out
	}
	if len(s.used) > 0 {
		s.Use(Mark)
	}
	for _, a := range []string{Rank, Mark, Fill} {
		if slices.Contains(s.applied, a) {
			out = append(out, a)
		}
	}
	return out
}

// Line is the text report's context: line, or "" when context changed
// nothing. It names the layers whose marks were handed out, the notes,
// and skipped symbol detail.
func (s *State) Line() string {
	if s == nil || s.Off {
		return ""
	}
	var parts []string
	if s.Changes != nil && s.Changes.Skipped != "" {
		parts = append(parts, s.Changes.Skipped+", symbol detail skipped")
	}
	if len(s.Applied()) == 0 && len(parts) == 0 && len(s.notes) == 0 {
		return ""
	}
	if (s.used[MarkChanged] || s.used[MarkBranch]) && s.Changes != nil {
		parts = append(parts, s.changesPart())
	}
	if s.used[MarkFocus] && s.Focus != nil {
		parts = append(parts, "focus: "+s.Focus.Name()+" ("+s.Focus.By+", "+s.ago(s.Focus.At)+")")
	}
	if s.used[MarkFailing] && s.LastRun != nil {
		parts = append(parts, "last run: "+s.LastRun.By+" "+oneLine(s.LastRun.Cmd, 60)+" "+outcome(s.LastRun.Exit)+" ("+s.ago(s.LastRun.At)+")")
	}
	parts = append(parts, s.notes...)
	if len(parts) == 0 {
		return ""
	}
	return "context: " + strings.Join(parts, "; ")
}

func (s *State) changesPart() string {
	u, b := s.Changes.Count()
	var p []string
	if u > 0 {
		p = append(p, plural(u, "changed symbol")+" vs HEAD")
	}
	if b > 0 {
		p = append(p, plural(b, "symbol")+" changed on this branch since "+short(s.Changes.BranchBase))
	}
	if len(p) == 0 {
		return plural(len(s.Changes.Files), "changed file")
	}
	return strings.Join(p, ", ")
}

func (s *State) ago(t time.Time) string {
	d := s.now().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + " min ago"
	}
	return strconv.Itoa(int(d/time.Hour)) + " h ago"
}

// Context is the JSON context: nil when context is off, else what was
// available and what was applied.
func (s *State) Context() *report.Context {
	if s == nil || s.Off {
		return nil
	}
	c := &report.Context{Applied: s.Applied(), Notes: s.notes}
	if ch := s.Changes; ch != nil {
		u, b := ch.Count()
		c.Changed = &report.ContextChanged{Base: ch.Base, BranchBase: ch.BranchBase, Files: len(ch.Files), Uncommitted: u, Branch: b, Skipped: ch.Skipped}
	}
	if f := s.Focus; f != nil {
		c.Focus = &report.ContextFocus{Range: f.Range, Old: f.Old, New: f.New, Symbols: f.Total, By: f.By, At: f.At.Format(time.RFC3339)}
	}
	if r := s.LastRun; r != nil {
		c.LastRun = &report.ContextLastRun{Cmd: r.Cmd, Exit: r.Exit, Failing: len(r.Tests) + len(r.In), By: r.By, At: r.At.Format(time.RFC3339)}
	}
	return c
}

// oneLine is a command on one line: whitespace runs (newlines in an sh -c
// script) become one space, and it's cut to n characters with "…".
func oneLine(cmd string, n int) string {
	cmd = strings.Join(strings.Fields(cmd), " ")
	if r := []rune(cmd); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return cmd
}

func outcome(exit int) string {
	if exit == 0 {
		return "passed"
	}
	return "failed"
}

func short(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

func plural(n int, what string) string {
	s := strconv.Itoa(n) + " " + what
	if n != 1 {
		s += "s"
	}
	return s
}
