// Package e2e checks that the four tools agree on refs: every ref one of
// them prints is accepted by the others (docs/roadmap.md, "Phase 1: refs").
//
// It runs the real binaries, so it's skipped unless PRISM_E2E_BIN names a
// directory containing glint, seek, recap and crux. CI builds them against
// this checkout of Prism (see .github/workflows/ci.yml).
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

	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/report"
)

// the fixture: a commit that changes List and breaks the test that calls it
var files = map[string]string{
	"go.mod": "module example.com/shop\n\ngo 1.26\n",
	"internal/files/files.go": `package files

import "strings"

// List returns the names that contain the pattern,
// in their order.
func List(names []string, pattern string) []string {
	var out []string
	for _, n := range names {
		if strings.Contains(n, pattern) {
			out = append(out, n)
		}
	}
	return out
}

type globSet struct{ globs []string }

func (g globSet) match(p string) bool {
	for _, gl := range g.globs {
		if strings.HasSuffix(p, gl) {
			return true
		}
	}
	return false
}
`,
	"internal/files/files_test.go": `package files

import "testing"

func TestList(t *testing.T) {
	got := List([]string{"a.go", "b.md"}, ".go")
	if len(got) != 1 {
		t.Fatalf("got %d names, want 1", len(got))
	}
}
`,
	"main.go": `package main

import (
	"fmt"

	"example.com/shop/internal/files"
)

func init() {}

func init() {}

func run() []string {
	return files.List([]string{"a.go"}, ".go")
}

func main() { fmt.Println(run()) }
`,
}

type env struct {
	t   *testing.T
	bin string
	dir string
}

func setup(t *testing.T) *env {
	bin := os.Getenv("PRISM_E2E_BIN")
	if bin == "" {
		t.Skip("PRISM_E2E_BIN not set: the end-to-end test needs built glint, seek, recap and crux")
	}
	for _, tool := range []string{"glint", "seek", "recap", "crux"} {
		if _, err := os.Stat(filepath.Join(bin, tool)); err != nil {
			t.Fatalf("%s missing in PRISM_E2E_BIN: %v", tool, err)
		}
	}
	e := &env{t: t, bin: bin, dir: t.TempDir()}
	for _, kv := range [][2]string{
		{"GIT_CONFIG_GLOBAL", os.DevNull}, {"GIT_CONFIG_NOSYSTEM", "1"}, {"NO_COLOR", "1"},
		{"GIT_AUTHOR_NAME", "Test"}, {"GIT_AUTHOR_EMAIL", "test@example.com"},
		{"GIT_COMMITTER_NAME", "Test"}, {"GIT_COMMITTER_EMAIL", "test@example.com"},
	} {
		t.Setenv(kv[0], kv[1])
	}
	for p, content := range files {
		e.write(p, content)
	}
	e.git("init", "-q", "-b", "main")
	e.git("add", ".")
	e.git("commit", "-q", "-m", "first")
	e.write("internal/files/files.go", strings.Replace(files["internal/files/files.go"], "strings.Contains(n, pattern)", "strings.HasPrefix(n, pattern)", 1))
	e.git("commit", "-q", "-am", "List: match prefixes")
	return e
}

func (e *env) write(p, content string) {
	e.t.Helper()
	full := filepath.Join(e.dir, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) git(args ...string) {
	e.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = e.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// run runs a tool in the fixture and returns its output and exit code.
func (e *env) run(tool string, args ...string) (string, int) {
	e.t.Helper()
	cmd := exec.Command(filepath.Join(e.bin, tool), args...)
	cmd.Dir = e.dir
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		e.t.Fatalf("%s %v: %v", tool, args, err)
	}
	return string(out), code
}

var (
	// recap: a file header ("path  modified, …"), then change lines
	// ("  changed  func List  L7-15  +1 −1")
	recapFile   = regexp.MustCompile(`^(\S+)  (?:modified|new|deleted|renamed|moved)`)
	recapChange = regexp.MustCompile(`^  (?:changed|added|removed|renamed|moved) +\S+ +(\S+) +L\d+`)
	// glint: a header ("path — 27L …"), then "  12 name  signature" when the
	// signature doesn't show the name
	glintFile = regexp.MustCompile(`^(\S+) — \d+L`)
	glintName = regexp.MustCompile(`^  \d+ (\S+)  `)
	// seek -d: "path:Name  L7-15  signature"
	seekDef = regexp.MustCompile(`^(\S+:\S+)  L\d+`)
	// crux: "    in path:Name" and "    test path:Name"
	cruxRef = regexp.MustCompile(`^\s+(?:in|test) (\S+:\S+)$`)
)

func grouped(out string, header, line *regexp.Regexp) []string {
	var refs []string
	file := ""
	for _, l := range strings.Split(out, "\n") {
		if m := header.FindStringSubmatch(l); m != nil {
			file = m[1]
		} else if m := line.FindStringSubmatch(l); m != nil && file != "" {
			refs = append(refs, file+":"+m[1])
		}
	}
	return refs
}

func lines(out string, re *regexp.Regexp) []string {
	var refs []string
	for _, l := range strings.Split(out, "\n") {
		if m := re.FindStringSubmatch(l); m != nil {
			refs = append(refs, m[1])
		}
	}
	return refs
}

func need(t *testing.T, tool string, refs []string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(refs, w) {
			t.Errorf("%s printed %q, want it to include %s", tool, refs, w)
		}
	}
}

// TestRefsRoundTrip follows the roadmap's acceptance idea for Phase 1:
// collect the refs each tool prints, then hand every one to the tools that
// take symbols.
func TestRefsRoundTrip(t *testing.T) {
	e := setup(t)

	out, _ := e.run("recap", "show", "-A", "HEAD")
	changed := grouped(out, recapFile, recapChange)
	need(t, "recap show", changed, "internal/files/files.go:List")

	var printed []string
	for _, f := range []string{"internal/files/files.go", "main.go"} {
		out, _ := e.run("glint", "-A", f)
		printed = append(printed, grouped(out, glintFile, glintName)...)
	}
	need(t, "glint -A", printed, "internal/files/files.go:globSet.match", "main.go:init#1", "main.go:init#2")

	out, _ = e.run("seek", "-A", "-d", "List")
	defs := lines(out, seekDef)
	need(t, "seek -d", defs, "internal/files/files.go:List")
	printed = append(printed, defs...)

	out, code := e.run("crux", "-A", "go", "test", "./...")
	if code == 0 {
		t.Fatalf("crux go test passed, but the fixture's test should fail:\n%s", out)
	}
	failed := lines(out, cruxRef)
	need(t, "crux", failed, "internal/files/files_test.go:TestList")
	printed = append(printed, failed...)
	printed = append(printed, changed...)

	for _, r := range printed {
		out, code := e.run("seek", "-A", "-b", "20", r)
		if code != 0 || !strings.HasPrefix(out, "seek: "+r) {
			t.Errorf("seek %s: exit %d, want the definition:\n%s", r, code, out)
		}
	}
	for _, r := range changed {
		out, code := e.run("recap", "show", "-A", "HEAD", "--show", r)
		if code != 0 || !strings.Contains(out, "List") {
			t.Errorf("recap show HEAD --show %s: exit %d:\n%s", r, code, out)
		}
	}
	// a crux location goes straight into recap: the line of the change
	out, code = e.run("recap", "show", "-A", "HEAD", "--show", "internal/files/files.go:10")
	if code != 0 || !strings.Contains(out, "List") {
		t.Errorf("recap --show with a location: exit %d:\n%s", code, out)
	}
	t.Logf("%d refs from 4 tools, all accepted", len(printed))
}

// runJSON runs a tool with --json and checks the envelope's shape.
func (e *env) runJSON(tool string, args ...string) (*report.Decoded, []byte, int) {
	e.t.Helper()
	out, code := e.run(tool, args...)
	d, err := report.Check([]byte(out))
	if err != nil {
		e.t.Fatalf("%s %v: not a valid envelope: %v\n%s", tool, args, err, out)
	}
	if d.Tool != tool {
		e.t.Errorf("%s %v: envelope says tool %q", tool, args, d.Tool)
	}
	return d, []byte(out), code
}

func decode(t *testing.T, d *report.Decoded, v any) {
	t.Helper()
	if err := json.Unmarshal(d.Result, v); err != nil {
		t.Fatalf("%s result: %v\n%s", d.Tool, err, d.Result)
	}
}

// TestJSONRoundTrip is Phase 2's done criterion: every tool's --json is a
// valid envelope, and every ref in it is accepted by the tools that take
// symbols.
func TestJSONRoundTrip(t *testing.T) {
	e := setup(t)
	var refs []string
	collect := func(data []byte) {
		rs, err := report.Refs(data)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, rs...)
	}

	d, data, _ := e.runJSON("recap", "--json", "show", "HEAD")
	var rc report.Recap
	decode(t, d, &rc)
	var changed []string
	for _, f := range rc.Files {
		for _, c := range f.Changes {
			changed = append(changed, c.Symbol.Ref)
		}
	}
	need(t, "recap --json show", changed, "internal/files/files.go:List")
	if rc.Commit == nil || rc.Commit.Subject != "List: match prefixes" || rc.Commit.Date.IsZero() {
		t.Errorf("recap --json show: commit %+v", rc.Commit)
	}
	collect(data)

	var outlined []string
	for _, f := range []string{"internal/files/files.go", "main.go"} {
		d, data, _ := e.runJSON("glint", "-A", "--json", f)
		var g report.Glint
		decode(t, d, &g)
		if g.File == nil || g.File.Path != f {
			t.Fatalf("glint -A --json %s: file %+v", f, g.File)
		}
		for _, s := range g.File.Symbols {
			outlined = append(outlined, s.Ref)
		}
		collect(data)
	}
	need(t, "glint -A --json", outlined, "internal/files/files.go:globSet.match", "main.go:init#1", "main.go:init#2")

	d, _, _ = e.runJSON("glint", "--json", "internal")
	var listing report.Glint
	decode(t, d, &listing)
	if listing.Items == nil || len(*listing.Items) == 0 || listing.Dir == "" {
		t.Errorf("glint --json listing: %+v", listing)
	}

	d, data, _ = e.runJSON("seek", "--json", "-d", "List")
	var sk report.Seek
	decode(t, d, &sk)
	if len(sk.Definitions) != 1 || sk.Definitions[0].Ref != "internal/files/files.go:List" {
		t.Errorf("seek --json -d List: definitions %+v", sk.Definitions)
	}
	collect(data)

	d, data, code := e.runJSON("crux", "--json", "go", "test", "./...")
	var cx report.Crux
	decode(t, d, &cx)
	if code == 0 || cx.Exit == 0 || len(cx.Failures) == 0 {
		t.Fatalf("crux --json go test: exit %d, result %+v", code, cx)
	}
	if f := cx.Failures[0]; f.Loc == nil || f.Loc.In != "internal/files/files_test.go:TestList" {
		t.Errorf("crux --json: first failure %+v", f)
	}
	collect(data)

	// a ref that names nothing is an error envelope with candidates
	d, _, code = e.runJSON("seek", "--json", "internal/files/files.go:Nope")
	if code == 0 || d.Error == nil || d.Error.Code != report.CodeNoMatch || !slices.Contains(d.Error.Candidates, "internal/files/files.go:List") {
		t.Errorf("seek --json with an unknown ref: exit %d, error %+v", code, d.Error)
	}

	n := 0
	for _, r := range refs {
		if !ref.Parse(r, nil).HasTarget {
			continue // a file, not a symbol
		}
		n++
		d, _, code := e.runJSON("seek", "--json", r)
		var s report.Seek
		if d.Error == nil {
			decode(t, d, &s)
		}
		if code != 0 || len(s.Definitions) == 0 {
			t.Errorf("seek --json %s: exit %d, error %+v, definitions %+v", r, code, d.Error, s.Definitions)
		}
	}
	for _, r := range changed {
		d, _, code := e.runJSON("recap", "--json", "show", "HEAD", "--show", r)
		var s report.Recap
		if d.Error == nil {
			decode(t, d, &s)
		}
		if code != 0 || len(s.Show) == 0 {
			t.Errorf("recap --json show HEAD --show %s: exit %d, error %+v", r, code, d.Error)
		}
	}
	t.Logf("%d symbol refs from the JSON of 4 tools, all accepted", n)
}
