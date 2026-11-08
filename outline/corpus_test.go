package outline_test

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
)

var update = flag.Bool("update", false, "rewrite testdata/corpus.golden from the parser's output")

// TestCorpus is the outline diff check: every symbol Parse finds in
// testdata/corpus, with its kind, qualified name, lines, ref and
// signature, must match testdata/corpus.golden. A parser change shows up
// as a diff of that file; review it, then regenerate with
// `go test ./outline -update`.
func TestCorpus(t *testing.T) {
	got := corpus(t, "testdata/corpus")
	const golden = "testdata/corpus.golden"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run `go test ./outline -update` to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("outline of testdata/corpus differs from %s (if the change is intended, run `go test ./outline -update` and review the diff):\n%s",
			golden, lineDiff(string(want), string(got)))
	}
}

// corpus lists every symbol of every file under dir, sorted by path:
//
//	path
//	  start-end kind qualified-name ref | signature
func corpus(t *testing.T, dir string) []byte {
	t.Helper()
	var b bytes.Buffer
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !outline.Supported(rel) {
			return fmt.Errorf("%s: outline has no rules for this file type", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		syms := outline.Parse(rel, data, 1<<20)
		refs := ref.Refs(ref.Targets(rel, syms))
		fmt.Fprintf(&b, "%s\n", rel)
		for i, s := range syms {
			fmt.Fprintf(&b, "  %d-%d %s %s %s | %s\n", s.Line, s.End, s.Kind, outline.Qualified(syms, i), refs[i], s.Sig)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// lineDiff shows the lines only in want (-) and only in got (+), in order;
// enough to see what a parser change did without a diff library.
func lineDiff(want, got string) string {
	a, b := strings.Split(want, "\n"), strings.Split(got, "\n")
	// longest common subsequence, then walk it
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out strings.Builder
	file, shown := "", "" // the file the lines belong to, and the last header printed
	emit := func(sign, line string) {
		if !strings.HasPrefix(line, " ") {
			file, shown = line, line
		} else if file != shown {
			out.WriteString(" " + file + "\n")
			shown = file
		}
		out.WriteString(sign + line + "\n")
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			if !strings.HasPrefix(a[i], " ") {
				file = a[i]
			}
			i, j = i+1, j+1
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			emit("-", a[i])
			i++
		default:
			emit("+", b[j])
			j++
		}
	}
	return out.String()
}
