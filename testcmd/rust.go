package testcmd

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Vaspyyy/prism/outline"
	"github.com/Vaspyyy/prism/ref"
)

// A Rust test's path (what libtest prints and --exact matches) is its
// module path in its target plus its name. Where the files tell, that's
// the file's place: src/a/b.rs holds a::b of the library or of the main
// binary (they share src/), tests/x/y.rs holds y of the test target x.

// rustTarget is a target cargo builds tests from.
type rustTarget struct{ kind, name string } // kind: lib, bin, test, example, bench

// rustCrate is what a Cargo package's files tell about its tests.
type rustCrate struct {
	dir     string
	name    string // [package] name
	main    string // the name of the binary in src/main.rs: the package's, unless a [[bin]] renames it
	custom  bool   // targets aren't cargo's discovery, or the manifest can't be read
	moved   bool   // a source moves modules away from their files' places, or can't be read
	targets []rustTarget
	flags   map[rustTarget]rustFlags
	files   map[string]rustFile // crate-relative path → file, for files that may hold wanted tests
}

// rustFlags are a target's manifest settings that change what a cargo test
// command runs: whether the default and --tests (test) or --benches
// (bench) select it, and whether libtest runs it (harness).
type rustFlags struct{ test, bench, noHarness bool }

// flag is a target's settings: cargo's defaults for its kind unless the
// manifest says otherwise.
func (c *rustCrate) flag(t rustTarget) rustFlags {
	if f, ok := c.flags[t]; ok {
		return f
	}
	return rustFlags{test: t.kind != "example" && t.kind != "bench", bench: t.kind != "example" && t.kind != "test"}
}

// rustFile is a source file's place: the targets it's built into (own,
// if known), and the test paths it holds.
type rustFile struct {
	own   []rustTarget
	known bool
	kind  string // for a shared module (tests/common/mod.rs): the kind of the targets that may include it
	paths []string
}

// rustDirs are the target kinds cargo discovers by directory.
var rustDirs = map[string]string{"tests": "test", "examples": "example", "benches": "bench"}

// rustOpaqueSrc matches what moves a module away from its file's place:
// #[path = …] (also under cfg_attr) and include!("…"), where an item can
// start: at the start of a line, or after ; { or }, maybe after other
// attributes. Not in a comment, nor after a quote on the same line.
var rustOpaqueSrc = regexp.MustCompile(`(?m)^[ \t]*(?:[^"'/\n]*[;{}][ \t]*)?(?:#!?\[[^\]\n]*\][ \t]*)*(?:#!?\[\s*(?:path\s*=|cfg_attr\s*\([^\n]*\bpath\s*=)|include\s*!\s*\(\s*r?#*")`)

// rustAttrLine is a line whose attributes share it with an item.
var rustAttrLine = regexp.MustCompile(`^([ \t]*)((?:#!?\[[^\]\n]*\][ \t]*)+)([^\s/#].*)$`)

// rustSplitAttrs puts attributes that share a line with an item
// ("#[test] fn x() {}", "#[cfg(test)] mod tests {") on a line of their
// own, as rustfmt would, so the outline lists the item.
func rustSplitAttrs(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if m := rustAttrLine.FindStringSubmatch(l); m != nil {
			out = append(out, m[1]+strings.TrimSpace(m[2]), m[1]+m[3])
		} else {
			out = append(out, l)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// loadCrate reads a package: its manifest, its targets as cargo discovers
// them, and its sources. Only files mentioning one of wanted (test names'
// last segments) are parsed for test paths.
func loadCrate(dir string, wanted []string) *rustCrate {
	c := &rustCrate{dir: dir, files: map[string]rustFile{}}
	manifest, ok := outline.Read(filepath.Join(dir, "Cargo.toml"))
	if !ok {
		c.custom = true
		return c
	}
	var sections []cargoSection
	c.name, sections, c.custom = cargoManifest(manifest)
	c.main = c.name
	for _, s := range sections {
		if s.kind == "bin" && path.Clean(s.path) == "src/main.rs" {
			c.main = s.name
		}
	}
	if exists(filepath.Join(dir, "src", "lib.rs")) {
		c.targets = append(c.targets, rustTarget{"lib", c.name})
	}
	if exists(filepath.Join(dir, "src", "main.rs")) {
		c.targets = append(c.targets, rustTarget{"bin", c.main})
	}
	for _, d := range [][2]string{{"src/bin", "bin"}, {"tests", "test"}, {"examples", "example"}, {"benches", "bench"}} {
		entries, _ := os.ReadDir(filepath.Join(dir, filepath.FromSlash(d[0])))
		for _, e := range entries {
			n := e.Name()
			switch {
			case !e.IsDir() && strings.HasSuffix(n, ".rs"):
				c.targets = append(c.targets, rustTarget{d[1], strings.TrimSuffix(n, ".rs")})
			case e.IsDir() && exists(filepath.Join(dir, filepath.FromSlash(d[0]), n, "main.rs")):
				c.targets = append(c.targets, rustTarget{d[1], n})
			}
		}
	}
	c.flags = map[rustTarget]rustFlags{}
	for _, s := range sections {
		t, ok := c.declared(s)
		if !ok {
			c.custom = true
			continue
		}
		f := c.flag(t)
		f.test = s.test.or(f.test)
		f.bench = s.bench.or(f.bench)
		f.noHarness = !s.harness.or(true)
		c.flags[t] = f
	}
	for _, top := range []string{"src", "tests", "examples", "benches"} {
		filepath.WalkDir(filepath.Join(dir, top), func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				if p != filepath.Join(dir, top) && (strings.HasPrefix(e.Name(), ".") || exists(filepath.Join(p, "Cargo.toml"))) {
					return filepath.SkipDir // another package, or not sources
				}
				return nil
			}
			if e.Type()&fs.ModeSymlink != 0 {
				c.moved = true // sources elsewhere: not walked, nor read
				return nil
			}
			if !strings.HasSuffix(p, ".rs") {
				return nil
			}
			data, ok := outline.Read(p)
			if !ok || rustOpaqueSrc.Match(data) {
				c.moved = true
				return nil
			}
			if !slices.ContainsFunc(wanted, func(w string) bool { return bytes.Contains(data, []byte(w)) }) {
				return nil
			}
			r := rel(dir, p, false)
			mod, own, known := c.place(r)
			f := rustFile{own: own, known: known}
			if !known && strings.HasPrefix(r, "src/bin/") {
				f.kind = "bin"
			} else if !known {
				f.kind = rustDirs[strings.SplitN(r, "/", 2)[0]]
			}
			data = rustSplitAttrs(data)
			syms := outline.Parse(filepath.Base(p), data, outline.MaxSize)
			for _, t := range ref.Targets(r, syms) {
				if IsTest(t, data) {
					f.paths = append(f.paths, rustJoin(mod, t.Name))
				}
			}
			c.files[r] = f
			return nil
		})
	}
	return c
}

// cargoSection is a target the manifest sets up: [lib], [[bin]], [[test]],
// [[example]] or [[bench]].
type cargoSection struct {
	kind, name, path     string
	test, bench, harness tomlBool
}

// tomlBool is a manifest boolean: unset, or true or false.
type tomlBool string

func (b tomlBool) or(def bool) bool {
	if b == "" {
		return def
	}
	return b == "true"
}

// cargoManifest reads a Cargo.toml for the package name and the targets
// it sets up. custom is true where targets aren't cargo's discovery as
// read here: an inline target array, an auto* key, a target key it
// doesn't know the effect of, or no name.
func cargoManifest(data []byte) (name string, sections []cargoSection, custom bool) {
	section := ""
	kinds := map[string]string{"[lib]": "lib", "[[bin]]": "bin", "[[test]]": "test", "[[example]]": "example", "[[bench]]": "bench"}
	// keys that don't change which tests a command runs, or where
	inert := []string{"crate-type", "proc-macro", "doctest", "doc", "edition", "plugin", "required-features", "filename"}
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || l[0] == '#' {
			continue
		}
		if l[0] == '[' {
			section = strings.Join(strings.Fields(strings.SplitN(l, "#", 2)[0]), "")
			if kinds[section] != "" {
				sections = append(sections, cargoSection{kind: kinds[section]})
			}
			continue
		}
		key, value, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		value = strings.Trim(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]), `"'`)
		switch kind := kinds[section]; {
		case section == "":
			custom = custom || slices.Contains([]string{"lib", "bin", "test", "example", "bench"}, strings.SplitN(key, ".", 2)[0])
		case section == "[package]":
			if key == "name" {
				name = value
			}
			custom = custom || strings.HasPrefix(key, "auto")
		case kind != "":
			s := &sections[len(sections)-1]
			switch key {
			case "name":
				s.name = value
			case "path":
				s.path = value
			case "test":
				s.test = tomlBool(value)
			case "bench":
				s.bench = tomlBool(value)
			case "harness":
				s.harness = tomlBool(value)
			default:
				custom = custom || !slices.Contains(inert, key)
			}
		}
	}
	return name, sections, custom || name == ""
}

// declared is the discovered target a manifest section sets up. ok is
// false for one whose path isn't where cargo would look for it (a [[bin]]
// may name src/main.rs whatever its name).
func (c *rustCrate) declared(s cargoSection) (rustTarget, bool) {
	if s.kind == "lib" {
		return rustTarget{"lib", c.name}, s.path == "" || path.Clean(s.path) == "src/lib.rs"
	}
	if s.name == "" {
		return rustTarget{}, false
	}
	t := rustTarget{s.kind, s.name}
	if s.path == "" {
		return t, true
	}
	p := path.Clean(s.path)
	if s.kind == "bin" && p == "src/main.rs" {
		return t, true
	}
	dir := map[string]string{"bin": "src/bin", "test": "tests", "example": "examples", "bench": "benches"}[s.kind]
	return t, p == dir+"/"+s.name+".rs" || p == dir+"/"+s.name+"/main.rs"
}

// opaque reports whether test paths or targets don't follow from the
// files: a module moved by #[path] or include!, a target with its own
// path, harness, test or required-features setting, discovery turned off,
// or a manifest or source it can't read.
func (c *rustCrate) opaque() bool { return c.custom || c.moved }

// place is where a crate-relative .rs file sits: its module path by
// file, and the targets it's built into. known is false for a module
// directory no target owns (tests/common/mod.rs: which targets include it,
// and where, the files don't tell) and for files outside the target
// directories.
func (c *rustCrate) place(r string) (mod string, own []rustTarget, known bool) {
	parts := strings.Split(strings.TrimSuffix(r, ".rs"), "/")
	modOf := func(ps []string) string {
		if n := len(ps); n > 0 && ps[n-1] == "mod" {
			ps = ps[:n-1]
		}
		return strings.Join(ps, "::")
	}
	var kind, dir string
	var sub []string
	switch {
	case len(parts) >= 3 && parts[0] == "src" && parts[1] == "bin":
		kind, dir, sub = "bin", "src/bin", parts[2:]
	case len(parts) >= 2 && parts[0] == "src":
		rest := parts[1:]
		switch {
		case len(rest) == 1 && rest[0] == "lib":
			return "", []rustTarget{{"lib", c.name}}, true
		case len(rest) == 1 && rest[0] == "main":
			return "", []rustTarget{{"bin", c.main}}, true
		}
		for _, t := range c.targets {
			if t.kind == "lib" || t == (rustTarget{"bin", c.main}) && exists(filepath.Join(c.dir, "src", "main.rs")) {
				own = append(own, t)
			}
		}
		return modOf(rest), own, len(own) > 0
	case len(parts) >= 2 && rustDirs[parts[0]] != "":
		kind, dir, sub = rustDirs[parts[0]], parts[0], parts[1:]
	default:
		return "", nil, false
	}
	t := []rustTarget{{kind, sub[0]}}
	switch {
	case len(sub) == 1:
		return "", t, true // the target's root, x.rs
	case len(sub) == 2 && sub[1] == "main" && exists(filepath.Join(c.dir, filepath.FromSlash(dir), sub[0], "main.rs")):
		return "", t, true // x/main.rs
	case exists(filepath.Join(c.dir, filepath.FromSlash(dir), sub[0], "main.rs")) || exists(filepath.Join(c.dir, filepath.FromSlash(dir), sub[0]+".rs")):
		return modOf(sub[1:]), t, true // a module of x
	}
	return modOf(sub), nil, false
}

// has reports whether a target may hold a test with the path.
func (c *rustCrate) has(t rustTarget, p string) bool {
	for _, f := range c.files {
		in := f.known && slices.Contains(f.own, t) || !f.known && f.kind == t.kind
		if in && slices.Contains(f.paths, p) {
			return true
		}
	}
	return false
}

// rustJoin is a test's path: the module path, then the qualified name.
func rustJoin(mod, name string) string {
	q := strings.ReplaceAll(name, ".", "::")
	if mod == "" {
		return q
	}
	return mod + "::" + q
}

// lastSegments are the qualified names' last parts: a cheap test for
// which files can hold the tests.
func lastSegments(ts []ref.Target) []string {
	var out []string
	for _, t := range ts {
		n := t.Name[strings.LastIndex(t.Name, ".")+1:]
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// cargoSel is the targets a cargo test command selects in each package
// it builds: by default those with test = true (the library, binaries and
// integration tests, unless the manifest says otherwise).
type cargoSel struct {
	given bool            // a target option was given: only what they name
	opts  map[string]bool // --lib, --bins, --tests (test = true), --benches (bench = true), --examples
	named map[string][]string
}

func (s *cargoSel) add(opt, value string) {
	if s.opts == nil {
		s.opts, s.named = map[string]bool{}, map[string][]string{}
	}
	single := map[string]string{"--bin": "bin", "--test": "test", "--example": "example", "--bench": "bench"}
	switch opt {
	case "--lib", "--bins", "--tests", "--benches", "--examples":
		s.given, s.opts[opt] = true, true
	case "--all-targets":
		s.given = true
		for _, o := range []string{"--lib", "--bins", "--tests", "--benches", "--examples"} {
			s.opts[o] = true
		}
	case "--bin", "--test", "--example", "--bench":
		s.given = true
		s.named[single[opt]] = append(s.named[single[opt]], value)
	}
}

func (s cargoSel) selects(c *rustCrate, t rustTarget) bool {
	f := c.flag(t)
	if !s.given {
		return f.test
	}
	if s.opts["--lib"] && t.kind == "lib" || s.opts["--bins"] && t.kind == "bin" || s.opts["--examples"] && t.kind == "example" ||
		s.opts["--tests"] && f.test || s.opts["--benches"] && f.bench {
		return true
	}
	for _, n := range s.named[t.kind] {
		if m, err := path.Match(n, t.name); n == t.name || err == nil && m {
			return true
		}
	}
	return false
}

// rustEntry is a test a cargo command is to run: its crate, the targets
// its file is built into (known), and its path.
type rustEntry struct {
	crate *rustCrate
	own   []rustTarget
	known bool
	file  string // repo-relative
	path  string
}

// rustCaveat works out whether a cargo test command with these packages
// in scope and these targets selected runs more than the entries' tests:
// --exact matches a path in each test binary it builds. ok is false when
// it doesn't build an entry's target at all.
//
// A path can repeat in the library and the main binary (they share
// src/, and may both include a file), and in any other selected target
// or package that holds a test with that path. A package whose sources
// move modules may hold any path in its targets, one with custom targets
// anything anywhere, and a target without libtest decides itself what
// runs; an entry whose target isn't known may be in any.
func rustCaveat(scope []*rustCrate, sel cargoSel, es []rustEntry) (caveat, ok bool) {
	selected := 0
	for _, c := range scope {
		caveat = caveat || c.custom
		for _, t := range c.targets {
			if sel.selects(c, t) {
				selected++
				caveat = caveat || c.flag(t).noHarness
			}
		}
	}
	for _, e := range es {
		if !slices.Contains(scope, e.crate) {
			return false, false
		}
		if !e.known {
			caveat = caveat || selected > 1
			continue
		}
		own := 0
		for _, t := range e.own {
			if sel.selects(e.crate, t) {
				own++
			}
		}
		if own == 0 {
			return false, false
		}
		caveat = caveat || own > 1
		for _, c := range scope {
			for _, t := range c.targets {
				if sel.selects(c, t) && !rustRerun(es, c, t, e.path) && (c.moved || c.has(t, e.path)) {
					caveat = true
				}
			}
		}
	}
	return caveat, true
}

// rustRerun reports whether the path in that target is one of the entries.
func rustRerun(es []rustEntry, c *rustCrate, t rustTarget, p string) bool {
	return slices.ContainsFunc(es, func(e rustEntry) bool {
		return e.known && e.crate == c && e.path == p && slices.Contains(e.own, t)
	})
}

// cargoScope lists the packages a cargo test command builds, from where
// it runs: the package of its manifest (--manifest-path, else the nearest
// Cargo.toml up from cwd), or with -p, --workspace, a virtual manifest or
// default-members, every package under the workspace root but those
// --exclude names (more than it may build: members, globs in -p and
// default-members aren't read). With one -p name, the loaded package of
// that name. load reads a package once.
func cargoScope(root, cwd string, pkgs, exclude []string, workspace bool, manifest string, load func(string) *rustCrate) ([]*rustCrate, bool) {
	dir := ""
	if manifest != "" {
		if !filepath.IsAbs(manifest) {
			manifest = filepath.Join(cwd, manifest)
		}
		dir = filepath.Dir(manifest)
	} else {
		dir = up(root, cwd, "Cargo.toml", nil)
	}
	data, ok := outline.Read(filepath.Join(dir, "Cargo.toml"))
	if dir == "" || !ok {
		return nil, false
	}
	plain := func(p string) bool {
		return strings.Trim(p, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") == ""
	}
	switch {
	case len(pkgs) == 1 && !workspace && plain(pkgs[0]):
		ws := up(root, dir, "Cargo.toml", []byte("[workspace]"))
		if ws == "" {
			ws = dir
		}
		for _, d := range cargoPackages(ws) {
			if m, ok := outline.Read(filepath.Join(d, "Cargo.toml")); ok {
				if name, _, _ := cargoManifest(m); name == pkgs[0] {
					return []*rustCrate{load(d)}, true
				}
			}
		}
		return nil, false
	case len(pkgs) == 0 && !workspace && bytes.Contains(data, []byte("[package]")) && !bytes.Contains(data, []byte("default-members")):
		return []*rustCrate{load(dir)}, true
	}
	ws := dir
	if !bytes.Contains(data, []byte("[workspace]")) {
		if w := up(root, dir, "Cargo.toml", []byte("[workspace]")); w != "" {
			ws = w
		}
	}
	var out []*rustCrate
	for _, d := range cargoPackages(ws) {
		c := load(d)
		if !slices.ContainsFunc(exclude, func(x string) bool { m, err := path.Match(x, c.name); return x == c.name || err == nil && m }) {
			out = append(out, c)
		}
	}
	return out, len(out) > 0
}

// cargoPackages lists the directories under dir with a Cargo.toml for a
// package, leaving out target/ and hidden directories.
func cargoPackages(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if e.IsDir() {
			if p != dir && (e.Name() == "target" || strings.HasPrefix(e.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Name() == "Cargo.toml" {
			if data, ok := outline.Read(p); ok && bytes.Contains(data, []byte("[package]")) {
				out = append(out, filepath.Dir(p))
			}
		}
		return nil
	})
	return out
}
