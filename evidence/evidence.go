// Package evidence names how a link between two results is known, and the
// words each basis allows (docs/evidence.md).
//
// A link's relation and its basis come as a pair: "mentions" is only ever
// known by reference, "covers" only by coverage. No combination of weaker
// links makes a stronger one, so there is no function here that combines
// them.
package evidence

import (
	"fmt"
	"strconv"

	"github.com/Vaspyyy/prism/report"
)

// Basis levels, weakest first.
const (
	Name      = "name"      // naming convention only: TestList next to List
	Reference = "reference" // the name appears as a whole word inside the other symbol
	Location  = "location"  // a line lies inside the symbol's range
	Execution = "execution" // the test ran and crux saw its result
	Coverage  = "coverage"  // coverage data shows the test executed the symbol's lines
)

// Relations. Each one needs exactly one basis.
const (
	NamedLike = "named-like" // basis Name
	Mentions  = "mentions"   // basis Reference
	Contains  = "contains"   // basis Location: the symbol contains the location
	Ran       = "ran"        // basis Execution
	Covers    = "covers"     // basis Coverage
)

var levels = []string{Name, Reference, Location, Execution, Coverage}

var needs = map[string]string{NamedLike: Name, Mentions: Reference, Contains: Location, Ran: Execution, Covers: Coverage}

// Strength orders bases from weakest (0) to strongest; -1 for an unknown
// basis.
func Strength(basis string) int {
	for i, b := range levels {
		if b == basis {
			return i
		}
	}
	return -1
}

// Needs returns the basis a relation needs ("" for an unknown relation).
func Needs(rel string) string { return needs[rel] }

// Link builds a link, refusing a relation its basis doesn't allow.
func Link(from, to, rel, basis, at string) (report.Link, error) {
	l := report.Link{From: from, To: to, Rel: rel, Basis: basis, At: at}
	return l, Check(l)
}

// Check reports a link whose relation its basis doesn't allow.
func Check(l report.Link) error {
	need := needs[l.Rel]
	switch {
	case need == "":
		return fmt.Errorf("link %s -> %s: unknown relation %q", l.From, l.To, l.Rel)
	case Strength(l.Basis) < 0:
		return fmt.Errorf("link %s -> %s: unknown basis %q", l.From, l.To, l.Basis)
	case need != l.Basis:
		return fmt.Errorf("link %s -> %s: %q needs basis %q, not %q", l.From, l.To, l.Rel, need, l.Basis)
	}
	return nil
}

// CheckAll checks every link of an envelope.
func CheckAll(e *report.Envelope) error {
	for _, l := range e.Links {
		if err := Check(l); err != nil {
			return err
		}
	}
	return nil
}

// TestsReason is the reason for a suggested command that runs n tests
// linked to a symbol by rel: "runs the 1 test that mentions List", "runs
// the 2 tests named like List". caveat, if not empty, says what else the
// command may run ("may also run tests with similar names") and is
// appended. It panics on Ran: a test that already ran isn't a reason to
// pick it, and its outcome is a separate statement.
func TestsReason(rel string, n int, name, caveat string) string {
	tests := "the " + strconv.Itoa(n) + " test"
	if n != 1 {
		tests += "s"
	}
	var s string
	switch rel {
	case NamedLike:
		s = "runs " + tests + " named like " + name
	case Mentions:
		s = "runs " + tests + " that " + verb(n, "mentions", "mention") + " " + name
	case Contains:
		s = "runs " + tests + " that " + verb(n, "contains", "contain") + " " + name
	case Covers:
		s = "runs " + tests + " that " + verb(n, "covers", "cover") + " " + name
	default:
		panic("evidence.TestsReason: no wording for " + strconv.Quote(rel))
	}
	if caveat != "" {
		s += "; " + caveat
	}
	return s
}

func verb(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
