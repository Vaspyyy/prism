// Package report is the JSON every tool prints with --json: one envelope
// per run, the shared objects in it (Symbol, Location, Link, Next, Error),
// and each tool's result (Seek, Recap, Crux, Glint). The shape is defined
// here once, so the tools can't drift apart. The format is specified in
// docs/output.md.
//
// A tool builds an envelope with New, sets Result (or calls Fail), and
// writes it with Write. Programs reading the output use Check, which also
// rejects a schema version they don't know.
package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/repo"
)

// Schema is the format's name and major version. Adding fields keeps it;
// removing or renaming one, changing a field's meaning, or changing how refs
// are formed makes it prism/2.
const Schema = "prism/1"

// Envelope is the one JSON document a tool prints per run.
type Envelope struct {
	Schema    string     `json:"schema"`
	Tool      string     `json:"tool"`
	Version   string     `json:"version"`
	Args      []string   `json:"args"`    // the arguments after the tool's name, as given
	Repo      Repo       `json:"repo"`    // where paths are relative to
	Context   *Context   `json:"context"` // what context the tool used; null until tools use context
	Result    any        `json:"result,omitempty"`
	Error     *Error     `json:"error,omitempty"` // instead of result, when the run failed
	Links     []Link     `json:"links"`
	Next      []Next     `json:"next"`
	Truncated *Truncated `json:"truncated"` // null unless a limit cut the result
}

// Repo is the repository (or, outside git, the directory) paths are
// relative to.
type Repo struct {
	Root          string `json:"root"` // absolute
	Git           bool   `json:"git"`
	GitDir        string `json:"git_dir,omitempty"`
	Head          string `json:"head,omitempty"`
	Branch        string `json:"branch,omitempty"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Dirty         bool   `json:"dirty"`
}

// Context is what context a run used (docs/context.md). Tools leave it nil
// until they use context; the fields grow with that work.
type Context struct {
	Applied []string `json:"applied"` // any of "rank", "mark", "fill"
}

// Symbol is a definition.
type Symbol struct {
	Ref   string   `json:"ref"`
	Path  string   `json:"path"`
	Name  string   `json:"name"` // qualified, without "#N"
	Kind  string   `json:"kind"`
	Line  int      `json:"line"` // the defining line
	End   int      `json:"end"`
	Doc   []int    `json:"doc,omitempty"` // first and last line of its doc comment and decorators
	Sig   string   `json:"sig,omitempty"`
	Rev   string   `json:"rev,omitempty"`   // set when it's the symbol as of a commit
	Marks []string `json:"marks,omitempty"` // context marks: "changed", "branch", "focus", "failing"
}

// Location is a place in a file, and the symbol containing it.
type Location struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Col  int    `json:"col,omitempty"`
	In   string `json:"in,omitempty"` // ref of the innermost symbol containing the line
}

// Link is a relation between two results and how it's known
// (docs/evidence.md).
type Link struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Rel   string `json:"rel"`   // "mentions", "contains", "ran", "covers", "named-like"
	Basis string `json:"basis"` // "name", "reference", "location", "execution", "coverage"
	At    string `json:"at,omitempty"`
}

// Next is a suggested command, runnable as is.
type Next struct {
	Cmd string `json:"cmd"`
	Why string `json:"why"`
}

// Error is why a run failed. Candidates are refs a program can pick from
// (for a ref that matched nothing, or too many).
type Error struct {
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Candidates []string `json:"candidates,omitempty"`
}

// Error codes.
const (
	CodeNoMatch   = "no_match"  // a ref or name named nothing
	CodeAmbiguous = "ambiguous" // a ref named more than the command can take
	CodeUsage     = "usage"     // bad arguments
	CodeFailed    = "failed"    // anything else
)

// Truncated says a limit cut a list.
type Truncated struct {
	Shown int `json:"shown"`
	Total int `json:"total"`
}

// MaxItems is the safety cap on a list in JSON output; the text budget
// doesn't apply to --json unless -b is given.
const MaxItems = 10000

// New starts an envelope. info may be nil (the repository couldn't be
// read); Dirty is computed here, since JSON is for programs and they get
// the full picture.
func New(tool, version string, args []string, info *repo.Info) *Envelope {
	e := &Envelope{Schema: Schema, Tool: tool, Version: version, Args: args, Links: []Link{}, Next: []Next{}}
	if e.Args == nil {
		e.Args = []string{}
	}
	if info != nil {
		e.Repo = Repo{Root: info.Root, Git: info.Git, GitDir: info.GitDir, Head: info.Head, Branch: info.Branch, DefaultBranch: info.DefaultBranch}
		e.Repo.Dirty, _ = info.Dirty()
	}
	return e
}

// Fail turns the envelope into a failure: no result, an error.
func (e *Envelope) Fail(code, message string, candidates ...string) {
	e.Result, e.Error = nil, &Error{Code: code, Message: message, Candidates: candidates}
}

// Write prints the envelope, indented, without HTML escaping (signatures
// contain <, > and &).
func Write(w io.Writer, e *Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}

// Decoded is an envelope read back, with the result left as raw JSON for
// the caller to decode into the tool's result type.
type Decoded struct {
	Envelope
	Result json.RawMessage `json:"result,omitempty"`
}

// Check reads one envelope and checks its shape: a prism/1 schema, the
// required fields, and exactly one of result and error.
func Check(data []byte) (*Decoded, error) {
	var d Decoded
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("not a JSON envelope: %w", err)
	}
	if dec.More() {
		return nil, errors.New("more than one JSON document")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var problems []string
	major, _ := strings.CutPrefix(d.Schema, "prism/")
	switch {
	case d.Schema == "":
		problems = append(problems, "no schema")
	case major != strings.TrimPrefix(Schema, "prism/"):
		problems = append(problems, fmt.Sprintf("schema %q, this reader knows %q", d.Schema, Schema))
	}
	for _, f := range []string{"tool", "version", "args", "repo", "context", "links", "next", "truncated"} {
		if _, ok := raw[f]; !ok {
			problems = append(problems, "no "+f)
		}
	}
	if d.Repo.Root == "" {
		problems = append(problems, "no repo.root")
	}
	hasResult := len(d.Result) > 0 && string(d.Result) != "null"
	switch {
	case hasResult && d.Error != nil:
		problems = append(problems, "both result and error")
	case !hasResult && d.Error == nil:
		problems = append(problems, "neither result nor error")
	case d.Error != nil && (d.Error.Code == "" || d.Error.Message == ""):
		problems = append(problems, "error without code or message")
	}
	if len(problems) > 0 {
		return nil, errors.New(strings.Join(problems, "; "))
	}
	return &d, nil
}

// refKeys are the fields whose string values are refs.
var refKeys = map[string]bool{"ref": true, "in": true, "test": true, "target": true, "from": true, "to": true, "old_ref": true}

// Refs returns every ref in a JSON document, in order: the values of the
// fields that hold refs (ref, in, test, target, from, to, old_ref),
// wherever they are.
func Refs(data []byte) ([]string, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, k := range slices.Sorted(maps.Keys(x)) {
				if s, ok := x[k].(string); ok && refKeys[k] && s != "" {
					out = append(out, s)
				} else {
					walk(x[k])
				}
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(v)
	return out, nil
}

// Symbols turns a file's outline into Symbols with refs (ordinals where a
// name is duplicated). path is repo-relative.
func Symbols(path string, syms []outline.Symbol) []Symbol {
	ts := ref.Targets(path, syms)
	refs := ref.Refs(ts)
	out := make([]Symbol, len(syms))
	for i, s := range syms {
		out[i] = Symbol{Ref: refs[i].String(), Path: path, Name: ts[i].Name, Kind: s.Kind, Line: s.Line, End: s.End, Sig: s.Sig}
	}
	return out
}

// SymbolOf is the Symbol of one target, with the ordinal its ref needs
// (ref.Ordinals over the whole file) and its signature.
func SymbolOf(t ref.Target, ordinal int, sig string) Symbol {
	return Symbol{Ref: ref.Symbol(t.Path, t.Name, ordinal).String(), Path: t.Path, Name: t.Name, Kind: t.Kind, Line: t.Line, End: t.End, Sig: sig}
}
