package outline

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Text is where a file's strings and comments span lines.
type Text struct {
	// In labels each line (1-based) that starts inside a string or comment
	// opened on an earlier line: "string" or "comment". The opening line
	// isn't labelled (code may come before the mark); the lines after it,
	// up to and including the closing one, are. nil if there are none.
	In map[int]string
	// Code holds, per line, the code inside the ${…} of JavaScript template
	// strings (and the $(…) and ${…} of shell "…") as byte ranges
	// [start, end) of the line. A ${…} may span lines, and hold strings of
	// its own.
	Code map[int][][2]int
}

// Multiline finds the strings and comments of a file that span lines:
// docstrings and other triple-quoted strings, template and raw strings,
// heredocs, block comments. Parse drops the definitions it would find on
// their lines; a tool labels a hit on such a line "string" or "comment".
// name only selects the language; a file type without such strings and
// comments gives the zero Text.
func Multiline(name string, data []byte) Text {
	sy := syntaxByExt[ext(name)]
	if sy == nil || len(data) == 0 {
		return Text{}
	}
	lines := splitLines(data) // "\r\n" ends a line like "\n", as for Parse
	var t Text
	lx := newLexer(&source{sy: sy, lines: lines})
	for n := range lines {
		if lx.inText() {
			if t.In == nil {
				t.In = map[int]string{}
			}
			t.In[n+1] = "string" // also a line that starts in a template's ${…}: Code has its code
			if lx.in == comment {
				t.In[n+1] = "comment"
			}
		}
		lx.scan(n)
		if len(lx.interps) > 0 {
			if t.Code == nil {
				t.Code = map[int][][2]int{}
			}
			t.Code[n+1] = append([][2]int(nil), lx.interps...)
		}
	}
	return t
}

func ext(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}

// textSyntax is how a language writes strings and comments, as far as telling
// code from text goes.
type textSyntax struct {
	line      string // a line comment: "//", "#" or "--"
	hashWord  bool   // # starts a comment only at the start of a word (shell: $#, ${#x})
	block     bool   // /* … */
	chars     bool   // '…' is a character literal or a lifetime, not a string (Rust)
	digitSep  bool   // ' between digits separates them (C++14: 10'000)
	multi     byte   // multiDQ: "…" may span lines (Rust); multiShell: '…' and "…" may (shell)
	regex     bool   // /…/ can be a regex literal (JavaScript)
	backtick  byte   // `…`: rawTick (Go: no escapes) or template (JavaScript: escapes and ${…})
	triple    string // the quotes that make triple-quoted strings: `"` for """, `"'` for ''' too
	tripleEsc bool   // a backslash escapes in them (Python, Java), not in raw ones (Kotlin, Scala)
	rawQuotes bool   // C# raw strings: three or more quotes, closed by as many
	rawRust   bool   // r"…", r#"…"#, br"…"
	rawCpp    bool   // R"(…)", R"delim(…)delim"
	verbatim  bool   // C# @"…" with "" for a quote
	luaLong   bool   // [[…]], [==[…]==], --[[…]]
	rubyBegin bool   // =begin … =end at the start of lines
	heredoc   byte   // <<TAG … TAG: hereRuby (<<~TAG, <<-TAG) or hereShell (<<-TAG, << TAG)
	special   [256]bool
}

const (
	rawTick    = 1
	template   = 2
	multiDQ    = 1
	multiShell = 2
	hereRuby   = 1
	hereShell  = 2
)

func mk(sy textSyntax) *textSyntax {
	for _, c := range "\\\"'" + sy.line {
		sy.special[c] = true
	}
	set := func(on bool, c byte) {
		if on {
			sy.special[c] = true
		}
	}
	set(sy.block || sy.regex, '/')
	set(sy.backtick != 0, '`')
	set(sy.backtick == template || sy.multi == multiShell, '{') // brackets count inside a ${…} or $(…)
	set(sy.backtick == template || sy.multi == multiShell, '}')
	set(sy.multi == multiShell, '(')
	set(sy.multi == multiShell, '$')
	set(sy.multi == multiShell, ')')
	set(sy.rawRust, 'r')
	set(sy.rawCpp, 'R')
	set(sy.verbatim, '@')
	set(sy.luaLong, '[')
	set(sy.heredoc != 0, '<')
	return &sy
}

var (
	cLike   = mk(textSyntax{line: "//", block: true})
	goText  = mk(textSyntax{line: "//", block: true, backtick: rawTick})
	jsText  = mk(textSyntax{line: "//", block: true, backtick: template, regex: true})
	pyText  = mk(textSyntax{line: "#", triple: `"'`, tripleEsc: true})
	cText   = mk(textSyntax{line: "//", block: true, rawCpp: true, digitSep: true})
	javText = mk(textSyntax{line: "//", block: true, triple: `"`, tripleEsc: true})
	ktText  = mk(textSyntax{line: "//", block: true, triple: `"`})
	csText  = mk(textSyntax{line: "//", block: true, rawQuotes: true, verbatim: true})
	rsText  = mk(textSyntax{line: "//", block: true, chars: true, multi: multiDQ, rawRust: true})
	luaText = mk(textSyntax{line: "--", luaLong: true})
	rbText  = mk(textSyntax{line: "#", rubyBegin: true, heredoc: hereRuby})
	shText  = mk(textSyntax{line: "#", hashWord: true, multi: multiShell, heredoc: hereShell})
	noText  = mk(textSyntax{}) // only one-line strings
)

var syntaxByExt = map[string]*textSyntax{
	"go": goText,
	"js": jsText, "mjs": jsText, "cjs": jsText, "ts": jsText, "tsx": jsText, "jsx": jsText, "mts": jsText, "cts": jsText,
	"svelte": jsText, "vue": jsText,
	"py": pyText, "pyi": pyText, "gd": pyText,
	"c": cText, "h": cText, "cc": cText, "cpp": cText, "cxx": cText, "hpp": cText, "hh": cText,
	"java": javText, "kt": ktText, "kts": ktText, "scala": ktText, "cs": csText, "rs": rsText,
	"lua": luaText, "rb": rbText, "sh": shText, "bash": shText, "zsh": shText,
	// no outline rules, but block comments to label
	"swift": cLike, "css": cLike, "scss": cLike, "less": cLike, "php": cLike, "dart": cLike,
	"zig": cLike, "glsl": cLike, "wgsl": cLike, "hlsl": cLike,
}

type kind uint8

const (
	code kind = iota
	str       // also a character or regex literal, a heredoc
	comment
	subcode // the code in a string's ${…} or $(…)
)

// seg is a part of a line of one kind: line[from:to].
type seg struct {
	from, to int
	kind     kind
}

// lexer walks a file's lines, carrying strings and comments that span
// lines over to the next one. A scan that starts on a line that begins in
// code (a definition's line) needs no earlier lines.
type lexer struct {
	sy    *textSyntax
	lines []string
	n     int   // the line being scanned
	segs  []seg // that line, cut into parts of one kind
	// the code inside a string's ${…} on that line (subcode in segs)
	interps [][2]int
	from    int  // where the current part started
	cur     kind // its kind
	noRegex bool // a regex literal didn't close on this line: don't look again (long lines stay linear)

	// the string or comment the scan is in after the last line, if any
	in     kind   // code when it's in none
	close  string // what ends it
	esc    bool   // a backslash escapes the next byte
	tmpl   bool   // ${…} is code (a template; also $(…) in a shell "…")
	quote  bool   // "" is a quote, not the end (C# verbatim)
	term   string // a heredoc or =begin comment: the line that ends it
	trim   bool   // the terminator may be indented
	prefix bool   // the terminator line only starts with term (=end)
	// the ${…} (or shell $(…)) the scan is in, innermost last. A string
	// opened in one is nested.
	stack    []subst
	codeFrom int // where the code of the innermost ${…} started on this line; -1 outside one

	tag     string // a heredoc opened on this line: its terminator, from the next line on
	tagTrim bool
	src     *source
}

// source is a file's lines, and what the lexers that scan them share.
type source struct {
	sy    *textSyntax
	lines []string
	terms map[hereKey][]int // the lines that could end a heredoc, built when one starts
}

// subst is code inside a string: ${…} in a template, ${…} or $(…) in a
// shell "…".
type subst struct {
	open, close byte   // { } or ( )
	depth       int    // how many open brackets of that kind inside it
	back        string // the mark that ends the string it's in; "" for a shell ${…} in code
	inSub       bool   // that ${…} in code is itself in a string's $(…)
}

type hereKey struct {
	tag  string
	trim bool // the terminator may be indented (<<-, <<~)
}

func newLexer(src *source) *lexer {
	return &lexer{sy: src.sy, lines: src.lines, src: src, codeFrom: -1}
}

// codeKind is the kind of code at this point: subcode in a string's ${…}.
func (lx *lexer) codeKind() kind {
	if n := len(lx.stack); n > 0 && (lx.stack[n-1].back != "" || lx.stack[n-1].inSub) {
		return subcode
	}
	return code
}

// inText: the next line starts inside a string or comment, or in the code
// of a template's ${…}.
func (lx *lexer) inText() bool {
	if lx.in != code {
		return true
	}
	for _, sub := range lx.stack {
		if sub.back != "" {
			return true // in the code of a ${…} in a string
		}
	}
	return false
}

// scan cuts line n into lx.segs and moves the state past it.
func (lx *lexer) scan(n int) {
	s := lx.lines[n]
	lx.n, lx.segs, lx.interps, lx.from, lx.cur, lx.noRegex = n, lx.segs[:0], lx.interps[:0], 0, lx.in, false
	if lx.in == code && lx.codeKind() == subcode {
		lx.cur, lx.codeFrom = subcode, 0 // a ${…} that continues from the line before
	}
	if lx.term != "" { // inside a heredoc or =begin comment: whole lines
		if lx.isTerm(s) {
			lx.in, lx.term = code, ""
		}
		lx.flush(len(s))
		return
	}
	if lx.in == code && lx.sy.rubyBegin && strings.HasPrefix(s, "=begin") && (len(s) == 6 || s[6] <= ' ') {
		lx.cur, lx.in, lx.term, lx.trim, lx.prefix = comment, comment, "=end", false, true
		lx.flush(len(s))
		return
	}
	for i := 0; i < len(s); {
		if lx.in != code {
			i = lx.text(s, i)
		} else {
			i = lx.code(s, i)
		}
	}
	lx.flush(len(s))
	if lx.codeFrom >= 0 {
		if lx.codeFrom < len(s) {
			lx.interps = append(lx.interps, [2]int{lx.codeFrom, len(s)})
		}
		lx.codeFrom = -1
	}
	if lx.tag != "" {
		if lx.in == code {
			lx.in, lx.term, lx.trim, lx.prefix = str, lx.tag, lx.tagTrim, false
		}
		lx.tag = ""
	}
}

// to starts a part of kind k at i.
func (lx *lexer) to(i int, k kind) {
	if k == lx.cur {
		return
	}
	if i > lx.from {
		lx.segs = append(lx.segs, seg{lx.from, i, lx.cur})
	}
	lx.from, lx.cur = i, k
}

func (lx *lexer) flush(n int) {
	if n > lx.from {
		lx.segs = append(lx.segs, seg{lx.from, n, lx.cur})
	}
}

// open starts a string or comment of kind k at s[at:], whose opening mark
// is n bytes long.
func (lx *lexer) open(at, n int, k kind, close string, esc bool) int {
	lx.to(at, k)
	lx.in, lx.close, lx.esc, lx.tmpl, lx.quote = k, close, esc, false, false
	return at + n
}

// text scans s from i inside a string or comment, up to its end.
func (lx *lexer) text(s string, i int) int {
	if !lx.esc && !lx.tmpl && !lx.quote {
		j := strings.Index(s[i:], lx.close)
		if j < 0 {
			return len(s)
		}
		return lx.closed(i + j + len(lx.close))
	}
	for i < len(s) {
		c := s[i]
		switch {
		case c == '\\' && lx.esc:
			i += 2
		case c == '$' && lx.tmpl && i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '(' && lx.sy.multi == multiShell):
			// code up to the matching } or ), on this line or a later one
			sub := subst{open: '{', close: '}', back: lx.close}
			if s[i+1] == '(' {
				sub.open, sub.close = '(', ')'
			}
			lx.stack = append(lx.stack, sub)
			lx.in, lx.codeFrom = code, i+2
			lx.to(i+2, subcode)
			return i + 2
		case c == '"' && lx.quote && i+1 < len(s) && s[i+1] == '"':
			i += 2
		case c == lx.close[0] && strings.HasPrefix(s[i:], lx.close):
			return lx.closed(i + len(lx.close))
		default:
			i++
		}
	}
	return i
}

// closed ends the string or comment just before i.
func (lx *lexer) closed(i int) int {
	lx.in = code
	lx.to(i, lx.codeKind())
	if len(lx.stack) > 0 && lx.stack[len(lx.stack)-1].back != "" && lx.codeFrom < 0 {
		lx.codeFrom = i // a template nested in a ${…} closed: its code goes on
	}
	return i
}

// code scans s from i in code, up to where a string or comment starts or
// past one that ends on this line.
func (lx *lexer) code(s string, i int) int {
	sy := lx.sy
	for i < len(s) && !sy.special[s[i]] {
		i++
	}
	if i >= len(s) {
		return i
	}
	c := s[i]
	next := byte(0)
	if i+1 < len(s) {
		next = s[i+1]
	}
	switch {
	case (c == '{' || c == '}' || c == '(' || c == ')') && len(lx.stack) > 0:
		top := &lx.stack[len(lx.stack)-1]
		switch {
		case c == top.open:
			top.depth++
		case c != top.close:
		case top.depth > 0:
			top.depth--
		default: // the end of the ${…}: back in its string
			back := top.back
			lx.stack = lx.stack[:len(lx.stack)-1]
			if back == "" {
				break // a shell ${…} in code
			}
			if lx.codeFrom >= 0 {
				lx.interps = append(lx.interps, [2]int{lx.codeFrom, i})
			}
			lx.codeFrom = -1
			lx.to(i, str)
			lx.in, lx.close, lx.esc, lx.tmpl, lx.quote = str, back, true, true, false
		}
	case c == '$' && next == '{' && sy.multi == multiShell:
		lx.stack = append(lx.stack, subst{open: '{', close: '}', inSub: lx.codeKind() == subcode}) // # in it isn't a comment
		return i + 2
	case sy.line != "" && c == sy.line[0] && strings.HasPrefix(s[i:], sy.line) && (!sy.hashWord || i == 0 || s[i-1] == ' ' || s[i-1] == '\t') &&
		!(sy.multi == multiShell && len(lx.stack) > 0 && lx.stack[len(lx.stack)-1].open == '{'): // ${x%%[ #]*}
		if sy.luaLong && next == '-' {
			if close, n := longBracket(s, i+2); n > 0 {
				return lx.open(i, 2+n, comment, close, false)
			}
		}
		lx.to(i, comment)
		return len(s)
	case c == '\\':
		return i + 2
	case c == '/':
		if next == '*' && sy.block {
			return lx.open(i, 2, comment, "*/", false)
		}
		if sy.regex && !lx.noRegex && next != '/' && next != '*' && regexStart(s[:i]) {
			if end := regexEnd(s, i); end > 0 {
				lx.to(i, str) // a regex literal: its quotes and backticks open nothing
				lx.to(end, lx.codeKind())
				return end
			}
			lx.noRegex = true
		}
	case c == '`' && sy.backtick != 0:
		if lx.codeFrom >= 0 { // a template nested in a ${…}: not code
			if lx.codeFrom < i {
				lx.interps = append(lx.interps, [2]int{lx.codeFrom, i})
			}
			lx.codeFrom = -1
		}
		i = lx.open(i, 1, str, "`", sy.backtick == template)
		lx.tmpl = sy.backtick == template
		return i
	case c == '"' || c == '\'':
		if sy.triple != "" && strings.IndexByte(sy.triple, c) >= 0 && i+2 < len(s) && next == c && s[i+2] == c {
			return lx.open(i, 3, str, s[i:i+3], sy.tripleEsc)
		}
		if c == '"' && sy.rawQuotes {
			n := 1
			for i+n < len(s) && s[i+n] == '"' {
				n++
			}
			if n >= 3 {
				return lx.open(i, n, str, s[i:i+n], false)
			}
		}
		switch {
		case sy.multi == multiShell: // '…' has no escapes, except $'…'; "…" has $(…) and ${…}
			i = lx.open(i, 1, str, s[i:i+1], c == '"' || i > 0 && s[i-1] == '$')
			lx.tmpl = c == '"'
			return i
		case sy.multi == multiDQ && c == '"':
			return lx.open(i, 1, str, `"`, true)
		case sy.digitSep && c == '\'' && digitSep(s, i):
			return i + 1
		}
		if c == '\'' && sy.chars {
			if end := charEnd(s, i); end > 0 {
				lx.to(i, str)
				lx.to(end, lx.codeKind())
				return end
			}
			return i + 1 // a lifetime
		}
		return lx.oneLine(s, i)
	case c == 'r' && sy.rawRust:
		if i > 0 && isIdent(s[i-1]) && !((s[i-1] == 'b' || s[i-1] == 'c') && (i < 2 || !isIdent(s[i-2]))) {
			break
		}
		j := i + 1
		for j < len(s) && s[j] == '#' {
			j++
		}
		if j < len(s) && s[j] == '"' {
			return lx.open(i, j+1-i, str, `"`+s[i+1:j], false)
		}
	case c == 'R' && sy.rawCpp && next == '"':
		p := strings.TrimSuffix(strings.TrimSuffix(s[:i], "u8"), "L")
		if len(p) == i && i > 0 && (s[i-1] == 'u' || s[i-1] == 'U') {
			p = s[:i-1]
		}
		if len(p) > 0 && isIdent(p[len(p)-1]) {
			break
		}
		if k := strings.IndexByte(s[i+2:min(len(s), i+19)], '('); k >= 0 {
			delim := s[i+2 : i+2+k]
			if !strings.ContainsAny(delim, " ()\\\t\"") {
				return lx.open(i, 3+k, str, ")"+delim+`"`, false)
			}
		}
	case c == '@' && sy.verbatim:
		n := 0
		if next == '"' {
			n = 2
		} else if next == '$' && i+2 < len(s) && s[i+2] == '"' {
			n = 3
		}
		if n > 0 {
			i = lx.open(i, n, str, `"`, false)
			lx.quote = true
			return i
		}
	case c == '[' && sy.luaLong:
		if close, n := longBracket(s, i); n > 0 {
			return lx.open(i, n, str, close, false)
		}
	case c == '<' && sy.heredoc != 0 && next == '<':
		if end := lx.heredocStart(s, i); end > 0 {
			return end
		}
		return i + 2
	}
	return i + 1
}

// oneLine skips a '…' or "…" string that starts at s[i]. One that doesn't
// close stops at the end of the line, unless a backslash continues it.
func (lx *lexer) oneLine(s string, i int) int {
	q, j := s[i], i+1
	for j < len(s) && s[j] != q {
		if s[j] == '\\' {
			j++
		}
		j++
	}
	lx.to(i, str)
	switch {
	case j > len(s): // the backslash at the end escapes the newline
		lx.in, lx.close, lx.esc, lx.tmpl, lx.quote = str, string(q), true, false, false
		return len(s)
	case j == len(s):
		return j
	}
	lx.to(j+1, lx.codeKind())
	return j + 1
}

// charEnd is the index after a Rust character literal at s[i] ('x',
// '\n', '\u{1F600}'), or -1 if the quote starts a lifetime ('a).
func charEnd(s string, i int) int {
	if i+2 >= len(s) {
		return -1
	}
	if s[i+1] == '\\' {
		if i+3 < len(s) {
			if k := strings.IndexByte(s[i+3:min(len(s), i+13)], '\''); k >= 0 {
				return i + 4 + k
			}
		}
		return -1
	}
	_, size := utf8.DecodeRuneInString(s[i+1:])
	if j := i + 1 + size; j < len(s) && s[j] == '\'' {
		return j + 1
	}
	return -1
}

// longBracket reads a Lua long bracket at s[i] ([[ or [==[): its closing
// mark and its length, or n 0 if there is none.
func longBracket(s string, i int) (close string, n int) {
	if i >= len(s) || s[i] != '[' {
		return "", 0
	}
	j := i + 1
	for j < len(s) && s[j] == '=' {
		j++
	}
	if j >= len(s) || s[j] != '[' {
		return "", 0
	}
	return "]" + s[i+1:j] + "]", j + 1 - i
}

// heredocStart reads <<TAG at s[i] (<<~TAG, <<-'TAG', and in shell
// << TAG): the heredoc's lines start on the next line and end with a line
// that is just TAG. It returns the index after TAG, or -1 when this is a
// shift (1 << n, items << item) or no line ends it.
func (lx *lexer) heredocStart(s string, i int) int {
	j := i + 2
	if j < len(s) && (s[j] == '<' || s[j] == '=') {
		return -1 // a here-string (<<<) or <<=
	}
	trim := false
	if j < len(s) && (s[j] == '-' || s[j] == '~' && lx.sy.heredoc == hereRuby) {
		trim = true
		j++
	}
	if lx.sy.heredoc == hereShell {
		for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
			j++
		}
	}
	q := byte(0)
	if j < len(s) && (s[j] == '\'' || s[j] == '"') {
		q = s[j]
		j++
	}
	k := j
	for k < len(s) && (isIdent(s[k]) && s[k] != '$') {
		k++
	}
	if k == j || s[j] >= '0' && s[j] <= '9' {
		return -1
	}
	tag := s[j:k]
	if q != 0 {
		if k >= len(s) || s[k] != q {
			return -1
		}
		k++
	}
	if !lx.src.terminated(tag, trim, lx.n) {
		return -1
	}
	if lx.tag == "" { // of several heredocs on a line, only the first is followed
		lx.tag, lx.tagTrim = tag, trim
	}
	return k
}

// terminated: a line after line n is just tag (indented, if trim), so a
// heredoc there would end. The lines that could end one are indexed the
// first time it's asked, so many heredocs don't make Parse slow.
func (src *source) terminated(tag string, trim bool, n int) bool {
	if src.terms == nil {
		src.terms = map[hereKey][]int{}
		for k, l := range src.lines {
			l = strings.TrimSuffix(l, "\r")
			t := strings.TrimLeft(l, " \t")
			if t == "" || t[0] >= '0' && t[0] <= '9' || strings.IndexFunc(t, func(r rune) bool { return r > 127 || !isIdent(byte(r)) || r == '$' }) >= 0 {
				continue
			}
			if t == l {
				src.terms[hereKey{t, false}] = append(src.terms[hereKey{t, false}], k)
			}
			src.terms[hereKey{t, true}] = append(src.terms[hereKey{t, true}], k)
		}
	}
	at := src.terms[hereKey{tag, trim}]
	return sort.SearchInts(at, n+1) < len(at)
}

func (lx *lexer) isTerm(l string) bool { return termLine(l, lx.term, lx.trim, lx.prefix) }

func termLine(l, term string, trim, prefix bool) bool {
	l = strings.TrimSuffix(l, "\r")
	if trim {
		l = strings.TrimLeft(l, " \t")
	}
	if prefix {
		return strings.HasPrefix(l, term) && (len(l) == len(term) || l[len(term)] <= ' ')
	}
	return l == term
}

// digitSep: the ' at s[i] separates digits of a number (10'000,
// 0xFF'FF), it doesn't start a character literal.
func digitSep(s string, i int) bool {
	if i == 0 || i+1 >= len(s) || !isHex(s[i-1]) || !isHex(s[i+1]) {
		return false
	}
	k := i - 1
	for k > 0 && i-k < 40 && (isIdent(s[k-1]) || s[k-1] == '\'' || s[k-1] == '.') {
		k-- // back to the start of the token, but not far (long lines stay linear)
	}
	return s[k] >= '0' && s[k] <= '9'
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// regexStart: a slash after this JavaScript code starts a regex literal,
// not a division (it follows an operator, an opening bracket, or nothing).
func regexStart(code string) bool {
	t := strings.TrimRight(code, " \t")
	if t == "" {
		return true
	}
	if strings.IndexByte("(,=:[!&|?{};+-*%<>~^", t[len(t)-1]) >= 0 {
		return true
	}
	for _, kw := range []string{"return", "typeof", "case", "in", "of", "delete", "void", "yield", "await"} {
		if strings.HasSuffix(t, kw) && (len(t) == len(kw) || !isIdent(t[len(t)-len(kw)-1])) {
			return true
		}
	}
	return false
}

// regexEnd is the index after a regex literal that starts at s[i] ('/'),
// or -1 if it doesn't close on this line.
func regexEnd(s string, i int) int {
	class := false
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			class = true
		case ']':
			class = false
		case '/':
			if !class {
				return j + 1
			}
		}
	}
	return -1
}

func isIdent(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
