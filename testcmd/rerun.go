package testcmd

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Vaspyyy/prism/evidence"
	"github.com/Vaspyyy/prism/ref"
	"github.com/Vaspyyy/prism/report"
)

// Rerun builds the command that reruns only the failed tests of a run of
// argv, keeping argv's own flags (-race, -tags, -count=1, -x, --release,
// --test-threads …): go test, pytest (also python -m pytest) and cargo
// test. root is the repository root (absolute); cwd is where argv ran and
// where the rerun will run. failed are the tests that failed, as symbols
// IsTest accepts.
//
// ok is false whenever the command can't be exact: another runner, a flag
// it doesn't know (it can't tell whether the next word is the flag's value
// or a package), a mode that doesn't just run tests (go test -fuzz, -list,
// -c; pytest --collect-only, --stepwise; cargo test --doc, --no-run), or
// failed tests the runner as invoked can't select by name.
func Rerun(root, cwd string, argv []string, failed []ref.Target) (Cmd, bool) {
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
		return cargoRerun(root, argv, failed)
	}
	return Cmd{}, false
}

// RerunNext is Rerun as a crux suggestion, with the reason worded by
// execution: "reruns the 2 failed tests".
func RerunNext(root, cwd string, argv []string, failed []ref.Target) (report.Next, bool) {
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
func all(ts []ref.Target, suffix string) bool {
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
// [-args …]. Packages and -run are replaced, -bench and -benchtime
// dropped; argv's packages only tell which module the run was in.
func goRerun(root, cwd string, argv []string, failed []ref.Target) (Cmd, bool) {
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
	head = append(head, "go")
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
	for ; i < len(argv); i++ {
		a := argv[i]
		if a == "-args" || a == "--args" {
			after = argv[i:]
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			if strings.HasSuffix(a, ".go") {
				return Cmd{}, false // files, not packages: a rerun by package builds something else
			}
			continue // a package: replaced
		}
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
	sel := goSelect(root, failed)
	args := append(append(head, flags...), "-run", sel.re)
	for _, d := range sel.dirs {
		args = append(args, rel(base, d, true))
	}
	return Cmd{Args: append(args, after...), Tests: sel.n, Caveat: sel.caveat}, true
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
)

// pytestRerun: <argv's options> <node ids>. Positional paths become the
// failed tests' node ids, relative to where pytest runs; -k, -m and the
// like stay, as the failed tests matched them.
func pytestRerun(root, cwd string, prefix, args []string, failed []ref.Target) (Cmd, bool) {
	if !all(failed, ".py") {
		return Cmd{}, false
	}
	opts := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			i = len(args) // the rest is positional: replaced
			continue
		case !strings.HasPrefix(a, "-") || a == "-":
			continue // a path or node id: replaced
		case strings.HasPrefix(a, "--"):
			name, _, hasValue := strings.Cut(a, "=")
			switch {
			case pyValue[name] && !hasValue:
				if i+1 >= len(args) {
					return Cmd{}, false
				}
				opts = append(opts, a, args[i+1])
				i++
			case pyValue[name], pyBool[name] && !hasValue:
				opts = append(opts, a)
			case pyOptional[name] && !hasValue && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-"):
				opts = append(opts, a, args[i+1])
				i++
			case pyOptional[name]:
				opts = append(opts, a)
			default:
				return Cmd{}, false
			}
		default: // short options, maybe clustered (-xvs) or with the value attached (-kfoo, -rA)
			n, ok := pyShort(a)
			if !ok {
				return Cmd{}, false
			}
			opts = append(opts, a)
			if n {
				if i+1 >= len(args) {
					return Cmd{}, false
				}
				opts = append(opts, args[i+1])
				i++
			}
		}
	}
	var ids []string
	for _, t := range failed {
		id := rel(cwd, filepath.Join(root, filepath.FromSlash(t.Path)), false) + "::" + strings.ReplaceAll(t.Name, ".", "::")
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	args = append(append(append([]string{}, prefix...), opts...), ids...)
	return Cmd{Args: args, Tests: len(ids)}, true
}

// pyShort checks a cluster of short options: each a known flag, ending at
// one that takes a value (attached, or the next word: needsNext).
func pyShort(a string) (needsNext, ok bool) {
	letters := a[1:]
	for j, c := range letters {
		opt := "-" + string(c)
		switch {
		case pyValue[opt]:
			return j == len(letters)-1, true
		case !pyBool[opt]:
			return false, false
		}
	}
	return false, true
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
// after it) become the failed tests' paths; target selection stays.
func cargoRerun(root string, argv []string, failed []ref.Target) (Cmd, bool) {
	if !all(failed, ".rs") {
		return Cmd{}, false
	}
	i := 1
	head := []string{"cargo"}
	if i < len(argv) && strings.HasPrefix(argv[i], "+") {
		head = append(head, argv[i])
		i++
	}
	if i >= len(argv) || argv[i] != "test" {
		return Cmd{}, false
	}
	head = append(head, "test")
	i++
	var opts, lib []string
	narrowed := false // target selection given: only those targets build
	for ; i < len(argv) && argv[i] != "--"; i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") {
			continue // TESTNAME: replaced
		}
		name, _, hasValue := strings.Cut(a, "=")
		switch {
		case cargoValue[name] && !hasValue:
			if i+1 >= len(argv) {
				return Cmd{}, false
			}
			opts = append(opts, a, argv[i+1])
			i++
		case cargoValue[name], cargoBool[name] && !hasValue:
			opts = append(opts, a)
		default:
			return Cmd{}, false // --doc, --no-run and anything unknown
		}
		switch name {
		case "--lib", "--test", "--bin", "--bins", "--tests", "--example", "--examples", "--bench", "--benches":
			narrowed = true
		}
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
	var filters []string
	crates := map[string]bool{}
	targets := map[string]bool{}
	for _, t := range failed {
		abs := filepath.Join(root, filepath.FromSlash(t.Path))
		crate := up(root, filepath.Dir(abs), "Cargo.toml", []byte("[package]"))
		if crate == "" {
			return Cmd{}, false
		}
		crates[crate] = true
		mod, target := rustModule(rel(crate, abs, false))
		targets[crate+"\x00"+target] = true
		f := strings.ReplaceAll(t.Name, ".", "::")
		if mod != "" {
			f = mod + "::" + f
		}
		if !slices.Contains(filters, f) {
			filters = append(filters, f)
		}
	}
	slices.Sort(filters)
	c := Cmd{Tests: len(filters)}
	// --exact matches a path in each test binary the run builds: with the
	// failed tests in several crates or targets, or a crate with a library
	// and a binary root and no target chosen, a test with the same path
	// elsewhere runs too
	shared := len(crates) > 1 || len(targets) > 1
	for crate := range crates {
		shared = shared || !narrowed && exists(filepath.Join(crate, "src", "lib.rs")) && exists(filepath.Join(crate, "src", "main.rs"))
	}
	if shared {
		c.Caveat = "also runs tests with the same path in the other targets"
	}
	args := append(append(head, opts...), "--")
	args = append(append(args, lib...), "--exact")
	c.Args = append(args, filters...)
	return c, true
}
