package testcmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Vaspyyy/prism/outline"
)

// pySetup is the rootdir and config file ("" for none) pytest settles on.
type pySetup struct{ rootdir, ini string }

// pytestSetup works out pytest's rootdir and config file for a run from
// cwd without -c, as pytest 9 does (determine_setup in
// _pytest/config/findpaths.py). args are the words its first parse takes
// for paths; rootArg is --rootdir's value, which both commands it's
// compared for share. ok is false where a candidate file can't be read or
// told apart.
//
// The config file is the first with pytest settings found from the common
// directory of the args that exist (cwd if none) up, else the first
// pyproject.toml there. Without one, the rootdir is the nearest directory
// up with a setup.py, else the args are searched one by one, else the
// rootdir is the common directory of cwd and the args'.
func pytestSetup(cwd string, args []string, rootArg string) (pySetup, bool) {
	var dirs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		p := strings.SplitN(a, "::", 2)[0]
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			p = filepath.Dir(p)
		}
		dirs = append(dirs, filepath.Clean(p))
	}
	ancestor := commonDir(cwd, dirs)
	rootdir, ini, ok := pyLocate(cwd, []string{ancestor})
	if !ok {
		return pySetup{}, false
	}
	if rootdir == "" && rootArg == "" {
		for d := ancestor; ; d = filepath.Dir(d) {
			if isFile(filepath.Join(d, "setup.py")) {
				rootdir = d
				break
			}
			if d == filepath.Dir(d) {
				break
			}
		}
		if rootdir == "" {
			if len(dirs) != 1 || dirs[0] != ancestor {
				if rootdir, ini, ok = pyLocate(cwd, dirs); !ok {
					return pySetup{}, false
				}
			}
			if rootdir == "" {
				rootdir = commonDir(cwd, []string{cwd, ancestor})
				if rootdir == filepath.Dir(rootdir) {
					rootdir = ancestor
				}
			}
		}
	}
	if rootArg != "" {
		rootdir = "--rootdir " + rootArg
	}
	return pySetup{rootdir, ini}, true
}

// commonDir is the deepest directory holding all of dirs, or def for none.
func commonDir(def string, dirs []string) string {
	c := ""
	for _, d := range dirs {
		if c == "" {
			c = d
		}
		for !within(c, d) {
			c = filepath.Dir(c)
		}
	}
	if c == "" {
		return def
	}
	return c
}

// pyConfigNames are the files pytest looks for in each directory, in order.
var pyConfigNames = []string{"pytest.toml", ".pytest.toml", "pytest.ini", ".pytest.ini", "pyproject.toml", "tox.ini", "setup.cfg"}

// pyLocate is pytest's locate_config: from each directory up, the first
// file with pytest settings, else the first pyproject.toml seen. rootdir
// is "" for none.
func pyLocate(cwd string, dirs []string) (rootdir, ini string, ok bool) {
	if len(dirs) == 0 {
		dirs = []string{cwd}
	}
	pyproject := ""
	for _, d := range dirs {
		for base := d; ; base = filepath.Dir(base) {
			for _, n := range pyConfigNames {
				p := filepath.Join(base, n)
				if !isFile(p) {
					continue
				}
				if n == "pyproject.toml" && pyproject == "" {
					pyproject = p
				}
				has, ok := pyConfig(p)
				if !ok {
					return "", "", false
				}
				if has {
					return base, p, true
				}
			}
			if base == filepath.Dir(base) {
				break
			}
		}
	}
	if pyproject != "" {
		return filepath.Dir(pyproject), pyproject, true
	}
	return "", "", true
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// pyConfig reports whether a file pytest finds holds its settings
// (load_config_dict_from_file): pytest.toml and pytest.ini always, a
// [pytest] section in tox.ini, [tool:pytest] in setup.cfg, [tool.pytest]
// or [tool.pytest.ini_options] in pyproject.toml. ok is false where it
// can't tell: a file it can't read (a symlink, say), a [pytest] section
// in setup.cfg (an error in pytest), or pyproject.toml settings written as
// dotted keys or inline tables.
func pyConfig(p string) (has, ok bool) {
	data, ok := outline.Read(p)
	if !ok {
		return false, false
	}
	switch filepath.Base(p) {
	case "pytest.toml", ".pytest.toml", "pytest.ini", ".pytest.ini":
		return true, true
	case "tox.ini":
		return iniSection(data, "pytest"), true
	case "setup.cfg":
		t := iniSection(data, "tool:pytest")
		return t, t || !iniSection(data, "pytest")
	}
	return pyprojectConfig(data)
}

// iniSection reports whether an ini file has the section, read as
// pytest's iniconfig does: a line starting with [, up to a comment.
func iniSection(data []byte, name string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "[") {
			continue
		}
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimRight(line, " \t\r")
		if len(line) >= 2 && strings.HasSuffix(line, "]") && line[1:len(line)-1] == name {
			return true
		}
	}
	return false
}

// pyprojectConfig reports whether a pyproject.toml has pytest settings: a
// [tool.pytest.ini_options] table, or a [tool.pytest] table (or subtable)
// with something in it.
func pyprojectConfig(data []byte) (has, ok bool) {
	norm := strings.NewReplacer(" ", "", "\t", "", `"`, "", "'", "")
	header, str := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if str != "" { // inside a multi-line string
			if strings.Count(l, str)%2 == 1 {
				str = ""
			}
			continue
		}
		if l == "" || l[0] == '#' {
			continue
		}
		if l[0] == '[' {
			header = strings.Trim(norm.Replace(strings.SplitN(l, "#", 2)[0]), "[]")
			if strings.HasPrefix(header, "tool.pytest.") {
				return true, true
			}
			continue
		}
		key := norm.Replace(strings.SplitN(l, "=", 2)[0])
		switch {
		case header == "tool.pytest":
			return true, true
		case header == "" && (key == "tool" || key == "tool.pytest" || strings.HasPrefix(key, "tool.pytest.")),
			header == "tool" && (key == "pytest" || strings.HasPrefix(key, "pytest.")):
			return false, false
		}
		for _, q := range []string{`"""`, "'''"} {
			if strings.Count(l, q)%2 == 1 {
				str = q
				break
			}
		}
	}
	return false, true
}
