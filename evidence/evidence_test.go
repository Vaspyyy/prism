package evidence

import (
	"strings"
	"testing"

	"github.com/Vaspyyy/prism/report"
)

func TestPairs(t *testing.T) {
	for rel, basis := range needs {
		if _, err := Link("a.go:A", "b.go:B", rel, basis, ""); err != nil {
			t.Errorf("%s/%s: %v", rel, basis, err)
		}
	}
	// never upgrade: a stronger basis doesn't license a weaker relation's
	// word either, and no weaker basis licenses "covers"
	for _, b := range levels {
		if b == Coverage {
			continue
		}
		if err := Check(report.Link{From: "t", To: "s", Rel: Covers, Basis: b}); err == nil {
			t.Errorf("covers allowed with basis %s", b)
		}
	}
	if Check(report.Link{Rel: Mentions, Basis: Execution}) == nil || Check(report.Link{Rel: "tests", Basis: Name}) == nil || Check(report.Link{Rel: Mentions, Basis: "hunch"}) == nil {
		t.Error("bad links passed")
	}
	if !(Strength(Name) < Strength(Reference) && Strength(Reference) < Strength(Location) && Strength(Location) < Strength(Execution) && Strength(Execution) < Strength(Coverage)) || Strength("x") != -1 {
		t.Error("order")
	}
	e := report.New("t", "0", nil, nil)
	e.Links = append(e.Links, report.Link{From: "a", To: "b", Rel: Mentions, Basis: Reference}, report.Link{From: "a", To: "b", Rel: Covers, Basis: Reference})
	if CheckAll(e) == nil {
		t.Error("CheckAll missed an upgraded link")
	}
}

func TestTestsReason(t *testing.T) {
	for _, c := range []struct {
		rel    string
		n      int
		caveat string
		want   string
	}{
		{Mentions, 1, "", "runs the 1 test that mentions List"},
		{Mentions, 2, "", "runs the 2 tests that mention List"},
		{NamedLike, 3, "", "runs the 3 tests named like List"},
		{Mentions, 2, "may also run tests with similar names", "runs the 2 tests that mention List; may also run tests with similar names"},
	} {
		if got := TestsReason(c.rel, c.n, "List", c.caveat); got != c.want {
			t.Errorf("TestsReason(%s, %d) = %q, want %q", c.rel, c.n, got, c.want)
		}
	}
	for _, rel := range []string{Mentions, NamedLike, Contains} {
		if s := TestsReason(rel, 2, "List", ""); strings.Contains(s, "cover") {
			t.Errorf("%s claims coverage: %q", rel, s)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("Ran has no wording, should panic")
		}
	}()
	TestsReason(Ran, 1, "List", "")
}
