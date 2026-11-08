// Package outline finds the definitions in a source file (functions, types,
// classes, Markdown headings) with cheap per-language rules, and where each
// one ends. It is not a parser: it is enough to tell where things live and
// which definition a changed line belongs to, and fast enough to run on
// every file of a repository. A line inside a string or comment that spans
// lines (see Multiline) defines nothing.
//
// testdata/corpus has a file in every language outline supports, with
// docstrings, template and raw strings, heredocs and block comments, and
// testdata/corpus.golden lists every symbol Parse finds there. A change to
// the rules shows up as a diff of that file: review it, then rewrite it
// with `go test ./outline -update`.
package outline

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"regexp/syntax"
	"strings"
)

// Symbol is one definition.
type Symbol struct {
	Line   int    // first line, 1-based
	End    int    // last line (inclusive); Line if the end isn't known
	Kind   string // "func", "type", "class", "h2", …
	Name   string
	Sig    string // the defining line, trimmed: name plus signature
	Parent int    // index of the enclosing symbol, or -1
}

type rule struct {
	re      *regexp.Regexp
	kind    string // "" means: take the kind from capture group 1
	qualify bool   // group 1 (if matched) qualifies the name: a Go method's receiver type
	need    []string
}

func r(kind, pattern string) rule { return rule{re: regexp.MustCompile(pattern), kind: kind} }

// prepare works out the literals a rule's matches contain (one of them),
// so that lines without any skip the regex.
func prepare(rules []rule) []rule {
	for i := range rules {
		if sre, err := syntax.Parse(rules[i].re.String(), syntax.Perl); err == nil {
			rules[i].need = required(sre.Simplify())
		}
	}
	return rules
}

// required returns literals one of which every match of re contains, or
// nil if there is no such set.
func required(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			return nil
		}
		return []string{string(re.Rune)}
	case syntax.OpCapture, syntax.OpPlus:
		return required(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min >= 1 {
			return required(re.Sub[0])
		}
	case syntax.OpConcat:
		var best []string
		for _, s := range re.Sub {
			if r := required(s); r != nil && (best == nil || shortest(r) > shortest(best)) {
				best = r
			}
		}
		return best
	case syntax.OpAlternate:
		var all []string
		for _, s := range re.Sub {
			r := required(s)
			if r == nil {
				return nil
			}
			all = append(all, r...)
		}
		return all
	}
	return nil
}

func shortest(lits []string) int {
	n := -1
	for _, l := range lits {
		if n < 0 || len(l) < n {
			n = len(l)
		}
	}
	return n
}

func (ru *rule) mayMatch(line string) bool {
	if ru.need == nil {
		return true
	}
	for _, l := range ru.need {
		if strings.Contains(line, l) {
			return true
		}
	}
	return false
}

// how a language marks where a definition ends
type style int

const (
	braces   style = iota // { … }
	indent                // the next line indented as much or less (Python, GDScript)
	keyword               // indentation, plus a closing "end" line (Ruby, Lua)
	headings              // the next heading of the same or a higher level
)

type lang struct {
	rules []rule
	style style
}

var (
	goRules = prepare([]rule{
		{re: regexp.MustCompile(`^func (?:\(\s*(?:\w+\s+)?\*?(\w+)(?:\[[^\]]*\])?\s*\) )?(\w+)`), kind: "func", qualify: true},
		r("", `^(type) (\w+)`),
	})
	rustRules = prepare([]rule{
		r("", `^\s*(?:pub(?:\([^)]*\))? )?(?:async )?(?:unsafe )?(?:const )?(fn|struct|enum|trait|mod|type|union|static|macro_rules!) (\w+)`),
		// also impls nested in mod tests or a function; tuple types in the generics
		r("impl", `^\s*impl(?:<[^>]*>)? ([\w:<>, '&()\[\]]+?)(?: \{|$| where)`),
	})
	pyRules = prepare([]rule{
		r("", `^\s*(?:async )?(def|class) ([\p{L}_][\p{L}\p{N}_]*)`), // also nested helpers and classes
	})
	jsRules = prepare([]rule{
		// also nested declarations (helpers inside a factory function)
		r("function", `^\s*(?:export |return )?(?:default )?(?:async )?function\*? ?(\w+)`),
		r("", `^(?:export )?(?:default )?(?:abstract )?(class|interface|enum|type) (\w+)`),
		r("const", `^\s*(?:export )?(?:const|let|var) (\w+)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:\([^)]*\)|\w+)\s*(?::[^=]+)?=>`),
		r("method", `^(?:  |    |\t)(?:public |private |protected |static |async |readonly |override |get |set )*\*?(\w+)\s*\([^)]*\)\s*(?::[^{]+)?\{\s*$`),
		// a function assigned to a property (worker.onmessage = (e) => {…}), and
		// an arrow function with a block body as an object property (set: (v) => {)
		r("function", `^\s*((?:[\w$]+\.)+[\w$]+)\s*=\s*(?:async\s+)?(?:function\b|\([^)]*\)\s*=>|[\w$]+\s*=>)`),
		r("method", `^\s+([\w$]+)\s*:\s*(?:async\s+)?(?:function\s*\([^)]*\)|\([^)]*\)\s*=>|[\w$]+\s*=>)\s*\{\s*$`),
	})
	cRules = prepare([]rule{
		r("", `^(?:typedef )?(struct|enum|union|class|namespace) (\w+)`),
		r("define", `^#define (\w+)`),
		// definitions start in column 0: a return type, the name, a parameter
		// list, then either the end of the line or the opening brace
		r("func", `^(?:[A-Za-z_][\w:<>,]*[\s*&]+)+([A-Za-z_][\w:~]*)\s*\([^;{}]*\)\s*(?:const\s*)?(?:\{.*)?$`),
		r("func", `^(?:[A-Za-z_][\w:<>,]*[\s*&]+)+([A-Za-z_][\w:~]*)\s*\([^;{})]*,\s*$`),
	})
	javaRules = prepare([]rule{
		r("", `^\s{0,4}(?:public |private |protected |internal |abstract |final |sealed |data |open |static )*(class|interface|enum|record|object) (\w+)`),
		r("fun", `^\s{0,4}(?:public |private |protected |internal |override |suspend |inline |open )*fun (?:<[^>]*> )?(?:\w+\.)?(\w+)`),
		r("method", `^    (?:public |private |protected |static |final |synchronized |abstract )+[\w<>\[\], ]+ (\w+)\s*\(`),
	})
	gdRules = prepare([]rule{
		r("", `^(func|class_name|signal|class|enum) (\w+)`),
	})
	shRules = prepare([]rule{
		r("function", `^(?:function )?([\w-]+)\s*\(\)\s*\{?`),
		r("function", `^function ([\w-]+)`),
	})
	luaRules = prepare([]rule{r("function", `^(?:local )?function ([\w.:]+)`)})
	rbRules  = prepare([]rule{r("", `^\s{0,2}(def|class|module) ([\w.?!:]+)`)})
	mdRules  = prepare([]rule{r("", `^(#{1,3}) (.+)`)})
)

var (
	goLang   = lang{rules: goRules}
	rustLang = lang{rules: rustRules}
	pyLang   = lang{rules: pyRules, style: indent}
	jsLang   = lang{rules: jsRules}
	cLang    = lang{rules: cRules}
	javaLang = lang{rules: javaRules}
	gdLang   = lang{rules: gdRules, style: indent}
	shLang   = lang{rules: shRules}
	luaLang  = lang{rules: luaRules, style: keyword}
	rbLang   = lang{rules: rbRules, style: keyword}
	mdLang   = lang{rules: mdRules, style: headings}
)

var langByExt = map[string]*lang{
	"go": &goLang, "rs": &rustLang, "py": &pyLang, "pyi": &pyLang,
	"js": &jsLang, "mjs": &jsLang, "cjs": &jsLang, "ts": &jsLang, "tsx": &jsLang, "jsx": &jsLang, "mts": &jsLang,
	"svelte": &jsLang, "vue": &jsLang,
	"c": &cLang, "h": &cLang, "cc": &cLang, "cpp": &cLang, "cxx": &cLang, "hpp": &cLang, "hh": &cLang,
	"java": &javaLang, "kt": &javaLang, "kts": &javaLang, "cs": &javaLang, "scala": &javaLang,
	"gd": &gdLang, "sh": &shLang, "bash": &shLang, "zsh": &shLang,
	"lua": &luaLang, "rb": &rbLang, "md": &mdLang, "markdown": &mdLang,
}

func langOf(name string) *lang { return langByExt[ext(name)] }

// Supported reports whether the file type has outline rules.
func Supported(name string) bool { return langOf(name) != nil }

// MaxSize is the largest file Parse looks at.
const MaxSize = 4 << 20

// Read reads a file for Parse: a regular file of at most MaxSize bytes. A
// symlink, device, pipe or bigger file gives ok false and isn't read, so a
// working-tree path that is now a symlink to /dev/zero or to a file outside
// the repository can't hang a tool or leak into its output. (git stores a
// symlink as its target's path, so its outline would be the wrong file's
// anyway.)
func Read(path string) (data []byte, ok bool) {
	if st, err := os.Lstat(path); err != nil || !st.Mode().IsRegular() || st.Size() > MaxSize {
		return nil, false
	}
	f, err := openFile(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	// the path may have been swapped since Lstat: what was opened must be
	// a regular file too, and reading stops at the limit
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil, false
	}
	data, err = io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil || len(data) > MaxSize {
		return nil, false
	}
	return data, true
}

// Parse returns the symbols of a file's contents (at most limit), with
// their ends and parents. name only selects the language. A definition
// whose line starts inside a multi-line string or comment (a docstring, a
// template string, a block comment: the lines Multiline labels) isn't one,
// so every tool sees the same symbols and ordinals.
func Parse(name string, data []byte, limit int) []Symbol {
	lg := langOf(name)
	if lg == nil || len(data) > MaxSize || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
		return nil
	}
	lines := splitLines(data)
	md := lg.style == headings
	// Strings and comments that span lines define nothing: a definition's
	// line must start in code. text marks the lines that don't, for the
	// ends of indented blocks.
	sy := syntaxByExt[ext(name)]
	var lx *lexer
	if sy != nil {
		lx = &lexer{sy: sy, lines: lines}
	}
	strip := sy != nil && sy.line != "#" // a rule may match a line with a # comment, as before
	needText := lx != nil && (lg.style == indent || lg.style == keyword)
	var text []bool
	var out []Symbol
	inFence := false
	for i, line := range lines {
		full := len(out) >= limit
		if full && !needText {
			break
		}
		if md && strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if inFence {
			continue
		}
		if lx != nil {
			inText := lx.in != code
			lx.scan(i)
			if inText {
				if text == nil {
					text = make([]bool, len(lines))
				}
				text[i] = true
				continue
			}
		}
		if full {
			continue // past the limit only the text lines are still needed, for indentEnd
		}
		if strip {
			line = stripComments(line, lx.segs)
			if strings.TrimSpace(line) == "" {
				continue
			}
		}
		for _, ru := range lg.rules {
			if !ru.mayMatch(line) {
				continue
			}
			m := ru.re.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			kind, name := ru.kind, m[len(m)-1]
			if kind == "" && len(m) >= 3 {
				kind = m[1]
			}
			if ru.qualify && m[1] != "" {
				name = m[1] + "." + name
			}
			if md {
				kind = "h" + string(rune('0'+len(m[1])))
			}
			if isNoise(name) {
				break
			}
			sig := strings.TrimSpace(line)
			if i := strings.IndexByte(sig, '{'); i > 0 && !md {
				sig = strings.TrimSpace(sig[:i]) // signature only, not a one-line body
			}
			if r := []rune(sig); len(r) > 110 {
				sig = string(r[:109]) + "…"
			}
			out = append(out, Symbol{Line: i + 1, End: i + 1, Kind: kind, Name: strings.TrimSpace(name), Sig: sig, Parent: -1})
			break
		}
	}
	findEnds(lg, sy, lines, text, out)
	findParents(out)
	return out
}

// stripComments returns the line without its comments, keeping strings
// as they are ("/*" in a string isn't a comment).
func stripComments(line string, segs []seg) string {
	n := 0
	for _, sg := range segs {
		if sg.kind == comment {
			n++
		}
	}
	switch {
	case n == 0:
		return line
	case n == 1 && segs[len(segs)-1].kind == comment:
		return line[:segs[len(segs)-1].from]
	}
	var b strings.Builder
	for _, sg := range segs {
		if sg.kind != comment {
			b.WriteString(line[sg.from:sg.to])
		}
	}
	return b.String()
}

// splitLines splits data into lines without their "\n" or "\r\n".
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	text := string(data)
	lines := make([]string, 0, strings.Count(text, "\n")+1)
	for len(text) > 0 {
		line := text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			text = ""
		}
		lines = append(lines, strings.TrimSuffix(line, "\r"))
	}
	return lines
}

func findEnds(lg *lang, sy *textSyntax, lines []string, text []bool, syms []Symbol) {
	for k := range syms {
		s := &syms[k]
		switch lg.style {
		case braces:
			// stop looking for the opening brace at the next definition
			limit := len(lines)
			if k+1 < len(syms) {
				limit = syms[k+1].Line - 1
			}
			s.End = braceEnd(lines, s.Line-1, limit, sy, s.Kind == "define")
		case indent, keyword:
			s.End = indentEnd(lines, text, s.Line-1, lg.style == keyword)
		case headings:
			s.End = len(lines)
			for j := k + 1; j < len(syms); j++ {
				if syms[j].Kind <= s.Kind { // "h1" < "h2": same or higher level
					s.End = syms[j].Line - 1
					break
				}
			}
		}
		if s.End < s.Line {
			s.End = s.Line
		}
	}
}

// braceEnd returns the 1-based line of the brace that closes the block
// opened on or after line start (0-based). A declaration without a block
// (`type ID int`, `fn f();`) ends on its own line.
func braceEnd(lines []string, start, limit int, sy *textSyntax, define bool) int {
	if define { // #define with continuation lines
		j := start
		for j+1 < len(lines) && strings.HasSuffix(strings.TrimRight(lines[j], " \t"), "\\") {
			j++
		}
		return j + 1
	}
	if sy == nil {
		sy = noText
	}
	lx := lexer{sy: sy, lines: lines} // the definition's line starts in code
	depth, opened, parens := 0, false, 0
	for j := start; j < len(lines); j++ {
		if !opened && j >= limit && j > start {
			return start + 1
		}
		lx.scan(j)
		line := lines[j]
		for _, sg := range lx.segs {
			if sg.kind != code {
				continue // strings (with the ${…} in a template string) and comments
			}
			for k := sg.from; k < sg.to; k++ {
				c := line[k]
				if !opened && parens > 0 && c != '(' && c != ')' {
					continue // braces in the parameters ({ a, b } = opts, struct{ x int }) aren't the body
				}
				switch c {
				case '(':
					if !opened {
						parens++
					}
				case ')':
					if !opened {
						parens--
					}
				case '{':
					depth++
					opened = true
				case '}':
					depth--
					if opened && depth == 0 {
						return j + 1
					}
				case ';':
					if !opened && depth == 0 {
						return j + 1
					}
				}
			}
		}
		if !opened && j-start > 30 {
			return start + 1
		}
	}
	if opened {
		return len(lines)
	}
	return start + 1
}

// indentEnd: the block ends before the next non-blank line that is
// indented as much as the definition or less (decorators and comments at
// that level belong to what follows). With endKeyword, a closing "end"
// line at the definition's level is part of it. A line inside a string or
// comment that spans lines (text) belongs to the block it is in, however
// it is indented.
func indentEnd(lines []string, text []bool, start int, endKeyword bool) int {
	base := indentOf(lines[start])
	last := start
	open := strings.Count(lines[start], "(") - strings.Count(lines[start], ")")
	for j := start + 1; j < len(lines); j++ {
		if text != nil && text[j] {
			last = j
			continue
		}
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		if open > 0 { // a signature spread over several lines
			open += strings.Count(lines[j], "(") - strings.Count(lines[j], ")")
			last = j
			continue
		}
		if indentOf(lines[j]) <= base {
			if endKeyword && (t == "end" || strings.HasPrefix(t, "end ") || strings.HasPrefix(t, "end)") || strings.HasPrefix(t, "end,")) {
				return j + 1
			}
			break
		}
		last = j
	}
	return last + 1
}

func indentOf(l string) int {
	n := 0
	for _, c := range l {
		switch c {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// findParents sets each symbol's Parent to the innermost symbol whose
// lines contain it.
func findParents(syms []Symbol) {
	for i := range syms {
		for j := i - 1; j >= 0; j-- {
			if syms[j].Line < syms[i].Line && syms[j].End >= syms[i].End && syms[j].End > syms[j].Line {
				syms[i].Parent = j
				break
			}
		}
	}
}

// Qualified is the symbol's name with its parents' names: "Repo.Open".
func Qualified(syms []Symbol, i int) string {
	name := syms[i].Name
	for p := syms[i].Parent; p >= 0; p = syms[p].Parent {
		if syms[p].Kind == "h1" || syms[p].Kind == "h2" || syms[p].Kind == "h3" {
			break // headings nest, but "Install.Linux" isn't a name anyone uses
		}
		parent := syms[p].Name
		if syms[p].Kind == "impl" { // "impl Shape for Circle": the methods are Circle's
			if _, typ, ok := strings.Cut(parent, " for "); ok {
				parent = typ
			}
			if i := strings.IndexByte(parent, '<'); i > 0 {
				parent = parent[:i] // Grid<'a>: Grid
			}
		}
		name = parent + "." + name
	}
	return name
}

func isNoise(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "return", "else", "sizeof", "catch", "main_", "do", "case", "":
		return true
	}
	return false
}
