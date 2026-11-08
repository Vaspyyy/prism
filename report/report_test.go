package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/repo"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var when = time.Date(2026, 11, 8, 5, 56, 0, 0, time.FixedZone("CET", 3600))

func envelope(tool string, args []string, result any) *Envelope {
	e := New(tool, "1.2.3", args, &repo.Info{Root: "/work/shop", Git: true, GitDir: "/work/shop/.git",
		Head: "b681ef1c0ffee", Branch: "main", DefaultBranch: "main"})
	e.Result = result
	return e
}

var list = Symbol{Ref: "internal/files/files.go:List", Path: "internal/files/files.go", Name: "List", Kind: "func",
	Line: 73, End: 107, Doc: []int{70, 72}, Sig: "func List(roots []string, o Options) ([]string, error)"}

// samples is one envelope per tool result, covering the shared objects;
// each has a golden file in testdata.
var samples = map[string]*Envelope{
	"seek": envelope("seek", []string{"internal/files/files.go:List"}, Seek{
		Pattern: "List", Target: "internal/files/files.go:List",
		Definitions: []Symbol{list},
		Groups: []SeekGroup{
			{Path: "main.go", Symbol: &Symbol{Ref: "main.go:run", Path: "main.go", Name: "run", Kind: "func", Line: 81, End: 200},
				Hits: []SeekHit{{Line: 104, Col: 22, Text: "paths, err := files.List(o.paths, opts)"}}},
			{Path: "internal/files/files.go", Hits: []SeekHit{{Line: 32, Text: "// Nested lists what List left out", Label: "comment"}}},
		},
		Counts: SeekCounts{Hits: 2, Files: 2, Definitions: 1},
	}),
	"recap": envelope("recap", []string{"show", "b681ef1"}, Recap{
		Old: "b681ef1^", New: "b681ef1",
		Commit: &Commit{Rev: "b681ef1c0ffee", Author: "Ada", Date: when, Subject: "List: match prefixes", Parents: []string{"a1b2c3d"}},
		Files: []RecapFile{{
			Path: "internal/files/files.go", Status: "modified", Added: 3, Removed: 1, Lines: 272,
			Changes: []RecapChange{
				{What: "changed", Symbol: list, Added: 1, Removed: 1},
				{What: "renamed", Symbol: Symbol{Ref: "internal/files/files.go:globSet.matches", Path: "internal/files/files.go", Name: "globSet.matches", Kind: "func", Line: 187, End: 201},
					OldRef: "internal/files/files.go:globSet.match", Added: 2},
			},
			Outside: &RecapOutside{Added: 1, Lines: []string{"+var debug = false"}},
		}},
	}),
	"crux": envelope("crux", []string{"go", "test", "./..."}, Crux{
		Command: []string{"go", "test", "./..."}, Exit: 1, DurationMS: 154, Tools: []string{"go test"},
		Tests:    &CruxTests{Passed: 41, Failed: 1, PkgOK: 3, PkgFailed: 1},
		Errors:   []CruxItem{},
		Failures: []CruxItem{{Kind: "fail", Title: "TestList", Test: "internal/files/files_test.go:TestList", Body: []string{"got 2 files, want 3"}, Loc: &Location{Path: "internal/files/files_test.go", Line: 31, In: "internal/files/files_test.go:check"}}},
		Warnings: []CruxItem{},
		Lines:    12, Log: "/home/u/.local/state/crux/logs/x.log",
	}),
	"glint": envelope("glint", []string{"-A", "--json", "internal/files/files.go"}, Glint{
		File: &GlintFile{Path: "internal/files/files.go", Lines: 272, Size: 7000, Modified: when, Git: "modified", Symbols: []Symbol{list}},
	}),
	"error": func() *Envelope {
		e := envelope("seek", []string{"files.go:Open"}, nil)
		e.Fail(CodeAmbiguous, `"Open" matches 2 definitions`, "a.go:Repo.Open", "b.go:File.Open")
		return e
	}(),
}

func TestGolden(t *testing.T) {
	for name, e := range samples {
		var buf bytes.Buffer
		if err := Write(&buf, e); err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join("testdata", name+".json")
		if *update {
			if err := os.WriteFile(golden, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("%v (run go test -update to create it)", err)
		}
		if buf.String() != string(want) {
			t.Errorf("%s: JSON differs from %s (run go test -update if the change is intended):\n%s", name, golden, buf.String())
		}
		if _, err := Check(buf.Bytes()); err != nil {
			t.Errorf("%s: Check: %v", name, err)
		}
	}
}

func TestWriteDoesNotEscapeHTML(t *testing.T) {
	e := envelope("glint", nil, Glint{File: &GlintFile{Path: "a.rs", Symbols: []Symbol{{Ref: "a.rs:Grid", Sig: "impl<'a> Grid<'a> for &T"}}}})
	var buf bytes.Buffer
	if err := Write(&buf, e); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "impl<'a> Grid<'a> for &T") {
		t.Errorf("signature escaped:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"args": []`) {
		t.Errorf("nil args should print as []:\n%s", buf.String())
	}
}

func TestNewOutsideGit(t *testing.T) {
	e := New("seek", "1", nil, &repo.Info{Root: "/tmp/x"})
	if e.Repo.Root != "/tmp/x" || e.Repo.Git || e.Repo.Dirty || e.Links == nil || e.Next == nil {
		t.Errorf("outside git: %+v", e)
	}
	if e := New("seek", "1", nil, nil); e.Repo.Root != "" {
		t.Errorf("nil info: %+v", e.Repo)
	}
}

func TestCheck(t *testing.T) {
	var good bytes.Buffer
	Write(&good, samples["seek"])
	d, err := Check(good.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if d.Tool != "seek" || !strings.Contains(string(d.Result), `"pattern": "List"`) {
		t.Errorf("Decoded: tool %q, result %s", d.Tool, d.Result)
	}
	for _, c := range []struct {
		name, edit, want string
	}{
		{"wrong schema", `"schema": "prism/1"=>"schema": "prism/2"`, `schema "prism/2"`},
		{"no schema", `"schema": "prism/1",=>`, "no schema"},
		{"no links", `"links": [],=>`, "no links"},
		{"no root", `"root": "/work/shop",=>`, "no repo.root"},
		{"no result", `"result"=>"other"`, "neither result nor error"},
		{"both", `"links": []=>"error": {"code": "failed", "message": "x"}, "links": []`, "both result and error"},
		{"not json", `{=>`, "not a JSON envelope"},
	} {
		from, to, _ := strings.Cut(c.edit, "=>")
		bad := strings.Replace(good.String(), from, to, 1)
		if _, err := Check([]byte(bad)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Check error %v, want it to mention %q", c.name, err, c.want)
		}
	}
	if _, err := Check(append(good.Bytes(), good.Bytes()...)); err == nil {
		t.Error("two documents should fail")
	}
}

func TestRefs(t *testing.T) {
	var buf bytes.Buffer
	Write(&buf, samples["crux"])
	got, err := Refs(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/files/files_test.go:check", "internal/files/files_test.go:TestList"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Refs(crux) = %q, want %q", got, want)
	}
	buf.Reset()
	Write(&buf, samples["recap"])
	got, _ = Refs(buf.Bytes())
	want = []string{"internal/files/files.go:List", "internal/files/files.go:globSet.match", "internal/files/files.go:globSet.matches"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Refs(recap) = %q, want %q", got, want)
	}
}

func TestSymbols(t *testing.T) {
	src := "package a\n\nfunc init() {}\n\nfunc init() {}\n\ntype Repo struct{}\n\nfunc (r *Repo) Open() error {\n\treturn nil\n}\n"
	got := Symbols("a/a.go", outline.Parse("a.go", []byte(src), 100))
	var refs []string
	for _, s := range got {
		refs = append(refs, s.Ref)
	}
	want := []string{"a/a.go:init#1", "a/a.go:init#2", "a/a.go:Repo", "a/a.go:Repo.Open"}
	if !reflect.DeepEqual(refs, want) {
		t.Errorf("Symbols refs = %q, want %q", refs, want)
	}
	if got[3].Name != "Repo.Open" || got[3].Line != 9 || got[3].End != 11 || got[3].Sig != "func (r *Repo) Open() error" {
		t.Errorf("Repo.Open: %+v", got[3])
	}
}

// Lists a mode produces are present even when empty, so programs can range
// over them; and a symlink's target isn't mistaken for a ref.
func TestEmptyListsAndLinkTargets(t *testing.T) {
	none := []string{}
	noItems := []GlintEntry{}
	for name, c := range map[string]struct {
		result any
		want   string
	}{
		"seek -l": {Seek{Pattern: "x", Definitions: []Symbol{}, Groups: []SeekGroup{}, Files: &none}, `"files": []`},
		"listing": {Glint{Items: &noItems, Dir: "/work"}, `"items": []`},
	} {
		var buf bytes.Buffer
		Write(&buf, envelope("t", nil, c.result))
		if !strings.Contains(buf.String(), c.want) {
			t.Errorf("%s: want %s in\n%s", name, c.want, buf.String())
		}
	}
	var buf bytes.Buffer
	items := []GlintEntry{{Name: "link", Path: "link", LinkTarget: "a.go:Open"}}
	Write(&buf, envelope("glint", nil, Glint{Items: &items}))
	if refs, _ := Refs(buf.Bytes()); len(refs) != 0 {
		t.Errorf("a symlink target was taken for a ref: %q", refs)
	}
	buf.Reset()
	Write(&buf, envelope("seek", nil, Seek{Pattern: "x", Definitions: []Symbol{}, Groups: []SeekGroup{}}))
	if strings.Contains(buf.String(), `"files": [`) || strings.Contains(buf.String(), `"files": null`) {
		t.Errorf("files outside -l:\n%s", buf.String())
	}
}
