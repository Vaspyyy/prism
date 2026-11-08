# Evidence: what a link may claim

When results build on each other, it's easy to suggest more than is known. A test that *mentions* `List` is not a test that *covers* the change to `List`. An agent that reads "covers" stops looking; an agent that reads "mentions" knows to check.

So every relation between two results is a **link** that records its **basis**, and the words a report may use come from the basis, not from whoever wrote the message.

Owned by `prism/evidence`.

## Links

```json
{ "from": "internal/files/files_test.go:TestList", "to": "internal/files/files.go:List",
  "rel": "mentions", "basis": "reference", "at": "internal/files/files_test.go:12" }
```

| Field | Meaning |
| --- | --- |
| `from`, `to` | refs |
| `rel` | the relation: `mentions`, `contains`, `ran`, `covers`, `named-like` |
| `basis` | how it is known, below |
| `at` | where the evidence is, a location ref (optional) |

## Basis levels

From weakest to strongest:

| Basis | How it's known | Example | Allowed wording |
| --- | --- | --- | --- |
| `name` | naming convention only | `TestList` or `files_test.go` next to `List` / `files.go` | "may test", "named like" |
| `reference` | the name appears as a whole word inside the other symbol (seek) | `TestList` calls `List` on line 12 (a name only inside a longer word, like `TestHugeWordList`, doesn't count) | "mentions", "uses (by name)" |
| `location` | a line lies inside the symbol's range (outline) | crux failure at `group.go:312` lies in `multiline` | "in" |
| `execution` | the test actually ran and crux saw its result | `TestList` failed | "ran", "failed", "passed" |
| `coverage` | coverage data shows the test executed lines of the symbol | coverprofile hits in `files.go:73-107` | "covers", "executes" |

Notes:

- `reference` is textual. A hit on `Open` can be a different `Open`. Call-shaped hits (`List(`) are still `reference`: seek shows the hit text, so the reader can judge.
- `location` is only as good as outline, which uses fast per-language rules, not a compiler. It's reliable for well-formatted code and can be wrong on unusual formatting.
- `execution` says the test ran. It says nothing about which code the test touched.

## Rules

1. **Wording comes from the basis.** Reports, `next:` reasons and JSON `rel` only use the words allowed for the link's basis.
2. **Never upgrade.** No combination of weaker bases makes a stronger one. A test that is named like `List`, mentions `List` and ran is still not "covers List".
3. **Absence is information, phrased carefully.** "No test mentions `List`" is a useful, true statement. "`List` is untested" is not, unless coverage data for the full test suite says so.
4. **Say which tests were chosen, and why.** A suggested crux command states its basis: "runs the 2 tests that mention List", not "runs the tests for List".

## crux failures and symbols

crux gets one small, honest link in v1. For each error or failure with a location:

1. Parse the file with outline and find the innermost symbol containing the line: `loc.in`, basis `location`.
2. If that symbol is in the derived or remembered context, mark it: `[changed]`, `[branch]` or `[focus]`.
3. For a failing test, also give the test's own ref (`test`), resolved from the runner's output: Go `TestX` plus the package directory, pytest `path::Class::test`, cargo `module::tests::name`.

Text example:

```
FAIL TestList  internal/files/files_test.go:31
  in internal/files/files_test.go:TestList
  got 2 files, want 3
error internal/group/group.go:312:9  undefined: spans
  in internal/group/group.go:multiline [changed]
```

The second entry is a connection backed by evidence: this error is inside a symbol you changed.

## Suggesting tests: `prism/testcmd`

seek's `next:` line can offer the crux command that runs the tests mentioning a symbol. `prism/testcmd` builds it from test refs:

| Ecosystem | Command | Precision |
| --- | --- | --- |
| Go | `go test -run '^(TestA\|TestB)$' ./pkg/dir/` | exact for one package. `-run` applies to every listed package, so with several, a test of the same name in another listed package runs too: checked, and the reason line says so |
| Python (pytest) | `pytest path/test_x.py::test_a path/test_x.py::TestB::test_c` | exact |
| Rust (cargo) | `cargo test -- --exact mod::tests::name`, with `--test x` for an integration test target | exact within a target. A crate with both `src/lib.rs` and `src/main.rs` can have the same test path in both, and the reason line says so |

Paths are relative to the directory the tool ran in, so the command runs as printed. A Go module or Cargo package elsewhere in the repo gets `go -C <dir>` or `--manifest-path <dir>/Cargo.toml`. What counts as a test: Go `TestXxx`, `FuzzXxx` and `ExampleXxx` functions in `_test.go` files; pytest's default discovery (`test_*` functions and methods in `Test*` classes, in `test_*.py` or `*_test.py`); Rust functions marked `#[test]` or `#[<crate>::test]`.

Other ecosystems: no suggestion until a precise command is known. A wrong suggestion is worse than none.

These tests have basis `reference` until crux runs them (`execution`). They never become "covers" without coverage data.

## Later: coverage

After v1, crux can collect coverage for the runners that support it cheaply (`go test -coverprofile`, `pytest --cov`, `cargo llvm-cov`) and map covered lines to symbols with outline. Only that produces `coverage` links and the word "covers". Until then, no tool says "covers".
