package report

import (
	"reflect"
	"testing"
)

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"internal/files/files.go:List": "internal/files/files.go:List",
		"^TestList$":                   "'^TestList$'",
		"shop.go:init#2":               "'shop.go:init#2'",
		"it's":                         `'it'\''s'`,
		"":                             "''",
		"a b":                          "'a b'",
		"~/x":                          "'~/x'",
		"main...feature":               "main...feature",
		"src/a.go:A@HEAD~1":            "'src/a.go:A@HEAD~1'",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
	if got := Cmd("go", "test", "-run", "^(TestA|TestB)$", "./internal/files/"); got != "go test -run '^(TestA|TestB)$' ./internal/files/" {
		t.Errorf("Cmd = %s", got)
	}
}

func TestNextLines(t *testing.T) {
	got := NextLines([]Next{
		{Cmd: "seek internal/files/files.go:List", Why: "definition and callers"},
		{Cmd: "crux go test -run '^TestList$' ./internal/files/", Why: "runs the 1 test that mentions List"},
		{Cmd: "recap", Why: ""},
		{Cmd: "dropped", Why: "a fourth"},
	})
	want := []string{
		"next: seek internal/files/files.go:List                  definition and callers",
		"next: crux go test -run '^TestList$' ./internal/files/   runs the 1 test that mentions List",
		"next: recap",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NextLines:\n%q\nwant\n%q", got, want)
	}
}
