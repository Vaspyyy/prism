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
	name    string // [package] name, the main binary's name
	custom  bool   // targets aren't (only) cargo's discovery, or the manifest can't be read
	moved   bool   // a source moves modules away from their files' places, or can't be read
	targets []rustTarget
	files   map[string]rustFile // crate-relative path → file, for files that may hold wanted tests
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
// #[path = …] (also under cfg_attr) and include!("…").
var rustOpaqueSrc = regexp.MustCompile(`#!?\[\s*(?:path\s*=|cfg_attr\s*\([^\n]*\bpath\s*=)|\binclude\s*!\s*\(\s*r?#*"`)

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
	c.name, c.custom = cargoManifest(manifest)
	if exists(filepath.Join(dir, "src", "lib.rs")) {
		c.targets = append(c.targets, rustTarget{"lib", c.name})
	}
	if exists(filepath.Join(dir, "src", "main.rs")) {
		c.targets = append(c.targets, rustTarget{"bin", c.name})
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

// cargoManifest reads a Cargo.toml for the package name, and whether a
// target is set up other than by discovery: a [lib], [[bin]], [[test]],
// [[example]] or [[bench]] with a path, harness, test or
// required-features key, an inline target array, or an auto* key.
func cargoManifest(data []byte) (name string, custom bool) {
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || l[0] == '#' {
			continue
		}
		if l[0] == '[' {
			section = strings.Join(strings.Fields(strings.SplitN(l, "#", 2)[0]), "")
			continue
		}
		key, value, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"'`)
		switch section {
		case "":
			custom = custom || slices.Contains([]string{"lib", "bin", "test", "example", "bench"}, strings.SplitN(key, ".", 2)[0])
		case "[package]":
			if key == "name" {
				name = strings.Trim(strings.TrimSpace(strings.SplitN(value, "#", 2)[0]), `"'`)
			}
			custom = custom || strings.HasPrefix(key, "auto")
		case "[lib]", "[[bin]]", "[[test]]", "[[example]]", "[[bench]]":
			custom = custom || slices.Contains([]string{"path", "harness", "test", "required-features"}, key)
		}
	}
	return name, custom || name == ""
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
			return "", []rustTarget{{"bin", c.name}}, true
		}
		for _, t := range c.targets {
			if t.kind == "lib" || t == (rustTarget{"bin", c.name}) {
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
// it builds: by default the library, binaries and integration tests.
type cargoSel struct {
	given bool                // a target option was given: only what they name
	kinds map[string]bool     // whole kinds: --lib, --bins, --tests …
	named map[string][]string // --bin x, --test x … (names or globs)
}

func (s *cargoSel) add(opt, value string) {
	if s.kinds == nil {
		s.kinds, s.named = map[string]bool{}, map[string][]string{}
	}
	kinds := map[string][]string{
		"--lib": {"lib"}, "--bins": {"bin"}, "--examples": {"example"}, "--benches": {"bench"},
		"--tests":       {"lib", "bin", "test"}, // targets with test = true; others make the crate opaque
		"--all-targets": {"lib", "bin", "test", "example", "bench"},
	}
	single := map[string]string{"--bin": "bin", "--test": "test", "--example": "example", "--bench": "bench"}
	switch {
	case kinds[opt] != nil:
		s.given = true
		for _, k := range kinds[opt] {
			s.kinds[k] = true
		}
	case single[opt] != "":
		s.given = true
		s.named[single[opt]] = append(s.named[single[opt]], value)
	}
}

func (s cargoSel) selects(t rustTarget) bool {
	if !s.given {
		return t.kind == "lib" || t.kind == "bin" || t.kind == "test"
	}
	if s.kinds[t.kind] {
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
// anything anywhere; an entry whose target isn't known may be in any.
func rustCaveat(scope []*rustCrate, sel cargoSel, es []rustEntry) (caveat, ok bool) {
	selected := 0
	for _, c := range scope {
		caveat = caveat || c.custom
		for _, t := range c.targets {
			if sel.selects(t) {
				selected++
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
			if sel.selects(t) {
				own++
			}
		}
		if own == 0 {
			return false, false
		}
		caveat = caveat || own > 1
		for _, c := range scope {
			for _, t := range c.targets {
				if sel.selects(t) && !rustRerun(es, c, t, e.path) && (c.moved || c.has(t, e.path)) {
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
// default-members, every package under the workspace root (more than it
// may build: globs, default-members and --exclude aren't read). With one
// -p name, the loaded package of that name. load reads a package once.
func cargoScope(root, cwd string, pkgs []string, workspace bool, manifest string, load func(string) *rustCrate) ([]*rustCrate, bool) {
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
				if name, _ := cargoManifest(m); name == pkgs[0] {
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
		out = append(out, load(d))
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
