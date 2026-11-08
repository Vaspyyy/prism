package outline

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// lineLabels returns one byte per line of src: '.' for code, 's' for a line
// inside a string, 'c' inside a comment.
func lineLabels(name, src string) string {
	t := Multiline(name, []byte(src))
	n := strings.Count(strings.TrimSuffix(src, "\n"), "\n") + 1
	var b strings.Builder
	for i := 1; i <= n; i++ {
		switch t.In[i] {
		case "string":
			b.WriteByte('s')
		case "comment":
			b.WriteByte('c')
		case "":
			b.WriteByte('.')
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

func TestMultiline(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// the opening line is code, the closing line isn't
		{"a.py", "x = 1\n\"\"\"doc\ndef f():\n\"\"\"\ny = 2\n", "..ss."},
		{"a.py", "'''\ndef f():\n'''\n", ".ss"},
		{"a.py", "\"\"\"has \\\"\"\" inside\ndef f():\n\"\"\"\n", ".ss"},   // an escaped quote doesn't close it
		{"a.py", "# \"\"\" in a comment\ns = '\"\"\"'\ndef f():\n", "..."}, // nor open it
		{"a.gd", "func f():\n\t\"\"\"\n\tdoc\n\t\"\"\"\n", "..ss"},
		{"a.go", "/*\nfunc x() {}\n*/\nfunc y() {}\n", ".cc."},
		{"a.go", "var p = `C:\\`\nfunc f() {}\n", ".."}, // no escapes in a raw string
		{"a.go", "var r = '`'\nfunc f() {}\n", ".."},
		{"a.ts", "const s = `\nfunction f() {\n`;\n", ".ss"},
		{"a.ts", "const s = `a \\` b\nfunction f() {\n`;\n", ".ss"},
		{"a.ts", "const r = /`/;\nfunction f() {}\n", ".."}, // a regex literal
		{"a.ts", "const q = '`';\nfunction f() {}\n", ".."},
		{"a.ts", "const r = /a\\/*b/;\nfunction f() {}\n", ".."},
		{"a.svelte", "<script>\nconst s = `\nfunction f() {\n`;\n</script>\n", "..ss."},
		{"a.c", "/* one line */\nint f(void);\n", ".."},
		{"a.c", "char *s = \"a \\\nint f(void) {\";\nint g(void);\n", ".s."}, // a backslash continues a string
		{"a.cpp", "auto s = R\"x(\n)\" int f() {\n)x\";\nint g();\n", ".ss."},
		{"a.cpp", "auto s = LR\"(\nint f() {\n)\";\n", ".ss"},
		{"a.cpp", "int fooR(\"x\");\nint g();\n", ".."},
		{"a.cs", "var s = @\"say \"\"hi\"\"\npublic void F() {\n\";\n", ".ss"},
		{"a.cs", "var s = \"\"\"\npublic class C {\n\"\"\";\n", ".ss"},
		{"a.java", "String s = \"\"\"\n    \\\"\"\"\n    class C {\n    \"\"\";\n", ".sss"},
		{"a.kt", "val s = \"\"\"a\\\"\"\"\nfun f() {}\n", ".."}, // no escapes in a raw string
		{"a.rs", "let s = \"a\nfn f() {\n\";\n", ".ss"},
		{"a.rs", "let s = r#\"a \" b\nfn f() {\n\"#;\n", ".ss"},
		{"a.rs", "let s = br\"\nfn f() {\n\";\n", ".ss"},
		{"a.rs", "let q = '\"'; let b = '{';\nfn f<'a>(x: &'a str) {}\n", ".."},
		{"a.rs", "let q = '\\''; let s = \"\nfn f() {\n\";\n", ".ss"},
		{"a.rs", "let r#type = 1;\nfn f() {}\n", ".."}, // a raw identifier
		{"a.lua", "--[[\nfunction f()\n]]\nlocal s = [==[\nfunction g()\n]==]\n", ".cc.ss"},
		{"a.lua", "-- [[ just a comment\nfunction f()\n", ".."},
		{"a.rb", "=begin\ndef f\n=end\ndef g\n", ".cc."},
		{"a.rb", "X = <<~TEXT\n  def f\n  TEXT\ndef g\n", ".ss."},
		{"a.rb", "class << self\n  def f\n  end\nend\nitems << item\n", "....."},
		{"a.rb", "x = <<NOPE\ndef f\nend\n", "..."}, // no line ends it: a shift, not a heredoc
		{"a.sh", "cat <<EOF\nf() {\nEOF\ng() {\n", ".ss."},
		{"a.sh", "cat <<-'END' | grep x\n\tf() {\n\tEND\n", ".ss"},
		{"a.sh", "cat << EOF\nf() {\nEOF\n", ".ss"},
		{"a.sh", "echo $((1 << 2))\ncat <<< \"x\"\n# <<EOF\necho $# <<X\nf() {\n", "....."},
		{"a.txt", "/*\nnot code\n*/\n", "..."},
		// shell quotes span lines; '…' has no escapes, $'…' has
		{"a.sh", "f() {\n  awk '{\n    print $1\n  }' file\n}\n", "..ss."},
		{"a.sh", "echo \"a \\\" {\n}\"\ng() {\n", ".s."},
		{"a.sh", "printf $'it\\'s {\\n'\ng() {\n", ".."},
		{"a.sh", "echo 'a\\'\ng() {\n", ".."},
		// $(…) and ${…} in a "…" are code, with quotes of their own
		{"a.sh", "W=\"$(dl \"https://x/$(dl \"https://x/ip\")\" | awk -F '\"' '/c/{print $4}')\"\nf() {\n", ".."},
		{"a.sh", "x=\"$(cat <<EOF\nf() {\nEOF\n)\"\ng() {\n", ".sss."},
		{"a.sh", "echo \"${x:-\"a}b\"}\" # it's\ng() {\n", ".."},
		{"a.sh", "n=\"${l%%[ #]*}\"; m=${l%%[ #]*} # it's\ng() {\n", ".."}, // # in ${…} isn't a comment
		// a failed <<EOF lookup doesn't hide a later <<-EOF
		{"a.sh", "a=$((b<<EOF))\ncat <<-EOF\nfake() {\n\tEOF\ng() {\n", "..ss."},
		// C# raw strings close with as many quotes as they opened with
		{"a.cs", "var e = \"\"\"\"\n    has \"\"\" inside\n    \"\"\"\";\npublic void Real2() {}\n", ".ss."},
		{"a.cs", "var e = \"\";\npublic void F() {}\n", ".."},
		// C++14 digit separators aren't character literals
		{"a.cpp", "if (x > 10'000) { /*\nint f();\n*/\n", ".cc"},
		{"a.cpp", "c = u8'a'; /*\n*/\n", ".c"},
	} {
		if got := lineLabels(c.name, c.src); got != c.want {
			t.Errorf("%s %q:\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
	}
}

func TestMultilineTemplateCode(t *testing.T) {
	for _, c := range []struct {
		src    string
		labels string
		code   map[int][][2]int
	}{
		{"const s = `a ${b} c ${ {d: 1}.d } ${}`;\n", ".", map[int][][2]int{1: {{15, 16}, {22, 32}, {36, 36}}}},
		{"const s = `${\"}\"}`;\nfunction f() {}\n", "..", map[int][][2]int{1: {{13, 16}}}}, // a } in a string doesn't close it
		// a ${…} that spans lines is code to its }, nested templates aren't
		{"const t = `\n  ${e}\n  ${f(\n  g)} x\n`;\n", ".ssss", map[int][][2]int{2: {{4, 5}}, 3: {{4, 6}}, 4: {{0, 4}}}},
		{"const h = `\n  ${items.map((item) => `\n    <li>Don't ${item}</li>`).join(\"\")}\n`;\nfunction after() {}\n",
			".sss.", map[int][][2]int{2: {{4, 24}}, 3: {{16, 20}, {27, 37}}}},
	} {
		got := Multiline("a.ts", []byte(c.src))
		if l := lineLabels("a.ts", c.src); l != c.labels || !reflect.DeepEqual(got.Code, c.code) {
			t.Errorf("%q:\n got %s %v\nwant %s %v", c.src, l, got.Code, c.labels, c.code)
		}
	}
	if got := Multiline("a.go", []byte("var s = `${x}`\n")).Code; got != nil {
		t.Errorf("Go raw strings have no ${…}: %v", got)
	}
}

// "\r\n" ends a line like "\n": a backslash before it continues a string.
func TestMultilineCRLF(t *testing.T) {
	src := "x = \"abc\\\r\ndef fake():\\\r\n\"\r\n\r\ndef real():\r\n    return x\r\n"
	if got, want := lineLabels("a.py", src), ".ss..."; got != want {
		t.Errorf("labels %s, want %s", got, want)
	}
	if got, want := spans("a.py", src), "def real 5-6"; got != want {
		t.Errorf("symbols %s, want %s", got, want)
	}
}

// Parse and Multiline stay linear on long lines that keep starting regex
// literals, template ${…} or strings that don't close, and on many lines:
// 16 times the input must not take anywhere near 256 times as long.
func TestMultilineLongLines(t *testing.T) {
	for _, c := range []struct{ name, unit string }{
		{"a.js", "x=(/[a"},
		{"a.js", "a = b ? 1 / 2 : (/["},
		{"a.js", "a=`${\"{\"}x`; "},
		{"a.js", "a=`${b"},
		{"a.cpp", "1'2"},
		{"a.sh", "a=$((b<<C)) "},
		{"a.rs", "r#"},
		{"a.lua", "[=="},
	} {
		small := strings.Repeat(c.unit, 500)
		big := strings.Repeat(c.unit, 8000)
		ts, tb := fastest(c.name, small), fastest(c.name, big)
		if tb > 64*ts+5*time.Millisecond {
			t.Errorf("%s %q: 500 repeats took %v, 8000 took %v", c.name, c.unit, ts, tb)
		}
	}
	// many lines: heredocs without an end, siblings in one block
	for _, c := range []struct{ name, head, line, tail string }{
		{"a.sh", "", "f%d() { cat <<T%d; }\n", ""},
		{"a.rb", "", "def f%d; x <<T%d; end\n", ""},
		{"a.js", "class A {\n", "  m%d() {\n    return %d;\n  }\n", "}\n"},
	} {
		lines := func(n int) string {
			var b strings.Builder
			b.WriteString(c.head)
			for i := range n {
				fmt.Fprintf(&b, c.line, i, i)
			}
			b.WriteString(c.tail)
			return b.String()
		}
		ts, tb := fastest(c.name, lines(500)), fastest(c.name, lines(8000))
		if tb > 64*ts+5*time.Millisecond {
			t.Errorf("%s %q: 500 lines took %v, 8000 took %v", c.name, c.line, ts, tb)
		}
	}
}

// fastest is the shortest of a few runs of Parse and Multiline over src.
func fastest(name, src string) time.Duration {
	best := time.Duration(1 << 62)
	for range 3 {
		start := time.Now()
		Parse(name, []byte(src), 1<<20)
		Multiline(name, []byte(src))
		best = min(best, time.Since(start))
	}
	return best
}

// A definition inside a string or comment that spans lines isn't one,
// and a dedented line of a string belongs to the block it is in.
func TestParseSkipsText(t *testing.T) {
	src := `"""Run the tool.

Usage:
    def run():
        ...
"""


def run():
    return 1


class Runner:
    def start(self):
        doc = '''
def not_a_method():
    pass
'''
        return doc

    def stop(self):
        return None


def run():
    return 2
`
	if got, want := spans("tool.py", src), "def run 9-10; class Runner 13-22; def Runner.start 14-19; def Runner.stop 21-22; def run 25-26"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// Past the limit Parse still scans the strings, for the ends of the blocks
// it found.
func TestParseLimitKeepsEnds(t *testing.T) {
	src := "def f():\n    s = '''\nnot indented\n'''\n    return s\n\ndef g():\n    pass\n"
	syms := Parse("a.py", []byte(src), 1)
	if len(syms) != 1 || syms[0].End != 5 {
		t.Errorf("got %+v, want f ending on line 5", syms)
	}
}
