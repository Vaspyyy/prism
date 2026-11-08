// Package ref is the identifier every tool prints and accepts for a file, a
// symbol in a file, or a location in a file:
//
//	internal/files/files.go               a file
//	internal/files/files.go:List          a symbol (its qualified name)
//	internal/files/files.go:73            a line (73:5 with a column, 73-107 a range)
//	main.go:run@b681ef1                   a symbol as it was in a commit
//	lib.cpp:parse#2                       the second definition named parse
//
// Parse reads a ref without touching the file system. The tool resolves its
// path with prism/repo and matches its name against the symbols it has with
// Match, so each tool keeps its own scope (recap a diff, seek a repository).
// The format is specified in docs/refs.md.
package ref

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Vaspyyy/prism/outline"
)

// Ref is a parsed ref.
type Ref struct {
	Path    string // the part before the first ':'; the whole text when there's no ':'
	Name    string // qualified symbol name without "#N"; "" for a file or a location
	Ordinal int    // N from a trailing "#N" on the name; 0 if none
	Line    int    // a location's line; 0 when the ref isn't a location
	Col     int    // a location's column; 0 if none
	End     int    // a location's last line ("73-107"); 0 if none
	Rev     string // revision from a trailing "@rev"; "" if none

	// HasTarget is true when a ':' followed the path, so the ref names a
	// symbol or location rather than (possibly) a file or a bare name.
	HasTarget bool

	body string // the text without "@rev", for AsName
}

var (
	locationRe = regexp.MustCompile(`^([0-9]+)(?:-([0-9]+)|:([0-9]+))?$`)
	ordinalRe  = regexp.MustCompile(`^(.+)#([1-9][0-9]*)$`)
)

// Parse reads a ref. isRev tells whether the text after the last '@' is a
// revision (repo.Info.IsRev); when it isn't, or isRev is nil, the '@' is
// part of the name or path ("README.md:Contact @me").
func Parse(s string, isRev func(string) bool) Ref {
	s = strings.TrimSpace(s)
	var r Ref
	if i := strings.LastIndexByte(s, '@'); i > 0 && isRev != nil {
		if rev := s[i+1:]; rev != "" && !strings.ContainsAny(rev, " \t") && isRev(rev) {
			r.Rev, s = rev, s[:i]
		}
	}
	r.body = s
	path, target, ok := strings.Cut(s, ":")
	r.Path, r.HasTarget = path, ok
	if !ok || target == "" {
		return r
	}
	if m := locationRe.FindStringSubmatch(target); m != nil {
		r.Line = atoi(m[1])
		r.End, r.Col = atoi(m[2]), atoi(m[3])
		if r.End > 0 && r.End < r.Line {
			r.Line, r.End = r.End, r.Line
		}
		if r.Line > 0 {
			return r
		}
		r.Line, r.End, r.Col = 0, 0, 0 // line 0: a name made of digits
	}
	r.Name, r.Ordinal = splitOrdinal(target)
	return r
}

func splitOrdinal(name string) (string, int) {
	if m := ordinalRe.FindStringSubmatch(name); m != nil {
		return m[1], atoi(m[2])
	}
	return name, 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// AsName reads the whole ref (before any "@rev") as a symbol name with no
// path. Tools use it for text that may be a file or a name ("List", tried
// as a file first) and for names that contain ':' themselves (C++
// "Foo::bar", Lua "obj:method") when the part before the first ':' isn't a
// file.
func (r Ref) AsName() Ref {
	n := Ref{Rev: r.Rev, body: r.body, HasTarget: true}
	n.Name, n.Ordinal = splitOrdinal(r.body)
	return n
}

// IsLocation reports whether the ref names a line or range.
func (r Ref) IsLocation() bool { return r.Line > 0 }

// String formats the ref so that Parse reads it back.
func (r Ref) String() string {
	var b strings.Builder
	b.WriteString(r.Path)
	switch {
	case r.Line > 0:
		b.WriteString(":" + strconv.Itoa(r.Line))
		if r.End > 0 {
			b.WriteString("-" + strconv.Itoa(r.End))
		} else if r.Col > 0 {
			b.WriteString(":" + strconv.Itoa(r.Col))
		}
	case r.Name != "":
		if r.Path != "" {
			b.WriteByte(':')
		}
		b.WriteString(r.Name)
		if r.Ordinal > 0 {
			b.WriteString("#" + strconv.Itoa(r.Ordinal))
		}
	}
	if r.Rev != "" {
		b.WriteString("@" + r.Rev)
	}
	return b.String()
}

// File is the ref of a file.
func File(path string) Ref { return Ref{Path: path} }

// Symbol is the ref of a symbol. ordinal is 0 unless the name is duplicated
// in its file (see Ordinals).
func Symbol(path, name string, ordinal int) Ref {
	return Ref{Path: path, Name: name, Ordinal: ordinal, HasTarget: true}
}

// Location is the ref of a line, with a column if col > 0.
func Location(path string, line, col int) Ref {
	return Ref{Path: path, Line: line, Col: col, HasTarget: true}
}

// Target is a symbol a ref can name.
type Target struct {
	Path string // repo-relative, forward slashes
	Name string // qualified name, as outline.Qualified returns it
	Kind string // outline kind: "func", "type", "impl", "h2", …
	Line int    // first line (the defining line)
	End  int    // last line
}

// Targets turns a file's outline symbols into targets, in the same order.
func Targets(path string, syms []outline.Symbol) []Target {
	ts := make([]Target, len(syms))
	for i, s := range syms {
		ts[i] = Target{Path: path, Name: outline.Qualified(syms, i), Kind: s.Kind, Line: s.Line, End: s.End}
	}
	return ts
}

// Ordinals returns the ordinal each target's ref needs: 0 when its name is
// unique in its file, else its 1-based position among the targets in that
// file with the same name. A Rust type and its impls share a name: the type
// keeps the plain name and the impls get ordinals. Pass all of a file's
// symbols, not just the ones being printed, so ordinals don't depend on
// what's shown.
func Ordinals(ts []Target) []int {
	out := make([]int, len(ts))
	for _, g := range groups(ts, func(int) bool { return true }) {
		if len(g) < 2 {
			continue
		}
		winner := typeWinner(ts, g)
		for pos, i := range g {
			if i != winner {
				out[i] = pos + 1
			}
		}
	}
	return out
}

// Refs returns the ref of every target, with ordinals where needed.
func Refs(ts []Target) []Ref {
	ords := Ordinals(ts)
	out := make([]Ref, len(ts))
	for i, t := range ts {
		out[i] = Symbol(t.Path, t.Name, ords[i])
	}
	return out
}

// Match returns the indexes of the targets r names, in their order in ts.
// files limits the search to those paths (the tool resolves r.Path with
// prism/repo); nil means any file, for a bare name.
//
// A location matches the innermost target containing its line (one per
// file). A name matches targets with exactly that qualified name, else
// targets whose name ends in it at a '.' boundary ("match" finds
// "globSet.match"). Among targets in one file with the same name, an
// ordinal picks one, and without an ordinal a type beats its impls. More
// than one result means the ref is ambiguous: the tool shows them all or
// lists their refs, never just fails.
func Match(r Ref, ts []Target, files []string) []int {
	in := func(i int) bool { return true }
	if files != nil {
		set := make(map[string]bool, len(files))
		for _, f := range files {
			set[f] = true
		}
		in = func(i int) bool { return set[ts[i].Path] }
	}
	if r.Line > 0 {
		var out []int
		seen := map[string]bool{}
		for i := range ts {
			if in(i) && !seen[ts[i].Path] {
				seen[ts[i].Path] = true
				if j := Innermost(ts, ts[i].Path, r.Line); j >= 0 {
					out = append(out, j)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	if r.Name == "" {
		return nil
	}
	try := func(keep func(t Target) bool, ordinal int) []int {
		var out []int
		for _, g := range groups(ts, func(i int) bool { return in(i) && keep(ts[i]) }) {
			switch {
			case ordinal > 0:
				if ordinal <= len(g) {
					out = append(out, g[ordinal-1])
				}
			case len(g) == 1:
				out = append(out, g[0])
			default:
				if w := typeWinner(ts, g); w >= 0 {
					out = append(out, w)
				} else {
					out = append(out, g...)
				}
			}
		}
		slices.Sort(out)
		return out
	}
	exact := func(name string) func(Target) bool { return func(t Target) bool { return t.Name == name } }
	if m := try(exact(r.Name), r.Ordinal); len(m) > 0 {
		return m
	}
	if r.Ordinal > 0 { // "#N" that is part of the name: "Issue #12"
		if m := try(exact(r.Name+"#"+strconv.Itoa(r.Ordinal)), 0); len(m) > 0 {
			return m
		}
	}
	return try(func(t Target) bool { return strings.HasSuffix(t.Name, "."+r.Name) }, r.Ordinal)
}

// Innermost returns the index of the innermost target in path whose lines
// contain line, or -1.
func Innermost(ts []Target, path string, line int) int {
	best := -1
	for i, t := range ts {
		if t.Path != path || line < t.Line || line > max(t.End, t.Line) {
			continue
		}
		if best < 0 || max(t.End, t.Line)-t.Line < max(ts[best].End, ts[best].Line)-ts[best].Line {
			best = i
		}
	}
	return best
}

// groups returns the indexes of the targets that pass keep, grouped by
// file and name, each group in order.
func groups(ts []Target, keep func(int) bool) [][]int {
	type key struct{ path, name string }
	at := map[key]int{}
	var out [][]int
	for i, t := range ts {
		if !keep(i) {
			continue
		}
		k := key{t.Path, t.Name}
		g, ok := at[k]
		if !ok {
			g = len(out)
			at[k] = g
			out = append(out, nil)
		}
		out[g] = append(out[g], i)
	}
	return out
}

// typeWinner is the one target in a group of same-named targets that isn't
// an impl (a Rust type next to its impl blocks), or -1.
func typeWinner(ts []Target, g []int) int {
	w := -1
	for _, i := range g {
		if ts[i].Kind != "impl" {
			if w >= 0 {
				return -1
			}
			w = i
		}
	}
	if w >= 0 && len(g) > 1 {
		return w
	}
	return -1
}
