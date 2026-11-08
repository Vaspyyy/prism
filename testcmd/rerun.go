package testcmd

import (
	"cmp"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Vaspyyy/prism/evidence"
	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/report"
)

// Failed is a test that failed in a run.
type Failed struct {
	ref.Target // the test, as a symbol IsTest accepts

	// Printed is the test as the runner printed it, if known: cargo's test
	// path ("parser::tests::works"), pytest's node id
	// ("tests/test_x.py::test_p[2]"), go's test name ("TestA/bad" for a
	// subtest). A cargo rerun selects that path rather than the one the
	// file's place in the crate gives, a pytest rerun that
	// parametrization, and a go rerun that subtest.
	Printed string
}

// Rerun builds the command that reruns only the failed tests of a run of
// argv, keeping argv's own flags (-race, -tags, -count=1, -x, --release,
// --test-threads …): go test, pytest (also python -m pytest) and cargo
// test. root is the repository root (absolute); cwd is where argv ran and
// where the rerun will run.
//
// ok is false whenever the command can't be exact: another runner, a flag
// it doesn't know (it can't tell whether the next word is the flag's value
// or a package), a mode that doesn't just run tests (go test -fuzz, -list,
// -c; pytest --collect-only, --stepwise; cargo test --doc, --no-run), or
// failed tests the runner as invoked can't select by name.
func Rerun(root, cwd string, argv []string, failed []Failed) (Cmd, bool) {
	if len(argv) == 0 || len(failed) == 0 {
		return Cmd{}, false
	}
	switch name := filepath.Base(argv[0]); {
	case name == "go":
		return goRerun(root, cwd, argv, failed)
	case name == "pytest" || name == "py.test":
		return pytestRerun(root, cwd, argv[:1], argv[1:], failed)
	case isPython(name) && len(argv) >= 3 && argv[1] == "-m" && argv[2] == "pytest":
		return pytestRerun(root, cwd, argv[:3], argv[3:], failed)
	case name == "cargo":
		return cargoRerun(root, cwd, argv, failed)
	}
	return Cmd{}, false
}

// RerunNext is Rerun as a crux suggestion, with the reason worded by
// execution: "reruns the 2 failed tests".
func RerunNext(root, cwd string, argv []string, failed []Failed) (report.Next, bool) {
	c, ok := Rerun(root, cwd, argv, failed)
	if !ok {
		return report.Next{}, false
	}
	return report.Next{Cmd: "crux " + c.String(), Why: evidence.RerunReason(c.Tests, c.Caveat)}, true
}

func isPython(name string) bool {
	rest, ok := strings.CutPrefix(name, "python")
	return ok && strings.Trim(rest, "0123456789.") == ""
}

// all reports whether every test is in a file with the suffix.
func all(ts []Failed, suffix string) bool {
	for _, t := range ts {
		if !strings.HasSuffix(t.Path, suffix) {
			return false
		}
	}
	return true
}

// Go test and build flags (go help testflag, go help build) by what a
// rerun does with them. A flag missing here makes the rerun give up.
var (
	goValue = set( // take a value: -flag v or -flag=v
		"p", "asmflags", "buildmode", "compiler", "gccgoflags", "gcflags", "installsuffix", "ldflags",
		"mod", "modfile", "overlay", "pgo", "pkgdir", "tags", "toolexec", "covermode", "coverpkg",
		"debug-actiongraph", "debug-runtime-trace", "debug-trace",
		"count", "coverprofile", "cpu", "parallel", "skip", "timeout", "vet", "shuffle", "exec",
		"blockprofile", "blockprofilerate", "cpuprofile", "memprofile", "memprofilerate",
		"mutexprofile", "mutexprofilefraction", "outputdir", "trace", "fuzztime", "fuzzminimizetime",
		"run", "bench", "benchtime")
	goBool = set( // don't: -flag or -flag=true
		"a", "race", "msan", "asan", "cover", "v", "work", "x", "linkshared", "modcacherw", "trimpath",
		"buildvcs", "failfast", "fullpath", "json", "short", "benchmem", "artifacts")
	goDrop   = set("run", "bench", "benchtime")   // replaced, or would run benchmarks
	goGiveUp = set("fuzz", "list", "c", "o", "n") // don't (only) run tests
)

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// goRerun: go [-C dir] test <argv's flags> -run '^(A|B)$' <failed packages>
// [-args …], or -run '^A$/^(x|y)$|^B$/^z$' for failed subtests
// (goRerunSelect).
// Packages and -run are replaced, -bench and -benchtime dropped; argv's
// packages only tell which module the run was in.
func goRerun(root, cwd string, argv []string, failed []Failed) (Cmd, bool) {
	if !all(failed, "_test.go") {
		return Cmd{}, false
	}
	var head []string // go [-C dir] test [-C dir]
	i := 1
	base := cwd
	chdir := func() bool {
		if i+1 >= len(argv) {
			return false
		}
		head = append(head, argv[i], argv[i+1])
		if d := argv[i+1]; filepath.IsAbs(d) {
			base = d
		} else {
			base = filepath.Join(cwd, d)
		}
		i += 2
		return true
	}
	head = append(head, argv[0])
	if i < len(argv) && argv[i] == "-C" && !chdir() {
		return Cmd{}, false
	}
	if i >= len(argv) || argv[i] != "test" {
		return Cmd{}, false
	}
	head = append(head, "test")
	i++
	if i < len(argv) && argv[i] == "-C" && len(head) == 2 && !chdir() {
		return Cmd{}, false
	}
	var flags, after []string
	inPkgs, seenPkgs := false, false
	for ; i < len(argv); i++ {
		a := argv[i]
		if a == "-args" || a == "--args" || a == "--" {
			after = argv[i:] // to the test binary (with -- itself), as in the run
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			if seenPkgs && !inPkgs {
				// go test's rule: a word after the packages and a flag goes, with
				// all that follows, to the test binary; -args keeps it there
				after = append([]string{"-args"}, argv[i:]...)
				break
			}
			if strings.HasSuffix(a, ".go") {
				return Cmd{}, false // files, not packages: a rerun by package builds something else
			}
			inPkgs, seenPkgs = true, true
			continue // a package: replaced
		}
		inPkgs = false
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		if goGiveUp[name] || !goValue[name] && !goBool[name] {
			return Cmd{}, false
		}
		flag := []string{a}
		if goValue[name] && !hasValue { // the value is the next word, whatever it looks like
			if i+1 >= len(argv) {
				return Cmd{}, false
			}
			i++
			value = argv[i]
			flag = append(flag, value)
		}
		if name == "run" && strings.Contains(value, "/") {
			return Cmd{}, false // a subtest filter: rerunning the whole test would run more than ran
		}
		if !goDrop[name] {
			flags = append(flags, flag...)
		}
	}
	for _, a := range after {
		f := strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-")
		for _, bad := range []string{"test.run", "test.bench", "test.list", "test.fuzz"} {
			if f == bad || strings.HasPrefix(f, bad+"=") {
				return Cmd{}, false // the test binary's own filter would override the rerun's
			}
		}
	}
	// one module, which the run could reach from where it ran
	mod := ""
	for _, t := range failed {
		m := up(root, filepath.Dir(filepath.Join(root, filepath.FromSlash(t.Path))), "go.mod", nil)
		if m == "" || mod != "" && m != mod {
			return Cmd{}, false
		}
		mod = m
	}
	if !within(mod, base) && up(root, base, "go.work", nil) == "" {
		return Cmd{}, false
	}
	sel, ok := goRerunSelect(root, mod, failed)
	if !ok {
		return Cmd{}, false
	}
	args := append(append(head, flags...), "-run", sel.re)
	for _, d := range sel.dirs {
		args = append(args, rel(base, d, true))
	}
	return Cmd{Args: append(args, after...), Tests: sel.n, Caveat: sel.caveat}, true
}

// goRerunSelect is goSelect by the names go test printed, for a run in
// module mod: a failed subtest is selected by its path, each level quoted
// (TestA/with_space, as go prints "with space": -run
// '^TestA$/^with_space$'). Subtests under one parent at one depth share a
// group ('^TestA$/^outer$/^(a|b)$'), and groups are alternatives
// ('^TestA$/^bad$|^TestB$/^x$/^y$'), which go test splits on since go
// 1.18; for a module that allows an older go, the parent tests are
// selected instead. A test that failed on its own as well as through a
// subtest reruns whole. The caveat says which tests also rerun passing
// subtests. A printed name that isn't the test's own, or a subtest of it,
// gives no selection.
func goRerunSelect(root, mod string, failed []Failed) (goSelection, bool) {
	ts := make([]ref.Target, len(failed))
	byDir := map[string]map[string]bool{} // package directory: printed names
	var paths []string                    // distinct printed names
	for i, t := range failed {
		ts[i] = t.Target
		p := cmp.Or(t.Printed, t.Name)
		if p != t.Name && !strings.HasPrefix(p, t.Name+"/") || slices.Contains(strings.Split(p, "/"), "") {
			return goSelection{}, false
		}
		d := filepath.Dir(filepath.Join(root, filepath.FromSlash(t.Path)))
		if byDir[d] == nil {
			byDir[d] = map[string]bool{}
		}
		byDir[d][p] = true
		if !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	sel := goSelect(root, ts)
	sel.n = 0
	for _, ps := range byDir {
		sel.n += len(ps)
	}
	if !slices.ContainsFunc(paths, func(p string) bool { return strings.Contains(p, "/") }) {
		return sel, true // tests only: goSelect's filter
	}
	slices.Sort(paths)
	var alts, whole []string // what -run selects; tests that rerun whole
	for _, p := range paths {
		if i := slices.IndexFunc(alts, func(a string) bool { return strings.HasPrefix(p, a+"/") }); i >= 0 {
			if !slices.Contains(whole, alts[i]) {
				whole = append(whole, alts[i]) // failed on its own, and through p
			}
			continue
		}
		alts = append(alts, p)
	}
	groups := goGroups(alts)
	if len(groups) > 1 && !goAlternates(mod) {
		var tops []string
		for _, a := range alts {
			top, _, sub := strings.Cut(a, "/")
			if !slices.Contains(tops, top) {
				tops = append(tops, top)
			}
			if sub && !slices.Contains(whole, top) {
				whole = append(whole, top)
			}
		}
		for i, w := range whole {
			whole[i], _, _ = strings.Cut(w, "/")
		}
		slices.Sort(whole)
		whole = slices.Compact(whole)
		alts, groups = tops, [][]string{tops}
	}
	var re []string
	for _, g := range groups {
		re = append(re, goGroupRun(g))
	}
	sel.re = strings.Join(re, "|")
	var caveats []string
	if len(whole) > 0 {
		caveats = append(caveats, "also reruns passing subtests of "+andList(whole))
	}
	if goSameName(sel.dirs, byDir, alts) {
		caveats = append(caveats, "also runs tests of the same name in the other packages")
	}
	sel.caveat = strings.Join(caveats, "; ")
	return sel, true
}

// goGroups groups sorted names by their parent (all levels but the last),
// in order.
func goGroups(names []string) [][]string {
	var groups [][]string
	parent := func(n string) string {
		i := strings.LastIndex(n, "/")
		return n[:max(i, 0)]
	}
next:
	for _, n := range names {
		for i, g := range groups {
			if parent(g[0]) == parent(n) && strings.Count(g[0], "/") == strings.Count(n, "/") {
				groups[i] = append(g, n)
				continue next
			}
		}
		groups = append(groups, []string{n})
	}
	return groups
}

// goGroupRun is the -run filter for one group, each level quoted:
// ^TestA$/^outer$/^(a|b)$.
func goGroupRun(g []string) string {
	levels := strings.Split(g[0], "/")
	var re []string
	for _, l := range levels[:len(levels)-1] {
		re = append(re, "^"+regexp.QuoteMeta(l)+"$")
	}
	var last []string
	for _, n := range g {
		last = append(last, regexp.QuoteMeta(n[strings.LastIndex(n, "/")+1:]))
	}
	if len(last) == 1 {
		re = append(re, "^"+last[0]+"$")
	} else {
		re = append(re, "^("+strings.Join(last, "|")+")$")
	}
	return strings.Join(re, "/")
}

// goAlternates reports whether the module in dir requires a go whose test
// -run splits on |: go 1.18 or later, by its go.mod's go line.
func goAlternates(dir string) bool {
	data, ok := outline.Read(filepath.Join(dir, "go.mod"))
	if !ok {
		return false
	}
	for _, l := range strings.Split(string(data), "\n") {
		l, _, _ = strings.Cut(l, "//")
		if f := strings.Fields(l); len(f) == 2 && f[0] == "go" {
			minor, ok := strings.CutPrefix(f[1], "1.")
			if !ok {
				return false
			}
			n := 0
			for _, c := range minor {
				if c < '0' || c > '9' {
					break
				}
				n = n*10 + int(c-'0')
			}
			return n >= 18
		}
	}
	return false // no go line: go 1.16
}

// goSameName reports whether -run, which applies to every listed package,
// selects in one of several packages a test of the same name as one that
// failed only in another: one of the package's tests where nothing in the
// selected test, or around it, failed.
func goSameName(dirs []string, byDir map[string]map[string]bool, alts []string) bool {
	if len(dirs) < 2 {
		return false
	}
	for _, d := range dirs {
		tests := goTests(d)
		for _, a := range alts {
			top, _, _ := strings.Cut(a, "/")
			related := false
			for p := range byDir[d] {
				related = related || p == a || strings.HasPrefix(p, a+"/") || strings.HasPrefix(a, p+"/")
			}
			if slices.Contains(tests, top) && !related {
				return true
			}
		}
	}
	return false
}

// andList is "a", "a and b", "a, b and c".
func andList(s []string) string {
	if len(s) == 1 {
		return s[0]
	}
	return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
}

// pytest options by what a rerun does with them; an option missing here
// makes it give up. Plugins' options are known only where common (xdist,
// pytest-cov, pytest-timeout).
var (
	pyValue = set( // take a value: --opt v or --opt=v; short ones also -kv
		"-k", "-m", "-p", "-c", "-o", "-W", "-r", "-n",
		"--rootdir", "--basetemp", "--confcutdir", "--deselect", "--ignore", "--ignore-glob", "--maxfail",
		"--tb", "--durations", "--durations-min", "--junitxml", "--junit-xml", "--junitprefix", "--junit-prefix",
		"--log-level", "--log-format", "--log-date-format", "--log-file", "--log-file-level",
		"--log-file-format", "--log-file-date-format", "--log-cli-level", "--log-cli-format",
		"--log-cli-date-format", "--capture", "--import-mode", "--color", "--code-highlight",
		"--override-ini", "--config-file", "--pythonwarnings", "--report",
		"--numprocesses", "--maxprocesses", "--dist", "--cov-report", "--cov-config", "--cov-fail-under",
		"--timeout")
	pyBool = set(
		"-x", "-v", "-q", "-s", "-l",
		"--exitfirst", "--verbose", "--quiet", "--showlocals", "--strict-markers", "--strict-config",
		"--strict", "--runxfail", "--disable-warnings", "--disable-pytest-warnings", "--no-header",
		"--no-summary", "--full-trace", "--setup-show", "--pdb", "--trace", "--cache-clear",
		"--lf", "--last-failed", "--ff", "--failed-first", "--nf", "--new-first", "--doctest-modules",
		"--cov-append", "--no-cov", "--no-cov-on-fail")
	pyOptional = set("--cov") // argparse nargs='?': takes the next word unless it's an option
	// plugins' options with a value: pytest reads its config before it
	// loads plugins, and takes a value given as the next word for a path
	pyPlugin = set("-n", "--numprocesses", "--maxprocesses", "--dist", "--cov-report", "--cov-config",
		"--cov-fail-under", "--timeout", "--cov")
)

// pytestRerun: <argv's options> [-c <config>] <node ids>. Positional paths
// become the failed tests' node ids, relative to where pytest runs; -k, -m
// and the like stay, as the failed tests matched them. Where the node ids
// would make pytest settle on another config file or rootdir than the run
// did, -c (or --rootdir) pins the run's.
func pytestRerun(root, cwd string, prefix, args []string, failed []Failed) (Cmd, bool) {
	if !all(failed, ".py") {
		return Cmd{}, false
	}
	opts := []string{}
	var early, plugin []string // what pytest's first parse takes for paths: all, and plugin options' values
	ini, rootArg := "", ""
	value := func(name, v string, separate bool) {
		switch name {
		case "-c", "--config-file":
			ini = v
		case "--rootdir":
			rootArg = v
		}
		if separate && pyPlugin[name] {
			early, plugin = append(early, v), append(plugin, v)
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--" || !strings.HasPrefix(a, "-") || a == "-":
			pos := []string{a}
			if a == "--" {
				pos, i = args[i+1:], len(args) // the rest is positional
			}
			for _, p := range pos {
				if _, after, ok := strings.Cut(p, "::"); ok && strings.Contains(after, "[") {
					return Cmd{}, false // one parametrization: the rerun would run them all
				}
			}
			early = append(early, pos...) // paths and node ids: replaced
		case strings.HasPrefix(a, "--"):
			name, v, hasValue := strings.Cut(a, "=")
			switch {
			case pyValue[name] && !hasValue:
				if i+1 >= len(args) {
					return Cmd{}, false
				}
				opts = append(opts, a, args[i+1])
				value(name, args[i+1], true)
				i++
			case pyValue[name]:
				opts = append(opts, a)
				value(name, v, false)
			case pyBool[name] && !hasValue:
				opts = append(opts, a)
			case pyOptional[name] && !hasValue && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
				opts = append(opts, a, args[i+1])
				value(name, args[i+1], true)
				i++
			case pyOptional[name]:
				opts = append(opts, a)
			default:
				return Cmd{}, false
			}
		default: // short options, maybe clustered (-xvs) or with the value attached (-kfoo, -rA)
			opt, attached, ok := pyShort(a)
			if !ok {
				return Cmd{}, false
			}
			opts = append(opts, a)
			switch {
			case opt == "":
			case attached != "":
				value(opt, attached, false)
			case i+1 >= len(args):
				return Cmd{}, false
			default:
				opts = append(opts, args[i+1])
				value(opt, args[i+1], true)
				i++
			}
		}
	}
	var ids []string
	for _, t := range failed {
		param, ok := pyParam(t.Printed, t.Name)
		if !ok {
			return Cmd{}, false
		}
		id := rel(cwd, filepath.Join(root, filepath.FromSlash(t.Path)), false) + "::" + strings.ReplaceAll(t.Name, ".", "::") + param
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	args = append(append([]string{}, prefix...), opts...)
	if ini == "" { // with -c, both settle on that file
		run, ok1 := pytestSetup(cwd, early, rootArg)
		re, ok2 := pytestSetup(cwd, append(plugin, ids...), rootArg)
		switch {
		case !ok1 || !ok2:
			return Cmd{}, false
		case run == re:
		case run.ini != "":
			args = append(args, "-c", rel(cwd, run.ini, false))
		case re.ini == "":
			args = append(args, "--rootdir", rel(cwd, run.rootdir, false))
		default:
			return Cmd{}, false // the run had no config file; the rerun would
		}
	}
	return Cmd{Args: append(args, ids...), Tests: len(ids)}, true
}

// pyParam is the parametrization part of the node id pytest printed for
// a test: "[2]" of tests/test_x.py::test_p[2]. ok is false when the id is
// another test's.
func pyParam(printed, name string) (string, bool) {
	if printed == "" {
		return "", true
	}
	tail := "::" + strings.ReplaceAll(name, ".", "::")
	if strings.HasSuffix(printed, tail) && strings.HasSuffix(printed[:len(printed)-len(tail)], ".py") {
		return "", true
	}
	if i := strings.Index(printed, tail+"["); i >= 0 && strings.HasSuffix(printed[:i], ".py") && strings.HasSuffix(printed, "]") {
		return printed[i+len(tail):], true
	}
	return "", false
}

// pyShort checks a cluster of short options: each a known flag, ending at
// one that takes a value (opt), with the value attached or else the next
// word.
func pyShort(a string) (opt, attached string, ok bool) {
	letters := a[1:]
	for j, c := range letters {
		o := "-" + string(c)
		switch {
		case pyValue[o]:
			attached = letters[j+1:]
			if j == 0 { // -c=x is -c x
				attached = strings.TrimPrefix(attached, "=")
			}
			return o, attached, true
		case !pyBool[o]:
			return "", "", false
		}
	}
	return "", "", true
}

// cargo test options and libtest's (after --), by what a rerun does with
// them; one missing here makes it give up.
var (
	cargoValue = set("-p", "--package", "--exclude", "--test", "--bench", "--bin", "--example", "-F",
		"--features", "--target", "--target-dir", "--manifest-path", "-j", "--jobs", "--profile", "--color",
		"--message-format", "--config", "-Z")
	cargoBool = set("--lib", "--bins", "--tests", "--benches", "--examples", "--all-targets", "--no-fail-fast",
		"--workspace", "--all", "--all-features", "--no-default-features", "--release", "-r", "--locked",
		"--offline", "--frozen", "-q", "--quiet", "-v", "-vv", "--verbose", "--ignore-rust-version",
		"--future-incompat-report")
	libtestValue = set("--test-threads", "--skip", "--color", "--format", "-Z", "--shuffle-seed", "--logfile")
	libtestBool  = set("--exact", "--nocapture", "--no-capture", "--ignored", "--include-ignored",
		"--show-output", "--quiet", "-q", "--test", "--report-time", "--ensure-time", "--shuffle",
		"--force-run-in-process")
)

// cargoRerun: cargo [+toolchain] test <cargo options> -- <libtest
// options> --exact <test paths>. The filters (a TESTNAME before --, words
// after it) become the failed tests' paths: the ones cargo printed, else
// the ones their files' places give (not where those don't tell). Package
// and target selection stay.
func cargoRerun(root, cwd string, argv []string, failed []Failed) (Cmd, bool) {
	if !all(failed, ".rs") {
		return Cmd{}, false
	}
	i := 1
	head := []string{argv[0]}
	if i < len(argv) && strings.HasPrefix(argv[i], "+") {
		head = append(head, argv[i])
		i++
	}
	if i >= len(argv) || argv[i] != "test" {
		return Cmd{}, false
	}
	head = append(head, "test")
	i++
	var opts, lib, pkgs, exclude []string
	var sel cargoSel
	workspace, manifest := false, ""
	for ; i < len(argv) && argv[i] != "--"; i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			continue // TESTNAME: replaced
		}
		name, value, hasValue := strings.Cut(a, "=")
		switch {
		case cargoValue[name] && !hasValue:
			if i+1 >= len(argv) {
				return Cmd{}, false
			}
			value = argv[i+1]
			opts = append(opts, a, value)
			i++
		case cargoValue[name], cargoBool[name] && !hasValue:
			opts = append(opts, a)
		default:
			return Cmd{}, false // --doc, --no-run and anything unknown
		}
		switch name {
		case "-p", "--package":
			pkgs = append(pkgs, value)
		case "--exclude":
			exclude = append(exclude, value)
		case "--workspace", "--all":
			workspace = true
		case "--manifest-path":
			manifest = value
		}
		sel.add(name, value)
	}
	for i++; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			continue // a filter: replaced
		}
		name, _, hasValue := strings.Cut(a, "=")
		switch {
		case libtestValue[name] && !hasValue:
			if i+1 >= len(argv) {
				return Cmd{}, false
			}
			lib = append(lib, a, argv[i+1])
			i++
		case libtestValue[name], libtestBool[name] && !hasValue:
			if name != "--exact" { // added once below
				lib = append(lib, a)
			}
		default:
			return Cmd{}, false // --list, --bench and anything unknown
		}
	}
	ts := make([]ref.Target, len(failed))
	for i, t := range failed {
		ts[i] = t.Target
	}
	crates := map[string]*rustCrate{}
	load := func(dir string) *rustCrate {
		if crates[dir] == nil {
			crates[dir] = loadCrate(dir, lastSegments(ts))
		}
		return crates[dir]
	}
	var es []rustEntry
	for _, t := range failed {
		abs := filepath.Join(root, filepath.FromSlash(t.Path))
		dir := up(root, filepath.Dir(abs), "Cargo.toml", []byte("[package]"))
		if dir == "" {
			return Cmd{}, false
		}
		c := load(dir)
		mod, own, known := c.place(rel(dir, abs, false))
		e := rustEntry{crate: c, own: own, known: known && !c.opaque(), file: t.Path, path: t.Printed}
		q := strings.ReplaceAll(t.Name, ".", "::")
		switch {
		case e.path == "" && !e.known:
			return Cmd{}, false // the file's place doesn't give its path
		case e.path == "":
			e.path = rustJoin(mod, t.Name)
		case e.path != q && !strings.HasSuffix(e.path, "::"+q):
			return Cmd{}, false // printed for another test
		}
		es = append(es, e)
	}
	scope, ok := cargoScope(root, cwd, pkgs, exclude, workspace, manifest, load)
	if !ok {
		return Cmd{}, false
	}
	shared, ok := rustCaveat(scope, sel, es)
	if !ok {
		return Cmd{}, false
	}
	filters, n := rustFilters(es)
	c := Cmd{Tests: n}
	if shared {
		c.Caveat = "also runs tests with the same path in the other targets"
	}
	args := append(append(head, opts...), "--")
	args = append(append(args, lib...), "--exact")
	c.Args = append(args, filters...)
	return c, true
}

// rustFilters are the entries' paths, sorted, and how many tests they
// are: the same path in two files is two tests.
func rustFilters(es []rustEntry) (filters []string, n int) {
	seen := map[string]bool{}
	for _, e := range es {
		if !slices.Contains(filters, e.path) {
			filters = append(filters, e.path)
		}
		if k := e.file + "\x00" + e.path; !seen[k] {
			seen[k] = true
			n++
		}
	}
	slices.Sort(filters)
	return filters, n
}
