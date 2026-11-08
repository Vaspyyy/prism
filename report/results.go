package report

import "time"

// The tools' results, the envelope's "result". Lists that can be empty
// are always present ([]), so programs can range over them; optional
// values are left out when they don't apply.

// Seek is seek's result.
type Seek struct {
	Pattern     string      `json:"pattern"`
	Target      string      `json:"target,omitempty"` // in ref mode: the ref given as the pattern
	Definitions []Symbol    `json:"definitions"`      // definitions of what was searched for (-d, ref mode)
	Groups      []SeekGroup `json:"groups"`           // hits grouped by the definition containing them
	Files       *[]string   `json:"files,omitempty"`  // -l: the paths with hits (present, maybe empty, with -l only)
	Query       SeekQuery   `json:"query"`
	Skipped     []string    `json:"skipped,omitempty"` // what wasn't searched: binary or too big files, ignored folders, …
	Counts      SeekCounts  `json:"counts"`
}

// SeekQuery is how seek read the pattern.
type SeekQuery struct {
	WholeWord  bool `json:"whole_word"`
	IgnoreCase bool `json:"ignore_case"`
	PlainText  bool `json:"plain_text"` // searched as text: -F, or the pattern wasn't a valid regex
}

// SeekGroup is the hits in one definition, or outside any.
type SeekGroup struct {
	Path     string    `json:"path"`
	FileKind string    `json:"file_kind,omitempty"` // "generated", "vendored", "minified", "lock file"
	Symbol   *Symbol   `json:"symbol"`              // null for lines outside any definition
	Hits     []SeekHit `json:"hits"`
}

// SeekHit is one matching line.
type SeekHit struct {
	Line   int    `json:"line"`
	Col    int    `json:"col,omitempty"` // 1-based, of the first match
	Text   string `json:"text"`
	Label  string `json:"label,omitempty"`  // how seek labels the line: "comment", "string", …
	Writes bool   `json:"writes,omitempty"` // the line sets the name (definition, declaration, assignment)
}

// SeekCounts are totals over the whole search, also when the lists were cut.
type SeekCounts struct {
	Hits        int `json:"hits"`
	Files       int `json:"files"` // files with hits
	Definitions int `json:"definitions"`
	Searched    int `json:"searched"` // files searched
}

// Recap is recap's result: a comparison (recap, recap show) or a log.
type Recap struct {
	Old     string      `json:"old"`               // the old side as named: "b681ef1^", "HEAD", "index"
	New     string      `json:"new"`               // the new side: "b681ef1", "working tree"
	Commit  *Commit     `json:"commit,omitempty"`  // recap show
	Files   []RecapFile `json:"files"`             // empty for recap log
	Commits []RecapLog  `json:"commits,omitempty"` // recap log
	Show    []RecapShow `json:"show,omitempty"`    // --show: the symbols asked for, with their diffs
	Range   string      `json:"range,omitempty"`   // recap log: the revision range
	Notes   []string    `json:"notes,omitempty"`   // "merge in progress", a path that also matched from the working directory, …
}

// Commit is a git commit.
type Commit struct {
	Rev     string    `json:"rev"` // full id
	Author  string    `json:"author"`
	Date    time.Time `json:"date"`
	Subject string    `json:"subject"`
	Body    string    `json:"body,omitempty"`
	Parents []string  `json:"parents,omitempty"`
}

// RecapLog is one commit of recap log, with its files.
type RecapLog struct {
	Commit Commit      `json:"commit"`
	Files  []RecapFile `json:"files"`
}

// RecapFile is one changed file.
type RecapFile struct {
	Path        string        `json:"path"`
	OldPath     string        `json:"old_path,omitempty"` // renamed or moved
	Status      string        `json:"status"`             // "modified", "new", "deleted", "renamed", "untracked", "conflict", …
	Added       int           `json:"added"`
	Removed     int           `json:"removed"`
	Lines       int           `json:"lines,omitempty"`     // lines in the new version (the old one for deleted files)
	Generated   string        `json:"generated,omitempty"` // "lock file", "generated", "minified", "vendored"
	Binary      bool          `json:"binary,omitempty"`
	Similarity  int           `json:"similarity,omitempty"`   // renames and copies: git's similarity in percent
	LineEndings string        `json:"line_endings,omitempty"` // set when only line endings changed: "LF to CRLF"
	ModeChange  string        `json:"mode_change,omitempty"`  // "100644 to 100755"
	Submodule   bool          `json:"submodule,omitempty"`
	Symlink     bool          `json:"symlink,omitempty"`
	Untracked   bool          `json:"untracked,omitempty"` // the new version isn't tracked by git yet
	Changes     []RecapChange `json:"changes"`
	Outside     *RecapOutside `json:"outside,omitempty"` // changed lines outside any definition
	Defs        []RecapDef    `json:"defs,omitempty"`    // new and deleted files: their definitions
}

// RecapOutside is the changed lines outside any definition.
type RecapOutside struct {
	Added   int      `json:"added"`
	Removed int      `json:"removed"`
	Imports bool     `json:"imports,omitempty"` // they're all imports
	Lines   []string `json:"lines,omitempty"`   // the lines, when there are few, starting with '-' or '+'
}

// RecapDef is a definition in a new or deleted file.
type RecapDef struct {
	Symbol  Symbol `json:"symbol"`
	From    string `json:"from,omitempty"`     // new files: ref of the symbol it was moved from
	MovedTo string `json:"moved_to,omitempty"` // deleted files: the file it moved to
	Differs int    `json:"differs,omitempty"`  // with from or moved_to: lines that differ from the other copy
	Public  bool   `json:"public,omitempty"`
}

// RecapChange is one changed symbol.
type RecapChange struct {
	What    string `json:"what"`              // "changed", "added", "removed", "renamed", "moved"
	Symbol  Symbol `json:"symbol"`            // where it is now (where it was, for removed)
	OldRef  string `json:"old_ref,omitempty"` // renamed: the old ref
	OldSig  string `json:"old_sig,omitempty"` // set when the signature changed
	NewSig  string `json:"new_sig,omitempty"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	From    string `json:"from,omitempty"`     // added: ref of the symbol or file its body came from
	MovedTo string `json:"moved_to,omitempty"` // removed: the file it moved to
	Note    string `json:"note,omitempty"`     // "whitespace only", "rewritten", …
	Public  bool   `json:"public,omitempty"`
	OldLine int    `json:"old_line,omitempty"` // where it was before (or where its body came from)
	OldEnd  int    `json:"old_end,omitempty"`
	Differs int    `json:"differs,omitempty"` // with from or moved_to: lines that differ from the other copy
	Members int    `json:"members,omitempty"` // an added or removed class or the like: symbols inside it
	// Detail is a small change's changed lines, as the text shows them
	// inline ("-  return x", "+  return x + 1").
	Detail []string `json:"detail,omitempty"`
}

// RecapShow is one symbol (or, for --show <path>, one whole file) recap
// --show printed.
type RecapShow struct {
	Change *RecapChange `json:"change,omitempty"` // the symbol's change
	File   string       `json:"file,omitempty"`   // --show <path>: the file
	Diff   []string     `json:"diff"`             // its diff, one line each, starting with ' ', '-' or '+'
}

// Crux is crux's result.
type Crux struct {
	Command    []string   `json:"command"`
	Exit       int        `json:"exit"`             // the command's exit code (crux exits with it too)
	Signal     string     `json:"signal,omitempty"` // the signal that killed the command ("SIGKILL")
	DurationMS int64      `json:"duration_ms"`
	Tools      []string   `json:"tools"`           // recognised formats: "go test", "cargo", "pytest", …
	Tests      *CruxTests `json:"tests,omitempty"` // when tests ran
	Errors     []CruxItem `json:"errors"`
	Failures   []CruxItem `json:"failures"`
	Warnings   []CruxItem `json:"warnings"`
	Notes      []string   `json:"notes,omitempty"` // summary lines worth keeping
	Tail       []string   `json:"tail,omitempty"`  // the last meaningful lines, when nothing else explains the result
	Lines      int        `json:"lines"`           // lines of output in total
	Log        string     `json:"log,omitempty"`   // the full output's path
}

// CruxTests are test counts.
type CruxTests struct {
	Passed     int  `json:"passed"`
	Failed     int  `json:"failed"`
	Skipped    int  `json:"skipped"`
	XFailed    int  `json:"xfailed,omitempty"`
	XPassed    int  `json:"xpassed,omitempty"`
	Errors     int  `json:"errors,omitempty"` // errors outside tests (pytest)
	PkgOK      int  `json:"packages_ok,omitempty"`
	PkgFailed  int  `json:"packages_failed,omitempty"`
	PkgNoTests int  `json:"packages_without_tests,omitempty"`
	PkgNoRun   int  `json:"packages_no_run,omitempty"` // go test packages where no test matched -run
	NoneRan    bool `json:"none_ran,omitempty"`
	// CountsUnknown means per-test counts weren't reported (go test without
	// -v), so zeros don't mean nothing passed.
	CountsUnknown bool `json:"counts_unknown,omitempty"`
}

// CruxItem is an error, warning or test failure.
type CruxItem struct {
	Kind    string     `json:"kind"` // "error", "warning", "fail"
	Title   string     `json:"title"`
	Loc     *Location  `json:"loc,omitempty"`
	Test    string     `json:"test,omitempty"`    // a failure: ref of the failing test's definition
	Pkg     string     `json:"pkg,omitempty"`     // a go test failure without a line: its package directory
	Missing bool       `json:"missing,omitempty"` // the location's file doesn't exist from here
	Body    []string   `json:"body,omitempty"`
	Count   int        `json:"count,omitempty"` // occurrences folded into this one
	Locs    []Location `json:"locs,omitempty"`  // their locations
	Names   []string   `json:"names,omitempty"` // folded warnings: the distinct names in them
}

// Glint is glint's result: a listing, or one file's outline.
type Glint struct {
	Items *[]GlintEntry `json:"items,omitempty"` // a listing (glint --json); present, maybe empty, for a listing
	Dir   string        `json:"dir,omitempty"`   // with items: the absolute directory their paths are relative to
	File  *GlintFile    `json:"file,omitempty"`  // one file (glint -A --json <file>)
}

// GlintEntry is a file or directory in a listing. Its path is as listed
// (relative to the working directory, like ls).
type GlintEntry struct {
	Name       string       `json:"name"`
	Path       string       `json:"path"`
	Type       string       `json:"type"`
	Kind       string       `json:"kind"`
	Size       int64        `json:"size"`
	DirSize    *int64       `json:"dir_size,omitempty"`
	Mode       string       `json:"mode"`
	Octal      string       `json:"octal"`
	Modified   time.Time    `json:"modified"`
	Owner      string       `json:"owner,omitempty"`
	Group      string       `json:"group,omitempty"`
	LinkTarget string       `json:"link_target,omitempty"` // a symlink's target ("target" is reserved for refs)
	Broken     bool         `json:"broken,omitempty"`
	Git        string       `json:"git,omitempty"`
	Peek       string       `json:"peek,omitempty"`
	Commit     *GlintCommit `json:"last_commit,omitempty"`
	Error      string       `json:"error,omitempty"`
	Children   []GlintEntry `json:"children,omitempty"`
}

// GlintCommit is the last commit touching an entry.
type GlintCommit struct {
	Hash    string    `json:"hash"`
	Subject string    `json:"subject"`
	Author  string    `json:"author"`
	Time    time.Time `json:"time"`
	Count   int       `json:"commits"`
}

// GlintFile is one file's facts and outline.
type GlintFile struct {
	Path       string       `json:"path"` // repo-relative inside the repository, else absolute
	Lines      int          `json:"lines"`
	Size       int64        `json:"size"`
	Modified   time.Time    `json:"modified"`
	Mode       string       `json:"mode,omitempty"`
	Kind       string       `json:"kind,omitempty"`  // "binary", "image", "generated", "vendored", …
	Image      string       `json:"image,omitempty"` // an image's size: "1360x800"
	LinkTarget string       `json:"link_target,omitempty"`
	Git        string       `json:"git,omitempty"` // git status: "modified", "untracked", "ignored", …
	Commit     *GlintCommit `json:"last_commit,omitempty"`
	Symbols    []Symbol     `json:"symbols"`
}
