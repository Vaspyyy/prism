package outline

import "testing"

// A { or a ; in a type isn't the body or the end of the definition: array
// types, interface{} and struct{}, object types, const generics.
func TestEndsAfterBracesAndSemicolonsInTypes(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a.rs", `
impl Hit {
    fn beside(&self) -> [i32; 3] {
        [1, 2, 3]
    }
    pub fn nearest(
        &self,
        at: [f64; 3],
    ) -> Option<[i32; 3]> {
        None
    }
}

impl From<[u8; 4]> for Id {
    fn from(b: [u8; 4]) -> Self {
        Id(b)
    }
}

static TABLE: [Entry; 2] = [
    Entry { a: 1 },
    Entry { a: 2 },
];

fn lits(&self) -> ArrayVec<Lit, { Size::MAX }> {
    todo!()
}

type Hash = [u8; 32];

fn after() {}
`, "impl Hit 1-11; fn Hit.beside 2-4; fn Hit.nearest 5-10; impl From<[u8; 4]> for Id 13-17; fn Id.from 14-16; " +
			"static TABLE 19-22; fn lits 24-26; type Hash 28-28; fn after 30-30"},
		{"a.go", `
package a

func Any() interface{} {
	return nil
}

func (s *Set) Keys() map[string]struct{} {
	return s.m
}

func Min[T interface{ ~int; String() string }](x, y T) T {
	return x
}

type Pair[K interface{ comparable }, V any] struct {
	k K
	v V
}

func Done() <-chan struct{} { return nil }

func after() {}
`, "func Any 3-5; func Set.Keys 7-9; func Min 11-13; type Pair 15-18; func Done 20-20; func after 22-22"},
		{"a.ts", `
function split(minutes: number): { amount: number; unit: string } {
  return { amount: minutes, unit: "m" };
}

const boot = async (name: string): Promise<{ sample?: Sample; failure?: string }> => {
  return {};
};

export function parse(
  line: string,
): { ok: boolean } | null {
  return null;
}

const App: Component<{ children: Element }> = (props) => {
  return props.children;
};

export class Store<T extends { id: string }> extends Base<T> {
  get(id: string): T | undefined {
    return undefined;
  }
}

function pair(a: string): [{ a: string }, { b: number }] {
  return [{ a }, { b: 1 }];
}

function after() {}
`, "function split 1-3; const boot 5-7; function parse 9-13; const App 15-17; class Store 19-23; method Store.get 20-22; " +
			"function pair 25-27; function after 29-29"},
		{"a.kt", `
class Seasons {
    fun length(): Int = try {
        config.length
    } catch (e: Exception) {
        28
    }

    fun rank(n: Int) = if (n > 9) {
        "high"
    } else if (n > 4) {
        "mid"
    } else {
        "low"
    }

    fun ghosts(): List<Line> = slots.filter { it.left >= 0f }.map { (id, slot) ->
        Line(id, slot)
    }
}
`, "class Seasons 1-19; fun Seasons.length 2-6; fun Seasons.rank 8-14; fun Seasons.ghosts 16-18"},
	}
	for _, c := range cases {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// What closes a block on its line and then opens another is only the same
// definition's when nothing starts in between.
func TestEndsBeforeTheNextDefinitionOnTheLine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a.js", "function a(){return 1}function b(){\n\treturn 2\n}\n", "function a 1-1"},
		{"b.js", "const f=()=>{return 1},g=()=>{\n\treturn 2\n}\n", "const f 1-1"},
		{"a.go", "package a\n\ntype T struct{}; func (T) M() {\n}\n", "type T 3-3"},
		{"a.cpp", "struct A { int x; }; struct B {\n\tint y;\n};\n", "struct A 1-1"},
	}
	for _, c := range cases {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// A definition whose value is an expression ends where the expression
// does, also without a semicolon: a block further down belongs to what
// follows.
func TestEndsWithTheValue(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a.ts", `
export const abs = (n: bigint): bigint => (n < zero ? -n : n)

export const gcd: {
  (a: bigint): bigint
} = dual(2, f)

const platform = (c: Compression): Context<Platform> =>
  Context.make(
    Platform,
    { c },
  )

export type ToolError =
  | NotFound
  | Rejected

const next = () => count++

if (ready) {
  start()
}

const isBody = (body: unknown): boolean =>
  isObject(body) &&
  ("data" in body || "errors" in body)

function after() {}
`, "const abs 1-1; const platform 7-11; type ToolError 13-15; const next 17-17; const isBody 23-25; function after 27-27"},
		{"a.kt", `
class Board(val id: String) {
    override fun getSerializedName(): String = id

    fun able(p: Person): Boolean =
        p.isAlive && !p.isRemoved &&
            p.carried.isEmpty

    private class Icon(val width: Int, val height: Int)

    fun wet(x: Int, z: Int): Boolean

    @Volatile
    private var seen = 0

    companion object {
        val CODEC = build {
            it.stable()
        }
    }
}
`, "class Board 1-20; fun Board.getSerializedName 2-2; fun Board.able 4-6; class Board.Icon 8-8; fun Board.wet 10-10"},
		{"a.go", `
package a

type extractor func(s *state, i int) int

var extractors = []extractor{
	rust,
}

type Usage int

const (
	Any Usage = iota
)

func chancap(c any) int

var barrier struct {
	enabled bool
}

type F func(
	a int,
) error

func after() {}
`, "type extractor 3-3; type Usage 9-9; func chancap 15-15; type F 21-23; func after 25-25"},
	}
	for _, c := range cases {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// The header of a definition can go on after a line that ends like a
// statement: the block on a line of its own, a where clause, a constructor.
func TestHeaderGoesOnTheNextLine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a.kt", `
class Repo
@Inject
constructor(private val db: Db) : Source {
    fun <T> first(xs: List<T>): T
        where T : Comparable<T> {
        return xs.first()
    }

    fun name(): String
        = db.name
}

class Plain
{
    fun run() {}
}

class Hidden
    private constructor(val id: Int) {
    fun show() {}
}
`, "class Repo 1-11; fun Repo.first 4-7; fun Repo.name 9-10; class Plain 13-16; fun Plain.run 15-15; class Hidden 18-21; fun Hidden.show 20-20"},
		{"a.js", `
worker.onmessage = function (evt)
{
	handle(evt);
};

function after() {}
`, "function worker.onmessage 1-4; function after 6-6"},
	}
	for _, c := range cases {
		if got := spans(c.name, c.src); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// Parentheses around an arrow function, or around the object it returns,
// aren't a parameter list: the braces in them are the body. JSX in
// parentheses and the parameters of a second arrow function still aren't.
func TestParenthesesAroundTheValue(t *testing.T) {
	src := `
const ring = (cx: number, color: string): Shape => ({
  color,
  compile: (grid) => {
    grid.set(cx);
  },
});

const models = (() => {
  const out = [];
  return out;
})();

export const mocks = {
  search: ((input: unknown) => {
    state.search(input);
  }) as never,
};

const View = () => (
  <div style={{ margin: 0 }}>
    {name}
  </div>
);

const add = (a: number) => ({ b }: { b: number }) =>
  a + b;

function after() {}
`
	want := "const ring 1-6; method ring.compile 3-5; const models 8-11; method search 14-16; const View 19-23; const add 25-26; function after 28-28"
	if got := spans("a.tsx", src); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
