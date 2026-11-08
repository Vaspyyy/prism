package outline

import (
	"reflect"
	"strings"
	"testing"
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
	} {
		if got := lineLabels(c.name, c.src); got != c.want {
			t.Errorf("%s %q:\n got %s\nwant %s", c.name, c.src, got, c.want)
		}
	}
}

func TestMultilineTemplateCode(t *testing.T) {
	src := "const s = `a ${b} c ${ {d: 1}.d }`;\nconst t = `\n  ${e}\n  ${f\n`;\n"
	got := Multiline("a.ts", []byte(src)).Code
	want := map[int][][2]int{1: {{15, 16}, {22, 32}}, 3: {{4, 5}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := Multiline("a.go", []byte("var s = `${x}`\n")).Code; got != nil {
		t.Errorf("Go raw strings have no ${…}: %v", got)
	}
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
