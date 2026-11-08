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
	Files       []string    `json:"files,omitempty"`  // -l: the paths with hits, and nothing else
	Counts      SeekCounts  `json:"counts"`
}

// SeekGroup is the hits in one definition, or outside any.
type SeekGroup struct {
	Path   string    `json:"path"`
	Symbol *Symbol   `json:"symbol"` // null for lines outside any definition
	Hits   []SeekHit `json:"hits"`
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
	Files       int `json:"files"`
	Definitions int `json:"definitions"`
}

// Recap is recap's result: a comparison (recap, recap show) or a log.
type Recap struct {
	Old     string      `json:"old"`               // the old side as named: "b681ef1^", "HEAD", "index"
	New     string      `json:"new"`               // the new side: "b681ef1", "working tree"
	Commit  *Commit     `json:"commit,omitempty"`  // recap show
	Files   []RecapFile `json:"files"`             // empty for recap log
	Commits []RecapLog  `json:"commits,omitempty"` // recap log
	Show    []RecapShow `json:"show,omitempty"`    // --show: the symbols asked for, with their diffs
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
	Path      string        `json:"path"`
	OldPath   string        `json:"old_path,omitempty"` // renamed or moved
	Status    string        `json:"status"`             // "modified", "new", "deleted", "renamed", "untracked", "conflict", …
	Added     int           `json:"added"`
	Removed   int           `json:"removed"`
	Lines     int           `json:"lines,omitempty"`     // lines in the new version (the old one for deleted files)
	Generated string        `json:"generated,omitempty"` // "lock file", "generated", "minified", "vendored"
	Binary    bool          `json:"binary,omitempty"`
	Changes   []RecapChange `json:"changes"`
	Outside   *LineCounts   `json:"outside,omitempty"` // changed lines outside any definition
	Defs      []Symbol      `json:"defs,omitempty"`    // new and deleted files: their definitions
}

// LineCounts are added and removed lines.
type LineCounts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
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
}

// RecapShow is one symbol recap --show printed.
type RecapShow struct {
	Change RecapChange `json:"change"`
	Diff   []string    `json:"diff"` // its diff, one line each, starting with ' ', '-' or '+'
}

// Crux is crux's result.
type Crux struct {
	Command    []string   `json:"command"`
	Exit       int        `json:"exit"` // the command's exit code (crux exits with it too)
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
	NoneRan    bool `json:"none_ran,omitempty"`
}

// CruxItem is an error, warning or test failure.
type CruxItem struct {
	Kind  string     `json:"kind"` // "error", "warning", "fail"
	Title string     `json:"title"`
	Loc   *Location  `json:"loc,omitempty"`
	Test  string     `json:"test,omitempty"` // a failure: ref of the failing test's definition
	Body  []string   `json:"body,omitempty"`
	Count int        `json:"count,omitempty"` // occurrences folded into this one
	Locs  []Location `json:"locs,omitempty"`  // their locations
}

// Glint is glint's result: a listing, or one file's outline.
type Glint struct {
	Items []GlintEntry `json:"items,omitempty"` // a listing (glint --json)
	File  *GlintFile   `json:"file,omitempty"`  // one file (glint -A --json <file>)
}

// GlintEntry is a file or directory in a listing. Its path is as listed
// (relative to the working directory, like ls).
type GlintEntry struct {
	Name     string       `json:"name"`
	Path     string       `json:"path"`
	Type     string       `json:"type"`
	Kind     string       `json:"kind"`
	Size     int64        `json:"size"`
	DirSize  *int64       `json:"dir_size,omitempty"`
	Mode     string       `json:"mode"`
	Octal    string       `json:"octal"`
	Modified time.Time    `json:"modified"`
	Owner    string       `json:"owner,omitempty"`
	Group    string       `json:"group,omitempty"`
	Target   string       `json:"target,omitempty"`
	Broken   bool         `json:"broken,omitempty"`
	Git      string       `json:"git,omitempty"`
	Peek     string       `json:"peek,omitempty"`
	Commit   *GlintCommit `json:"last_commit,omitempty"`
	Error    string       `json:"error,omitempty"`
	Children []GlintEntry `json:"children,omitempty"`
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
	Path    string   `json:"path"` // repo-relative inside the repository, else absolute
	Lines   int      `json:"lines"`
	Size    int64    `json:"size"`
	Symbols []Symbol `json:"symbols"`
}
