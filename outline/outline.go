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
	// a block opens on the line its header ends on, as gofmt and Kotlin's
	// style have it, so a declaration that ends its line without one has
	// none (headerGoesOn has the exceptions)
	sameLine bool
}

var (
	goRules = prepare([]rule{
		{re: regexp.MustCompile(`^func (?:\(\s*(?:\w+\s+)?\*?(\w+)(?:\[[^\]]*\])?\s*\) )?(\w+)`), kind: "func", qualify: true},
		r("", `^(type) (\w+)`),
	})
	rustRules = prepare([]rule{
		r("", `^\s*(?:pub(?:\([^)]*\))? )?(?:async )?(?:unsafe )?(?:const )?(fn|struct|enum|trait|mod|type|union|static|macro_rules!) (\w+)`),
		// also impls nested in mod tests or a function; tuple and array types in the generics
		r("impl", `^\s*impl(?:<[^>]*>)? ([\w:<>, '&()\[\];]+?)(?: \{|$| where)`),
	})
	pyRules = prepare([]rule{
		r("", `^\s*(?:async )?(def|class) ([\p{L}_][\p{L}\p{N}_]*)`), // also nested helpers and classes
	})
	jsRules = prepare([]rule{
		// also nested declarations (helpers inside a factory function)
		r("function", `^\s*(?:export |return )?(?:default )?(?:async )?function(?:\s*\*\s*|\s+)(\w+)`),
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
	goLang   = lang{rules: goRules, sameLine: true}
	rustLang = lang{rules: rustRules}
	pyLang   = lang{rules: pyRules, style: indent}
	jsLang   = lang{rules: jsRules}
	cLang    = lang{rules: cRules}
	javaLang = lang{rules: javaRules}
	ktLang   = lang{rules: javaRules, sameLine: true}
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
	"java": &javaLang, "kt": &ktLang, "kts": &ktLang, "cs": &javaLang, "scala": &javaLang,
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
// template string, a block comment: exactly the lines Multiline labels)
// isn't one, so every tool sees the same symbols and ordinals.
func Parse(name string, data []byte, limit int) []Symbol {
	return parse(name, data, limit, false)
}

// parse is Parse; with keepText it also looks for definitions on the lines
// Multiline labels (for a test that they are the ones Parse drops).
func parse(name string, data []byte, limit int, keepText bool) []Symbol {
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
	if sy == nil {
		sy = noText
	}
	src := newSource(sy, lines, len(data))
	var lx *lexer
	if !md {
		lx = newLexer(src)
	}
	strip := sy.line != "" && sy.line != "#" // a rule may match a line with a # comment, as before
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
		inText := false
		if lx != nil {
			inText = lx.in != code // the code in a template's ${…} is code
			label := lx.line(i)
			if inText {
				if text == nil {
					text = make([]bool, len(lines))
				}
				text[i] = true
			}
			if label != "" && !keepText {
				continue // no definition starts here: Multiline labels the line
			}
		}
		if full {
			continue // past the limit only the text lines are still needed, for indentEnd
		}
		if strip || inText { // a name after a comment that closes on the line: */ function f() { in a bundle
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
			out = append(out, Symbol{Line: i + 1, End: i + 1, Kind: kind, Name: strings.TrimSpace(name), Sig: clip(sig), Parent: -1})
			break
		}
	}
	findEnds(lg, src, text, out)
	findParents(out)
	return out
}

// clip shortens a signature to at most 110 characters.
func clip(sig string) string {
	if r := []rune(sig); len(r) > 110 {
		return string(r[:109]) + "…"
	}
	return sig
}

// sigBefore is line n up to byte body, where the definition's block opens,
// made a signature as parse makes one: without comments, trimmed, clipped.
// It is for a line with a { before the block's (func f() interface{} {),
// where parse stopped at the first.
func sigBefore(src *source, n, body int) string {
	lx := newLexer(src)
	lx.scan(n)
	var b strings.Builder
	for _, sg := range lx.segs {
		if sg.kind != comment && sg.from < body {
			b.WriteString(src.lines[n][sg.from:min(sg.to, body)])
		}
	}
	return clip(strings.TrimSpace(b.String()))
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

func findEnds(lg *lang, src *source, text []bool, syms []Symbol) {
	lines := src.lines
	for k := range syms {
		s := &syms[k]
		switch lg.style {
		case braces:
			// stop looking for the opening brace at the next definition
			limit := len(lines)
			if k+1 < len(syms) {
				limit = syms[k+1].Line - 1
			}
			var body int
			s.End, body = braceEnd(src, s.Line-1, limit, s.Kind == "define", lg.sameLine)
			if body > 0 && strings.IndexByte(lines[s.Line-1][:body], '{') >= 0 {
				s.Sig = sigBefore(src, s.Line-1, body)
			}
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
// (`type ID int`, `fn f();`) ends on its own line, and one whose value is
// an expression (fun f() = x, const f = () => `…`) where that expression
// ends. With sameLine, the block opens on the line its header ends on.
// body is where the block's { is on the definition's own line, or -1 when
// it isn't there.
func braceEnd(src *source, start, limit int, define, sameLine bool) (end, body int) {
	lines := src.lines
	body = -1
	if define { // #define with continuation lines
		j := start
		for j+1 < len(lines) && strings.HasSuffix(strings.TrimRight(lines[j], " \t"), "\\") {
			j++
		}
		return j + 1, body
	}
	lx := newLexer(src) // the definition's line starts in code
	depth, opened, parens := 0, false, 0
	var last, prev byte      // the last code byte that isn't blank, and the one right before it (or ' ')
	inStmt, angle := true, 0 // still in the definition's statement; inside <generics>
	// A value without a { block that has a string in it (const f = () =>
	// `…`, type T = "a" | "b", fun f() = """…"""): the definition ends
	// where that expression does, at a ; or at a line that the next doesn't
	// continue, with its brackets closed.
	value, strBody, nest, brackets := false, false, 0, 0
	wrap := 0                // the ( open around the value, which aren't parameters: = ((a) => {
	atValue := func() bool { // right after = or =>
		return last == '=' && !strings.ContainsRune("=!<>", rune(prev)) || last == '>' && prev == '='
	}
	for j := start; j < len(lines); j++ {
		if !opened && j >= limit && j > start {
			switch {
			case strBody && nest > 0: // definitions in the value's brackets ({ f: function() {…} })
			case strBody:
				return j, body // the line before the next definition
			default:
				return start + 1, body
			}
		}
		if src.endBudget -= len(lines[j]) + 1; src.endBudget < 0 {
			break // as if the file ended here
		}
		lx.scan(j)
		line := lines[j]
		gap := true                          // a blank (or a new line) since last
		lineEnd, endPrev := byte(0), byte(0) // how the line ends: its last code byte ('"' for a string), the one before
		for si, sg := range lx.segs {
			if sg.kind == str {
				lineEnd, endPrev = '"', 0
				top := !opened && parens == 0 && angle == 0 && inStmt
				value = value || top && atValue()
				if top && value && !strBody {
					strBody, nest = true, brackets
				}
			}
			if sg.kind != code {
				continue // strings and comments; the code in a template's ${…} counts
			}
			for k := sg.from; k < sg.to; k++ {
				c := line[k]
				if opened { // in a block, or after one, only braces count
					switch c {
					case '{':
						if depth == 0 && j == start {
							body = k
						}
						depth++
					case '}':
						if depth--; depth == 0 {
							if blockFollows(line, lx.segs, si, k+1, value) {
								continue // it wasn't the body: func f() interface{} {
							}
							return j + 1, body
						}
					}
					continue
				}
				blank := c == ' ' || c == '\t'
				if !blank {
					lineEnd, endPrev = c, lineEnd
				}
				if strBody && c == '{' && nest <= 0 && (last == '>' && prev == '=' || !strings.ContainsRune("?:,([=|&!+-*/%^~<>", rune(last))) {
					strBody, brackets = false, 0 // a block after all: the string was in a type (f = (a): T["k"] => {)
				}
				if !blank && c != '=' && c != '>' && !opened && parens == 0 && angle == 0 && inStmt && atValue() {
					value = true
				}
				// a ( around an arrow function's object (=> ({ a: 1 })) or around
				// the whole function (= ((a) => {): what's in it is the value
				wraps := c == '(' && !opened && parens == 0 && !strBody && wrapsValue(line[k+1:], last, prev)
				switch {
				case blank:
					gap = true
				case gap:
					prev, last, gap = ' ', c, false
				default:
					prev, last = last, c
				}
				if strBody {
					switch c {
					case '(', '[', '{':
						nest++
					case ')', ']', '}':
						nest--
					case ';':
						if nest <= 0 {
							return j + 1, body
						}
					}
					continue
				}
				if !opened && parens > 0 && c != '(' && c != ')' {
					continue // braces in the parameters ({ a, b } = opts, struct{ x int }) aren't the body
				}
				if !opened && parens == 0 {
					switch {
					case c == '<':
						angle++
					case c == '>' && prev != '=' && prev != '-': // not => or ->
						angle--
					case c == '[':
						brackets++
					case c == ']':
						brackets--
					case brackets > 0 && (c == '{' || c == '}' || c == ';'):
						// in a type's brackets, not the body or the end:
						// fn f() -> [u8; 4] {, func f[T interface{ M() }]() {
						continue
					}
				}
				switch c {
				case '(':
					if wraps {
						wrap++
					} else {
						parens++
					}
				case ')':
					if parens == 0 && wrap > 0 {
						wrap--
					} else {
						parens--
					}
				case '{':
					if depth == 0 && j == start {
						body = k
					}
					depth++
					opened = true
				case '}':
					depth--
				case ';':
					if depth == 0 {
						return j + 1, body
					}
				}
			}
		}
		goesOn := strings.IndexByte(continuesAfter, lineEnd) >= 0 && (lineEnd != '>' || endPrev == '=') && // a > ends a type, => goes on
			!((lineEnd == '+' || lineEnd == '-') && endPrev == lineEnd) // and so does i++
		ends := !lx.inText() && !goesOn && !nextContinues(lines, j)
		if strBody {
			if ends && nest <= 0 && lineEnd != 0 { // a line of only a comment doesn't end it
				return j + 1, body
			}
			continue
		}
		if ends && !opened && parens <= 0 && brackets <= 0 && wrap <= 0 && lineEnd != 0 && (value || sameLine) && !headerGoesOn(lines, j) {
			// the expression ended without a block (fun f() = x), or the
			// header did (type ID int): a block further down isn't its
			return j + 1, body
		}
		if ends && !opened && parens <= 0 && angle <= 0 {
			inStmt = false // the statement ended without a value: a later = isn't its
		}
		if !opened && j-start > 30 {
			return start + 1, body
		}
	}
	if opened || strBody {
		return len(lines), body
	}
	return start + 1, body
}

// blockFollows reports whether the rest of a line, after a {…} that closed
// at byte k-1 of its part si, opens another block of the same definition.
// Then the one that closed wasn't the body: it was in a type
// (func f() interface{} {, (): { ok: boolean } => {, -> Vec<{ N }> {) or
// in an expression (= try { … } catch (e: E) {). The block isn't the
// definition's when a statement ends before it (;), when two names follow
// each other outside brackets, as a keyword and a name do where another
// definition starts (} function next() {; the words in goesOnWith aside),
// or at an = once the definition has its value: the next declarator
// (}, g = () => {).
func blockFollows(line string, segs []seg, si, k int, value bool) bool {
	word := -1     // where the name the scan is in started, or -1
	named := false // a name ended, and only blanks since
	nest := 0      // the ( and [ open since the block closed: (x T) are parameters
	var before byte
	for ; si < len(segs); si, k = si+1, -1 {
		sg := segs[si]
		if sg.kind == comment {
			continue
		}
		if sg.kind != code {
			word, named, before = -1, false, '"'
			continue
		}
		for i := max(k, sg.from); i < sg.to; i++ {
			c := line[i]
			switch {
			case c == '{':
				return true
			case c == ';':
				return false
			case c == ' ' || c == '\t':
				if word >= 0 {
					named = nest <= 0 && !goesOnWith[line[word:i]]
				}
				word = -1
			case isIdent(c) || c >= 0x80:
				if named {
					return false
				}
				if word < 0 {
					word = i
				}
			default:
				switch c {
				case '(', '[':
					nest++
				case ')', ']':
					nest--
				case '=':
					lone := !strings.ContainsRune("=!<>", rune(before)) && (i+1 >= sg.to || line[i+1] != '=' && line[i+1] != '>')
					if lone && value && nest <= 0 {
						return false
					}
				}
				word, named = -1, false
			}
			before = c
		}
		word = -1 // a name doesn't go on in the next part
	}
	return false
}

// wrapsValue reports whether a ( that follows last and prev (see braceEnd)
// and comes before rest is around a value, so that a { in it can be the
// body: around the object an arrow function returns (=> ({ a: 1 }), but
// not the parameters of a second one, => ({ a }) => a), or around a whole
// arrow function (= ((a) => {, set: ((v) => {).
func wrapsValue(rest string, last, prev byte) bool {
	rest = strings.TrimLeft(rest, " \t")
	switch {
	case rest == "":
		return false
	case last == '>' && prev == '=':
		if rest[0] != '{' {
			return false
		}
		depth := 1 // the ( itself
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				if depth--; depth == 0 {
					return !strings.Contains(rest[i:], "=>")
				}
			}
		}
		return true
	case last == ':' || last == '=' && !strings.ContainsRune("=!<>", rune(prev)):
		return rest[0] == '('
	}
	return false
}

// headerGoesOn reports whether the next line of code goes on with the
// definition although line j ended like a statement: with its block, on a
// line of its own ({), its value (= x), or more of its header (where T : X,
// private constructor(…), also after its annotations).
func headerGoesOn(lines []string, j int) bool {
	for k := j + 1; k < len(lines) && k <= j+10; k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") {
			continue
		}
		if i := strings.Index(t, "constructor"); i >= 0 && (i == 0 || !isIdent(t[i-1])) && (i+11 == len(t) || !isIdent(t[i+11])) {
			return true
		}
		if t[0] == '@' {
			continue // an annotation of what the line after it declares
		}
		return t[0] == '{' || t[0] == '=' && !strings.HasPrefix(t, "==") || strings.HasPrefix(t, "where") && (len(t) == 5 || !isIdent(t[5]))
	}
	return false
}

// goesOnWith are the words that continue a definition after a block, with
// a name right after them: } else if (x) {, <T extends { id: string }>
// extends Base {.
var goesOnWith = map[string]bool{"else": true, "extends": true, "implements": true, "throws": true, "where": true}

// An expression goes on after a line that ends with one of these, or
// when the next line starts with one of continuesBefore.
const (
	continuesAfter  = "+-*/%&|^?:.,([{=<>!~"
	continuesBefore = "+-%&|^?:.,>"
)

func nextContinues(lines []string, j int) bool {
	for k := j + 1; k < len(lines) && k <= j+10; k++ {
		t := strings.TrimSpace(lines[k])
		if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") {
			continue // comment lines don't end it
		}
		return strings.IndexByte(continuesBefore, t[0]) >= 0 || strings.HasPrefix(t, "else") && (len(t) == 4 || !isIdent(t[4]))
	}
	return false
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
// lines contain it (the nearest one before it).
func findParents(syms []Symbol) {
	contains := func(j, i int) bool {
		return syms[j].Line < syms[i].Line && syms[j].End >= syms[i].End && syms[j].End > syms[j].Line
	}
	furthest := 0 // the last line of any symbol so far
	for i := range syms {
		if furthest >= syms[i].End {
			// Blocks nest, so it is the symbol before it or one of that one's
			// parents, which is quick even with thousands of siblings. Only
			// ends that overlap (a miscounted brace) need the long way.
			for j := i - 1; j >= 0 && syms[i].Parent < 0; j = syms[j].Parent {
				if contains(j, i) {
					syms[i].Parent = j
				}
			}
			for j := i - 1; j >= 0 && syms[i].Parent < 0; j-- {
				if contains(j, i) {
					syms[i].Parent = j
				}
			}
		}
		furthest = max(furthest, syms[i].End)
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
