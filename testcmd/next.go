package testcmd

import (
	"github.com/Vaspyyy/prism/evidence"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/report"
)

// Next turns the tests linked to a symbol into suggestions that run them
// through crux, one per command, with the reason worded by the link:
//
//	crux go test -run '^TestList$' ./internal/files/   runs the 1 test that mentions List
//
// rel is how the tests are linked (evidence.Mentions for tests seek found
// the name in); name is the symbol's name as the reader knows it. Tests
// are filtered with IsTest only by the caller, who has the source.
func Next(root, cwd string, tests []ref.Target, rel, name string) []report.Next {
	var out []report.Next
	for _, c := range For(root, cwd, tests) {
		out = append(out, report.Next{Cmd: "crux " + c.String(), Why: evidence.TestsReason(rel, c.Tests, name, c.Caveat)})
	}
	return out
}
