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

// What the review of those fixes found: each case was right before them.
var secondReview = []struct{ name, src, want string }{
	// an impl's header ends in more than a name: blanks, a comment, generics that go on
	{"blanks.rs", "impl Foo for Bar  {\n    fn f(&self) {}\n}\n", "impl Foo for Bar 1-3; fn Bar.f 2-2"},
	{"note.rs", "impl Foo for Bar // why\n{\n    fn f(&self) {}\n}\n", "impl Foo for Bar 1-4; fn Bar.f 3-3"},
	{"open.rs", "impl<A, B> Service<Request> for Foo<\n    A,\n    B,\n> {\n    fn f(&self) {}\n}\n", "impl Service<Request> for Foo< 1-6; fn Foo.f 5-5"},
	{"wide.rs", "impl <'state,\n      T: 'state> Drop for State<'state, T> {\n    fn drop(&mut self) {}\n}\n", "impl <'state, 1-4; fn <'state,.drop 3-3"}, // the name is the line's, as it was
	{"cgen.rs", "impl Provides<wl::Compositor, { VERSION }> for State {\n    fn bound(&self) {}\n}\n", "impl Provides<wl::Compositor, { VERSION }> for State 1-3; fn State.bound 2-2"},
	{"cgen2.rs", "impl Provides<wl::Compositor, { State::MAX }>\n    for State\n{\n    fn bound(&self) {}\n}\n\nfn after() {}\n", "impl Provides<wl::Compositor, { State::MAX }> 1-5; fn Provides.bound 4-4; fn after 7-7"},
	// impl at the start of a line is a parameter's type when a comma ends it, or when it goes on
	{"ret.rs", "fn find<'u>(\n    &'u self,\n) -> Lookup<\n    impl Continuation<\n        Output = u8,\n    >,\n> {\n    todo!()\n}\n", "fn find 1-9"},
	{"arg.rs", "pub fn expression<I, O>(\n    prefix: u8,\n    postfix:\n    impl Parser<I, Postfix<I, O>>,\n) -> u8 {\n    0\n}\n", "fn expression 1-7"},
	// a line that is indented more starts the next statement when a keyword says so,
	// or when two values meet there; never in Go
	{"odd.d.ts", "type MatchType = \"begin\" | \"end\"\n\n interface Emitter {\n    addText(text: string): void\n }\n\ninterface Mode {\n  a: string\n}\n", "type MatchType 1-1; interface Mode 7-9"},
	{"odd2.d.ts", "export interface Disposable {\n    dispose(): void;\n}\n/**\n * An event.\n */\n export interface Event<T> {\n    (listener: (e: T) => any): Disposable;\n}\n\nexport type Agent = any;\n", "interface Disposable 1-3; type Agent 11-11"},
	{"odd.ts", "const noop = () => undefined\n  doSomething({\n    a: 1,\n  })\nconst next = () => 1\n", "const noop 1-1; const next 5-5"},
	{"odd.kt", "interface A {\n    fun a(): Int\n      // sloppy\n      val x: Int\n    fun b(): Int\n}\n", "interface A 1-6; fun A.a 2-2; fun A.b 5-5"},
	{"odd.go", "package a\n\ntype A int\n\tvar x = 1\n\nfunc after() {}\n", "type A 3-3; func after 6-6"},
	// ... but not in the definition's generics, where const starts a type parameter
	{"cparam.ts", "export function encodeKeys<\n  S extends Constraint & { readonly fields: Fields },\n  const M extends { readonly [K in keyof S]?: PropertyKey }\n>(mapping: M) {\n  return mapping\n}\n\nexport const next = () => 1\n", "function encodeKeys 1-6; const next 8-8"},
	// else continues an if of the definition, not one around it, and isn't a method's name
	{"else.ts", "class CodeGen {\n  elseIf(cond: boolean): CodeGen {\n    return this._elseNode(cond)\n  }\n\n  else(): CodeGen {\n    return this._elseNode(true)\n  }\n}\n", "class CodeGen 1-9; method CodeGen.elseIf 2-4"},
	{"else.js", "if (typeof define === \"function\")\n  exports.make = function () {\n    return 1\n  }\nelse\n  exports.make = function () {\n    return 2\n  }\n", "function exports.make 2-4; function exports.make 6-8"},
	// a ? at the end of a line is half a ?: in JavaScript, wherever the other half is
	{"cond.d.ts", "type U<X> = (X extends unknown ? (d: X) => void : never) extends (m: infer I) => void ? // note\nI & X : never;\ntype Next = 1;\n", "type U 1-2; type Next 3-3"},
	{"nullish.ts", "const pick = (a?: number) => a ??\n0\nconst next = () => 1\n", "const pick 1-2; const next 3-3"},
	// a function that is called where its body ends goes on with the call
	{"iife.js", "var o = {\n  onDone: function (_onDone) {\n    return _onDone;\n  }(function () {\n    done();\n  })\n};\n", "method onDone 2-6"},
	// *[Symbol.iterator]() is a member, not a line of a comment
	{"iter.ts", "class List<E> {\n  remove(node: E): void {\n    this.size -= 1\n  }\n\n  *[Symbol.iterator](): Iterator<E> {\n    let node = this.first\n    while (node) {\n      yield node\n    }\n  }\n}\n", "class List 1-12; method List.remove 2-4"},
	// a constructor after its annotation, on the line after the class's
	{"inject.kt", "class LoginViewModel\n@Inject constructor(\n    private val repo: Repo,\n) : ViewModel() {\n    fun f() = 1\n}\n\nfun after() = 1\n", "class LoginViewModel 1-6; fun LoginViewModel.f 5-5; fun after 8-8"},
	{"named.kt", "class Api\n@Inject\n@Named(\"io\") internal constructor(val c: Client) {\n    fun f() = 1\n}\n", "class Api 1-5; fun Api.f 4-4"},
	// a comment between a header and its value doesn't end the statement
	{"note.kt", "class M {\n  private fun name(s: String): String =\n      // as the UI manager has it\n      if (s.startsWith(\"on\")) {\n        \"top\"\n      } else {\n        s\n      }\n\n  private companion object {\n    const val A = 1\n  }\n}\n", "class M 1-13; fun M.name 2-8"},
	// Scala ends a value with its line, as Kotlin does
	{"value.scala", "object M {\n    final def id(x: Int): Int = x\n    lazy val v = {\n      1\n    }\n    def run(): Unit = {}\n}\n", "object M 1-7; method M.id 2-2"},
}

func TestEndsFoundInTheSecondReview(t *testing.T) {
	for _, c := range secondReview {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// What the re-check of those fixes found.
var thirdReview = []struct{ name, src, want string }{
	// a JSX element without parentheses around it: its attributes and children aren't statements
	{"jsx.tsx", "const Row = ({ item }) =>\n  <Line\n    index={item.index}\n    text={item.text}\n  />\nconst next = () => 1\n", "const Row 1-5; const next 6-6"},
	{"frag.tsx", "const Row = () =>\n  <>\n    {a}\n    {b}\n  </>\nconst next = () => 1\n", "const Row 1-5; const next 6-6"},
	{"tag.jsx", "const Row = ({ item }) => <Line\n  index={item.index}\n  text={item.text}\n/>\nconst next = () => 1\n", "const Row 1-4; const next 5-5"},
	{"text.jsx", "const Row = () => <div\n  className=\"x\"\n>\n  text\n</div>\nconst next = () => 1\n", "const Row 1-5; const next 6-6"},
	{"attrs.tsx", "const Card = (id: string) =>\n  <Box>\n    <Orb look={look} compact theme={id} />\n    {on ? (\n      <Mark />\n    ) : null}\n  </Box>\n\nconst next = () => 1\n", "const Card 1-7; const next 9-9"},
	// ... but a type before a value starts with < too
	{"assert.ts", "const make = () => <Foo>{\n  a: 1,\n}\nrun(make())\n", "const make 1-3"},
	// ... and </script> isn't the end of one
	{"page.vue", "<script>\nconst one = () => 1\n</script>\n", "const one 2-2"},
	// a line that starts with an operator goes on wherever it is: tabs and blanks mix
	{"tabs.ts", "function outer() {\n\t\t\tconst deploy = (port?: number) =>\n        stack\n          .deploy(port)\n          .pipe(provide);\n\t\t\treturn deploy;\n}\n", "function outer 1-7; const outer.deploy 2-5"},
	// the line below a condition is its branch, whatever it starts with
	{"branch.kt", "fun check(x: Int): Int =\n    if (x < 0)\n        throw IllegalArgumentException(\"x\")\n    else\n        x\n\nfun after() = 1\n", "fun check 1-5; fun after 7-7"},
	{"branch.scala", "object M {\n    private def check(x: Int): Int =\n        if (x < 0)\n            throw new IllegalArgumentException(\"x\")\n        else\n            x\n\n    private def one(): Int = 1\n}\n", "object M 1-9; method M.check 2-6; method M.one 8-8"},
	{"cond.kt", "class A {\n    fun f(x: Int) =\n        if (x > 0) g(x)\n    fun after() = 1\n}\n", "class A 1-5; fun A.f 2-3; fun A.after 4-4"},
	{"elseif.kt", "class A {\n    fun sign(x: Int) = if (x > 0) {\n        1\n    } else if (x < 0)\n        throw X()\n    else\n        0\n\n    fun after() = 1\n}\n", "class A 1-10; fun A.sign 2-7; fun A.after 9-9"},
	{"elsebranch.kt", "class A {\n    fun pick(x: Int) = if (x > 0) {\n        1\n    } else\n        return 0\n\n    fun after() = 1\n}\n", "class A 1-8; fun A.pick 2-5; fun A.after 7-7"},
	// a value starts with await, new or !, and a declaration with a modifier
	{"await.js", "const noop = () => undefined\n  await doSomething()\nconst next = () => 1\n", "const noop 1-1; const next 3-3"},
	{"not.js", "const noop = () => undefined\n  !done && finish()\nconst next = () => 1\n", "const noop 1-1; const next 3-3"},
	{"incr.js", "const noop = () => undefined\n  ++count\nconst next = () => 1\n", "const noop 1-1; const next 3-3"},
	{"member.ts", "class Clock {\n  now(): number {\n    return Date.now()\n  }\n    readonly zone = \"utc\"\n  other = 1\n}\n", "class Clock 1-7; method Clock.now 2-4"},
	{"suspend.kt", "interface A {\n    fun a(): Int\n      // sloppy\n      suspend fun x(): Int\n    fun b(): Int\n}\n", "interface A 1-6; fun A.a 2-2; fun A.b 5-5"},
	// ... but inner( is a call, and as goes on with the value
	{"infix.kt", "fun flags() = a or\n    inner(b)\nfun after() = 1\n", "fun flags 1-2; fun after 3-3"},
	{"as.ts", "const pick = (a: unknown) => a\n  as string\nconst next = () => 1\n", "const pick 1-2; const next 3-3"},
	// ... and a type can end in abstract, which wants its new
	{"ctor.ts", "type Ctor = abstract\n  new () => object\ntype Next = 1\n", "type Ctor 1-2; type Next 3-3"},
	// a comment in a Kotlin comment has its own end
	{"kdoc.kt", "fun a(x: Int): Int = x\n\n/**\n * Strips /* block */ comments.\n * Then more text.\n */\n@Test\nfun b(): Int = 1\n", "fun a 1-1; fun b 8-8"},
	{"nonest.js", "const f = (a) => a\n/* a comment about /* */\n  .then(b)\nconst next = () => 1\n", "const f 1-3; const next 4-4"},
	// else ( is an else in Kotlin, and else = a member in JavaScript
	{"else.kt", "class A {\n    fun f(x: Int) = if (x > 0) x\n    else (x + 1)\n}\n", "class A 1-4; fun A.f 2-3"},
	{"elseprop.ts", "class Cond {\n  then(): this {\n    return this\n  }\n  else = (): this => {\n    return this\n  }\n  other = 1\n}\n", "class Cond 1-9; method Cond.then 2-4"},
	// an else further out is the else of an if around the definition
	{"outer.js", "if (typeof define === \"function\")\n  exports.make = () => {\n    return 1\n  }\nelse\n  exports.make = () => {\n    return 2\n  }\n", "function exports.make 2-4; function exports.make 6-8"},
	{"outer2.js", "if (a)\n  var f = (x) =>\n    x + 1\nelse\n  var g = 2\n", "const f 2-3"},
	// an annotation belongs to what follows it, unless that's a constructor
	{"annot.kt", "object Marker\n\n@OptIn(X::class) {\n    1\n}\n", "object Marker 1-1"},
}

func TestEndsFoundInTheThirdReview(t *testing.T) {
	for _, c := range thirdReview {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}
