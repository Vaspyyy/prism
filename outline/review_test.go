package outline

import (
	"strings"
	"testing"
)

// What the review of the range rules found: each case went wrong once.
func TestEndsFoundInReview(t *testing.T) {
	long := strings.Repeat("    1, 2, 3, 4,\n", 40)
	cases := []struct{ name, src, want string }{
		// a ? or a ! ends a line more often than it goes on
		{"null.kt", "interface Dao {\n    suspend fun byId(id: Int): Entity?\n\n    @Insert\n    suspend fun insert(e: Entity)\n}\n",
			"interface Dao 1-6; fun Dao.byId 2-2; fun Dao.insert 5-5"},
		{"bang.kt", "class V {\n    fun view(): View = binding!!\n\n    private val head = 1\n    fun cur(): Fragment? = x as Fragment?\n    val a = 1\n}\n",
			"class V 1-7; fun V.view 2-2; fun V.cur 5-5"},
		{"bang.ts", "const root = () => document.getElementById(\"root\")!\n\nconst app = createApp({\n  a: 1,\n})\n", "const root 1-1"},
		// the word constructor isn't a header's unless the header is a class's
		{"ctor.ts", "const g = () => 10\nconst a = make({ constructor: g })\nconst b = 2\n", "const g 1-1"},
		{"ctor.kt", "class A {\n    fun one() = 1\n    constructor(x: Int) : this()\n}\n", "class A 1-4; fun A.one 2-2"},
		// a value goes on while its lines are indented, whatever they start or end with
		{"cmp.ts", "const big = (a: number[], b: number[]) =>\n  a.length >\n  b.length;\n\nconst next = () => 1;\n", "const big 1-3; const next 5-5"},
		{"asi.ts", "const big = (a: number[], b: number[]) =>\n  a.length\n  === b.length\n\nconst next = () => 1\n", "const big 1-3; const next 5-5"},
		{"if.kt", "fun f(x: Int): Int =\n    if (x > 0)\n        1\n    else\n        2\n\nfun g() = 1\n", "fun f 1-5; fun g 7-7"},
		{"or.kt", "fun bits(a: Int, b: Int): Int = a or\n    b\n\nfun g() = 1\n", "fun bits 1-2; fun g 4-4"},
		// where a semicolon is required, a value ends at it
		{"and.cs", "class C {\n    public bool Has(string s) => s is not null\n        and not \"\";\n    public int N() => 1;\n}\n", "class C 1-5; method C.Has 2-3; method C.N 4-4"},
		{"where.rs", "impl Lend for X {\n    type Item<'a> = &'a T\n    where\n        Self: 'a;\n    fn f() {}\n}\n", "impl Lend for X 1-6; type X.Item 2-4; fn X.f 5-5"},
		{"mul.rs", "static AREA: u32 = (WIDTH + PAD)\n    * HEIGHT;\n\nfn after() {}\n", "static AREA 1-2; fn after 4-4"},
		{"abi.rs", "impl IntoWasmAbi for X {\n    type Abi = <\n        Box<[JsValue]>\n        as IntoWasmAbi\n    >::Abi;\n}\n", "impl IntoWasmAbi for X 1-6; type X.Abi 2-5"},
		// braces in a table's brackets, before its first string
		{"table.rs", "static ITEMS: &[Item] = &[\n    Item { name: \"a\", n: 1 },\n    Item { name: \"b\", n: 2 },\n    Item { name: \"c\", n: 3 },\n];\n\nfn after() {}\n", "static ITEMS 1-5; fn after 7-7"},
		{"cols.ts", "const cols = () => [\n  { key: \"a\" },\n  { key: \"b\" }\n]\n\nconst next = () => 1\n", "const cols 1-4; const next 6-6"},
		// a table, or a call, of any length is the definition's while it's in its brackets
		{"long.rs", "static T: &[u8] = &[\n" + long + "];\n\nfn after() {}\n", "static T 1-42; fn after 44-44"},
		{"gen.ts", "export const make = (x: number) =>\n  Effect.gen(function* () {\n    const inner = (y: number) => y + 1;\n    return inner(x);\n  });\n\nexport const next = () => 1;\n",
			"const make 1-5; const make.inner 3-3; const next 7-7"},
		{"svc.ts", "export class Svc extends Effect.Service<Svc>()(\"Svc\", {\n  effect: Effect.gen(function* () {\n    function helper() {\n      return 1;\n    }\n    return { helper };\n  }),\n}) {}\n\nexport const next = () => 1;\n",
			"class Svc 1-8; function Svc.helper 3-5; const next 10-10"},
		// a function's body is the last of it, whatever the line goes on with
		{"two.rs", "fn a() {} impl<T> Foo<T> {\n    fn inner(&self) {}\n}\n", "fn a 1-1; fn inner 2-2"},
		{"two.js", "function a(){return 1}b.c=function(){\n\treturn 2\n}\nfunction Foo() {} Foo.prototype.bar = function () {\n  return 1\n}\n", "function a 1-1; function Foo 4-4"},
		{"two.cpp", "void a(void) {} namespace {\n\tint y;\n}\n", "func a 1-1"},
		{"spread.ts", "const o = {\n  validate: (n) => {\n    check(n)\n  },\n  ...options\n}\n", "method validate 2-4"},
		{"arg.ts", "Object.assign(\n  function sql(strings: unknown): any {\n    return 1\n  },\n  {\n    unsafe() {\n      return 2\n    },\n  },\n)\n", "function sql 2-4; method unsafe 6-8"},
		// a block in a type isn't the body, also when the header goes on below it
		{"upsert.ts", "export function upsert<T extends { id: string }>(\n  items: T[],\n): T[] {\n  return items;\n}\n\nexport function decode(\n  raw: string,\n): Result<\n  { readonly n: number },\n  Failure\n> {\n  return parse(raw);\n}\n\nexport function pick<\n  T extends { readonly id: string },\n>(xs: T[]): T {\n  return xs[0];\n}\n",
			"function upsert 1-5; function decode 7-14; function pick 16-20"},
		{"union.ts", "type Action =\n  | { type: \"a\" }\n  | { type: \"b\" };\n\nexport type Vpc = Resource<\n  \"AWS.Vpc\",\n  {\n    id: string;\n  },\n  never\n>;\n\nexport class Svc extends Context.Service<\n  Svc,\n  { readonly n: number }\n>()(\"app/Svc\") {}\n\nconst load = (id: string): Promise<{ ok: boolean }> =>\n  fetch(id);\n\nexport function g1<T extends { id: string }>(input: {\n  items: T[];\n}): readonly T[] {\n  return input.items;\n}\n",
			"type Action 1-3; type Vpc 5-11; class Svc 13-16; const load 18-19; function g1 21-25"},
		{"chain.kt", "fun names(xs: List<User>) = xs\n    .filter { it.active }\n    .map { it.name }\n\nfun g() = 1\n", "fun names 1-3; fun g 5-5"},
		{"elseif.rs", "static SEL: u8 = if cfg!(a) {\n    1\n} else if cfg!(b) {\n    2\n} else {\n    3\n};\n\nfn after() {}\n", "static SEL 1-7; fn after 9-9"},
		{"const.rs", "fn f<const N: usize>() -> Foo<{ N + 1 }>\nwhere\n    T: X,\n{\n    todo!()\n}\n\nfn o() -> Vec<{ N }> where [(); { N }]: Sized {\n    todo!()\n}\n\nfn w<const N: usize>() -> Foo<{ N + 1 }>\nwhere\n    Foo<{ N + 1 }>: Sized,\n{\n    todo!()\n}\n", "fn f 1-6; fn o 8-10; fn w 12-17"},
		// a comment between two definitions isn't the first one's, however it's indented
		{"doc.ts", "export function isOn(value: string | null): boolean {\n  return value === \"true\";\n}\n\n/**\n * Mirrors the gate, so the descriptor\n    never advertises what would be skipped.\n */\nexport function read(\n  secrets: Store,\n) {\n  return secrets.get(\"x\");\n}\n", "function isOn 1-3; function read 9-13"},
		// a declaration in a macro isn't an impl
		{"macro.rs", "impl_mul! {\n    impl Mul<BigInt> for BigInt;\n    impl Mul<[u8; 4]> for BigInt;\n}\n\nfn after() {}\n", "fn after 6-6"},
	}
	for _, c := range cases {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// The signature is the line up to the block that goes on, and up to the
// first block when the definition ends on its line.
func TestSignatureOfOneLine(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a.ts", "type U2 = { a: 1 } | { b: 2 }\n", "type U2 ="},
		{"a.kt", "fun rank(n: Int) = if (n > 9) { \"high\" } else { \"low\" }\n", "fun rank(n: Int) = if (n > 9)"},
		{"a.go", "package a\n\nfunc Any() interface{} {\n\treturn nil\n}\n", "func Any() interface{}"},
	} {
		if syms := Parse(c.name, []byte(c.src), 10); len(syms) != 1 || syms[0].Sig != c.want {
			t.Errorf("%s: %+v, want the signature %q", c.name, syms, c.want)
		}
	}
}
