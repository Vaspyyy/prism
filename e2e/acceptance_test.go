package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/evidence"
	"github.com/Vaspyyy/prism/focus"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/repo"
	"github.com/Vaspyyy/prism/report"
)

// step is one command of the chain and what it printed.
type step struct {
	cmd  string
	out  string
	code int
	next []report.Next
	doc  *report.Decoded // with --json
	data []byte
}

const (
	listRef = "internal/files/files.go:List"
	testRef = "internal/files/files_test.go:TestList"
	cruxCmd = `crux go test -run '^TestList$' ./internal/files/`
)

// overclaim is any wording that needs coverage data, which v1 never has.
var overclaim = regexp.MustCompile(`(?i)\bcover(s|ed|age)?\b`)

// TestAcceptance is the v1 acceptance test (docs/roadmap.md): recap, seek
// and crux chained through their own next: lines, never claiming more than
// the evidence; the same as JSON; and the same minus context with
// --no-context. The fixture's commit changes List and breaks TestList.
func TestAcceptance(t *testing.T) {
	e := setup(t)
	t.Setenv("PRISM_NO_CONTEXT", "")
	t.Setenv("PRISM_SESSION", "acceptance") // one agent session for the whole run
	info, err := repo.Detect(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	commit := info.Head[:7]

	on := e.chain(t, commit)
	e.checkText(t, on, commit, true)

	// focus and last run were remembered for this session
	store := focus.Open(info)
	if f := store.Focus(); f == nil || !f.Has(listRef) || f.New != info.Head {
		t.Errorf("recap show didn't record focus on List: %+v", f)
	}
	if r := store.LastRun(); r == nil || r.Exit == 0 || !slices.Contains(r.Tests, testRef) {
		t.Errorf("crux didn't record TestList failing: %+v", r)
	}

	asJSON := e.chain(t, commit, "--json")
	e.checkJSON(t, asJSON, on)

	off := e.chain(t, commit, "--no-context")
	e.checkText(t, off, commit, false)
	for i := range on {
		bodyOn, nextOn := normalize(on[i].out)
		bodyOff, nextOff := normalize(off[i].out)
		if bodyOn != bodyOff {
			t.Errorf("step %d (%s): --no-context changed more than marks and the context: line\n--- context on ---\n%s\n--- --no-context ---\n%s", i+1, off[i].cmd, bodyOn, bodyOff)
		}
		for _, n := range nextOff {
			if !slices.Contains(nextOn, n) {
				t.Errorf("step %d: --no-context suggests %q, which context-on didn't", i+1, n)
			}
		}
	}
}

// chain runs the three steps, each command taken from the previous step's
// next: lines. flags go right after each tool's name.
func (e *env) chain(t *testing.T, commit string, flags ...string) []step {
	t.Helper()
	asJSON := slices.Contains(flags, "--json")
	var steps []step
	run := func(cmd string) step {
		t.Helper()
		s := step{cmd: withFlags(cmd, flags...)}
		s.out, s.code = e.sh(s.cmd)
		if !asJSON {
			t.Logf("$ %s\n%s", s.cmd, s.out)
		}
		if overclaim.MatchString(s.out) {
			t.Errorf("%s: claims coverage it has no data for:\n%s", s.cmd, s.out)
		}
		if asJSON {
			d, err := report.Check([]byte(s.out))
			if err != nil {
				t.Fatalf("%s: not a valid envelope: %v\n%s", s.cmd, err, s.out)
			}
			s.doc, s.data, s.next = d, []byte(s.out), d.Next
		} else {
			s.next = nextLines(s.out)
		}
		steps = append(steps, s)
		return s
	}
	pick := func(s step, want string) string {
		t.Helper()
		for _, n := range s.next {
			if n.Cmd == want {
				return n.Cmd
			}
		}
		t.Fatalf("%s: no next: %s among %+v\n%s", s.cmd, want, s.next, s.out)
		return ""
	}
	s := run("recap show " + commit)
	s = run(pick(s, "seek "+listRef))
	run(pick(s, cruxCmd))
	return steps
}

// checkText checks what the roadmap asks of each step's text report.
func (e *env) checkText(t *testing.T, steps []step, commit string, context bool) {
	t.Helper()
	recap, seek, crux := steps[0], steps[1], steps[2]

	// 1. recap lists List as changed and suggests seek
	if !slices.Contains(grouped(recap.out, recapFile, recapChange), listRef) {
		t.Errorf("recap show: List not listed as changed:\n%s", recap.out)
	}

	// 2. seek: the definition from its func line, marked [focus] with
	// context; the hits in run and TestList, the test labelled "mentions";
	// the crux command with its reason
	header := regexp.MustCompile(`(?m)^seek: ` + regexp.QuoteMeta(listRef) + `( \[focus\])? \(func, L7-\d+\)`)
	m := header.FindStringSubmatch(seek.out)
	switch {
	case m == nil:
		t.Errorf("seek: no definition header for List from line 7:\n%s", seek.out)
	case context && m[1] == "":
		t.Errorf("seek: List isn't marked [focus]:\n%s", seek.out)
	case !context && m[1] != "":
		t.Errorf("seek --no-context: List is marked:\n%s", seek.out)
	}
	if context && !strings.HasPrefix(seek.out, "context: ") || !context && strings.Contains(seek.out, "context: ") {
		t.Errorf("seek: context: line (context %v):\n%s", context, seek.out)
	}
	if !regexp.MustCompile(`(?m)^\s+func run\b`).MatchString(seek.out) {
		t.Errorf("seek: no hit in run:\n%s", seek.out)
	}
	if !regexp.MustCompile(`(?m)^\s+func TestList\b.*\bmentions List\b`).MatchString(seek.out) {
		t.Errorf("seek: TestList isn't labelled as mentioning List:\n%s", seek.out)
	}
	if !slices.Contains(seek.next, report.Next{Cmd: cruxCmd, Why: "runs the 1 test that mentions List"}) {
		t.Errorf("seek: next: lines %+v, want %s with its reason", seek.next, cruxCmd)
	}

	// 3. crux reports the failure in TestList
	if crux.code == 0 || !regexp.MustCompile(`(?m)^\s+in `+regexp.QuoteMeta(testRef)+marks+`$`).MatchString(crux.out) {
		t.Errorf("crux: exit %d, want TestList's failure with its in line:\n%s", crux.code, crux.out)
	}
	if !context {
		for _, s := range steps {
			if strings.Contains(s.out, "context: ") || markRe.MatchString(s.out) {
				t.Errorf("%s: context shows with --no-context:\n%s", s.cmd, s.out)
			}
		}
	}
}

// checkJSON checks the JSON run: every document is an envelope with
// context, its links are honest, its next: items are the text run's, and
// every symbol ref in it is accepted by seek and, for changed symbols,
// recap.
func (e *env) checkJSON(t *testing.T, steps, text []step) {
	t.Helper()
	var refs []string
	for i, s := range steps {
		if s.doc.Context == nil {
			t.Errorf("%s: context is null with context on", s.cmd)
		}
		var env report.Envelope
		if err := json.Unmarshal(s.data, &env); err != nil {
			t.Fatal(err)
		}
		if err := evidence.CheckAll(&env); err != nil {
			t.Errorf("%s: %v", s.cmd, err)
		}
		var cmds, textCmds []string
		for _, n := range s.next {
			cmds = append(cmds, n.Cmd)
		}
		for _, n := range text[i].next {
			textCmds = append(textCmds, n.Cmd)
		}
		if !slices.Equal(cmds, textCmds) {
			t.Errorf("step %d: JSON next %q, text next %q", i+1, cmds, textCmds)
		}
		rs, err := report.Refs(s.data)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, rs...)
	}

	var sk report.Seek
	decode(t, steps[1].doc, &sk)
	if len(sk.Definitions) == 0 || sk.Definitions[0].Ref != listRef || !slices.Contains(sk.Definitions[0].Marks, "focus") {
		t.Errorf("seek --json: definitions %+v, want List marked focus", sk.Definitions)
	}
	var env report.Envelope
	json.Unmarshal(steps[1].data, &env)
	if !slices.ContainsFunc(env.Links, func(l report.Link) bool {
		return l.From == testRef && l.To == listRef && l.Rel == evidence.Mentions && l.Basis == evidence.Reference
	}) {
		t.Errorf("seek --json: no mentions link from TestList to List: %+v", env.Links)
	}
	var cx report.Crux
	decode(t, steps[2].doc, &cx)
	if len(cx.Failures) == 0 || cx.Failures[0].Loc == nil || cx.Failures[0].Loc.In != testRef {
		t.Errorf("crux --json: failures %+v", cx.Failures)
	}

	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r] || !ref.Parse(r, nil).HasTarget {
			continue
		}
		seen[r] = true
		if out, code := e.run("seek", "-A", "-b", "20", r); code != 0 || !regexp.MustCompile(`^seek: `+regexp.QuoteMeta(r)+marks+` `).MatchString(stripContext(out)) {
			t.Errorf("seek %s (a ref from the JSON): exit %d\n%s", r, code, out)
		}
	}
}

// sh runs a command line as printed, with the built tools first on PATH.
func (e *env) sh(line string) (string, int) {
	e.t.Helper()
	cmd := exec.Command("sh", "-c", line)
	cmd.Dir = e.dir
	cmd.Env = append(os.Environ(), "PATH="+e.bin+string(filepath.ListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatalf("sh -c %q: %v", line, err)
	}
	return string(out), code
}

// withFlags puts flags right after the tool's name: "crux go test" becomes
// "crux --json go test".
func withFlags(line string, flags ...string) string {
	if len(flags) == 0 {
		return line
	}
	tool, rest, _ := strings.Cut(line, " ")
	return strings.Join(append(append([]string{tool}, flags...), rest), " ")
}

var nextLine = regexp.MustCompile(`^next: (.+?)(?: {3,}(.+))?$`)

// nextLines reads a text report's next: lines.
func nextLines(out string) []report.Next {
	var ns []report.Next
	for _, l := range strings.Split(out, "\n") {
		if m := nextLine.FindStringSubmatch(l); m != nil {
			ns = append(ns, report.Next{Cmd: m[1], Why: m[2]})
		}
	}
	return ns
}

var (
	markRe     = regexp.MustCompile(` \[(?:changed|branch|focus|failing)\]`)
	durationRe = regexp.MustCompile(`\b\d+(?:\.\d+)?(?:ms|s|m)\b`)
	logRe      = regexp.MustCompile(`(?m)^full log: .*$`)
	spacesRe   = regexp.MustCompile(` {2,}`)
)

// normalize splits a text report into its body without context (no
// context: line, no marks, durations and log paths blanked) and its next:
// lines with whitespace collapsed.
func normalize(out string) (string, []string) {
	var body, next []string
	for _, l := range strings.Split(stripContext(out), "\n") {
		l = markRe.ReplaceAllString(l, "")
		if strings.HasPrefix(l, "next: ") {
			next = append(next, spacesRe.ReplaceAllString(l, " "))
			continue
		}
		body = append(body, l)
	}
	b := strings.Join(body, "\n")
	b = logRe.ReplaceAllString(durationRe.ReplaceAllString(b, "<t>"), "full log: <log>")
	return b, next
}

// stripContext drops a report's context: line.
func stripContext(out string) string {
	if strings.HasPrefix(out, "context: ") {
		if _, rest, ok := strings.Cut(out, "\n"); ok {
			return rest
		}
	}
	return out
}
