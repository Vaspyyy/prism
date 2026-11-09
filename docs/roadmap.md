# Roadmap

## Order

1. **Identifiers** (refs)
2. **Structured output** (`--json`)
3. **Shared context** (derived context, focus, marks, `next:` lines, evidence)

Identifiers come first because everything else is built from them: JSON without stable refs is just text in braces, and context built before refs are settled would build in today's inconsistencies. Once every output carries refs that every tool accepts, most of "carry the selection forward" already works through `next:` lines, which keeps the context layer small.

**v1 = phases 0 to 3.** v1 has no `prism` binary: everything ships inside the four tools through the library.

## Phase 0: the core module (done 2026-11-08)

- Create `github.com/Vaspyyy/prism` (Go, stdlib only, GPL-3.0).
- Move `glint/outline` to `prism/outline`, history included (`git filter-repo` or a subtree split, so blame survives).
- glint imports `prism/outline`. `glint/outline` stays for one minor release as a forwarding package (type alias plus wrappers) marked deprecated, then goes away.
- seek and recap switch their import. From now on all three use one parser version.
- `prism/repo`: root, worktree, git dir, HEAD, branch, default branch, path conversion (repo-relative out; repo-, cwd-relative, absolute and suffix in).

Done when: glint, seek and recap build against `prism` and their existing tests pass unchanged.

## Phase 1: refs (done 2026-11-08)

- `prism/ref`: parse, format, match, as in [refs](refs.md), with the round-trip test.
- **seek**
  - Reports definition ranges from the defining line (`files.go:73-107`); doc comments stay visible with `-s`.
  - Prints full refs where it prints a definition outside a file group (`seek -d`).
  - Accepts a ref as PATTERN: if PATTERN parses as a ref whose path exists, `seek REF` prints the definition (as `-s` would), then the hits of its name elsewhere grouped by containing definition (its callers and mentions). Otherwise PATTERN is a regex, as today; `-F` or `-e` force that reading.
  - Accepts location refs (`main.go:104`): searches for the enclosing symbol.
- **recap**
  - `--show` uses `prism/ref`: accepts location refs (`group.go:312`), `@rev` and `#N`.
  - Grouped output already satisfies the concatenation rule. Check it for impl members and nested symbols.
- **glint**: `--outline` shows the qualified name when the signature doesn't show it as-is.
- **crux**: imports `prism/outline`, adds `in <ref>` to every error and failure with a location (basis `location`), and the failing test's ref.
- Docs: each tool's README and `skill/SKILL.md` get one shared paragraph on refs (the same text everywhere, maintained in this repo).

Done when: the cross-tool test passes (every ref printed by any tool in the acceptance scenario resolves in every other tool that takes symbols).

## Phase 2: JSON (done 2026-11-08)

- `prism/report`: envelope, Symbol, Location, Link, Next, Error, schema `prism/1`, golden tests.
- `--json` on seek, recap and crux. glint's `--json` moves into the envelope (`result.items`), with a note in its release notes.
- JSON error output with candidates for ambiguity and no-match.

Done when: every tool's JSON for the acceptance scenario validates against the golden shapes, and refs inside it round-trip.

## Phase 3: context and evidence (done 2026-11-08)

- `prism/changes`: uncommitted and branch changes as symbols, with the 200-file and ~50 ms limits.
- `prism/focus`: focus and last run, per worktree and session, atomic, expiring.
- `prism/evidence`: link types, basis levels, wording table.
- `prism/testcmd`: Go, pytest, cargo.
- Each tool implements its row of the table in [context](context.md#what-each-tool-does-with-it): marks, ranking, fill-in, `context:` line, up to three `next:` lines.
- `--no-context` and `PRISM_NO_CONTEXT=1`; the tools' golden tests set it.

Done when: the acceptance test below passes.

## v1 acceptance test

`TestAcceptance` in `e2e/acceptance_test.go`, run in CI against all four tools built from this checkout. It builds a fixture Go module in a temp dir (a doc-commented function `List`, a caller `run`, a test `TestList` that calls `List`, and a commit that changes `List` and breaks `TestList`), then runs the real binaries in one agent session. Each step's command is taken from the previous step's `next:` lines, never typed by the test.

1. `recap show <commit>`
   - lists `internal/files/files.go:List` as changed, and records the commit as focus
   - `next:` includes `seek internal/files/files.go:List`
2. the seek suggestion
   - shows `List`'s definition from its `func` line (not the doc comment), marked `[focus]`, under a `context:` line
   - shows the hit in `run` and the hit in `TestList`, the test labelled "mentions List"
   - `next:` includes `crux go test -run '^TestList$' ./internal/files/` with the reason "runs the 1 test that mentions List"
3. the crux suggestion
   - reports the failure with `in internal/files/files_test.go:TestList`, and records the run
   - no output anywhere says "covers", "covered" or "coverage"
4. every command again with `--json`
   - all documents are valid envelopes with `context`, their links pass `evidence.CheckAll`, their `next` items are the text run's, and every symbol ref in them is accepted by seek
5. the whole run again with `--no-context` added to every command
   - the same reports minus marks and the `context:` line, and no `next:` line that context-on didn't print (context may add suggestions, never remove one; proves rule 2: context never hides)

## Per-tool change list

| Tool | Phase 0 | Phase 1 | Phase 2 | Phase 3 |
| --- | --- | --- | --- | --- |
| glint | outline moves out, forwarding shim | qualified names in `--outline` | envelope for `--json` | mark changed files and symbols |
| seek | switch import | ranges, full refs, ref as PATTERN, location refs | `--json` | rank and mark, crux suggestions |
| recap | switch import | `--show` via `prism/ref` | `--json` | write focus, fill from focus, seek suggestions |
| crux | (none) | `in <ref>`, test refs | `--json` | write last run, mark failures, recap/seek suggestions |

## After v1

- **`prism` command**: `prism` with no arguments prints a short "where am I" card for an agent starting work: repo, branch, changed symbols, focus, last crux verdict mapped to symbols, and `next:` lines. Read-only; it composes the library, it doesn't wrap the tools.
- **Coverage evidence** in crux (see [evidence](evidence.md#later-coverage)).
- **recap shows callers and test mentions per changed symbol.** This needs seek's search in the core library; decide after v1 whether it earns that.
- More `testcmd` ecosystems (vitest, jest, JUnit) once a precise command is known for each.

## Found along the way

Things the work so far turned up that belong to a later step:

- **outline** doesn't recognise generic arrow functions in TypeScript (`export const card = <Message>(...) =>`), so lines inside them count as outside any definition. The golden corpus (`outline/testdata/corpus`) now shows the effect of such a change.
- **outline** doesn't report a Rust function written on one line with its attribute (`#[test] fn x() {}`), so crux and seek can't name such a test. testcmd works around it for its same-path check by splitting the line first.
- **outline**: a lone backtick in JSX text or Vue/Svelte markup still pairs with the next real template string's backtick. A lexer can't tell markup text from a template; only a backtick that nothing closes is ignored.
- **crux** doesn't parse `go vet`'s type-check errors (`vet: path:line:col: message`); they show as plain output lines, without a location or `in` line.
- **crux reruns**: a Go test that failed in its own body reruns its passing subtests without a caveat (testcmd only knows the subtests go printed). crux counts a parent test that only printed `t.Log` output as failing on its own, because go's plain output can't tell a log from an error; the rerun then takes the whole parent, with a caveat. A pytest rerun of a parametrized rerun gives no further rerun line.
- **testcmd** is over-cautious with cargo: a crate whose sources start an item with `#[path` or `include!(` anywhere, test fixtures included, gets no command, and `--workspace` caveats count every package under the workspace root.
- **seek** has no agent skill (`skill/SKILL.md`), unlike glint, recap and crux, so agents only learn its ref mode from the README. Its README GIF still shows a range that includes the doc comment.
- **report** gaps left after Phase 2: a moved definition in a new file can't say its signature changed (`RecapDef` has no `old_sig`), and `last_commit.commits` in glint is 1 unless `-H` asked for the count (it should be left out instead). seek passes a file kind `long lines` that the docs don't list. recap's note on special files is only in `notes`; `RecapFile` has no field for it.
- **glint**: `-A -M` orders children of equal size differently between runs, and files over 4 MiB show 0 lines (line counting stops at that size). In workspace maps, a loose folder too big to open counts its glance line twice against the budget.
- **crux** mention suggestions can still match a function name shared across packages (two `New`s) on text alone; ruling that out needs a repo-wide index.
- **outline** takes a call that ends its line with `{` for a method in JavaScript and TypeScript (`runPromise(Effect.gen(function*() {` becomes a method `runPromise`), because the method rule only looks at the shape of the line.
- **outline** still gives one line to a definition whose block is inside a call's parentheses (`static TABLE: LazyLock<T> = LazyLock::new(|| {`, `class Account extends Aggregate("Account", {`), and to a Go type whose type parameters hold a struct or interface that spans lines with no block after it (`type T[P struct {…}] int`).

Fixed since v1:

- **Ranges ended too early or too late where a type or a value has braces or a `;` in it.** Found by outlining 370,000 definitions of real projects before and after (Prism 0.9.0):
  - a `;` in an array type ended a Rust function on its first line (`fn f() -> [i32; 3] {`: 1.5% of the functions in a large Rust project), and `impl Codec<[u8; 4]> for X` wasn't a definition;
  - a `{…}` in a type was taken for the body: `func f() interface{} {`, `map[string]struct{}`, `function f(): { ok: boolean } {`, `Promise<{ … }> => {`, `<T extends { id: string }>`, `-> Vec<{ N }>`. The signature stopped at that brace too (`export function View(props:`);
  - a value without a block and without a semicolon (`fun name(): String = id`, `const abs = (n) => (n < 0 ? -n : n)`, `type ID int`) ran on into the next block that wasn't a definition, such as Kotlin's `companion object {`, and became the parent of what was in it;
  - a block that closes and opens again on one line ended the definition (`} catch (e: E) {`, `} else {` in a Kotlin function with an expression body);
  - parentheses around an arrow function or the object it returns hid the body (`= (() => {`, `=> ({`).

- **Ordinals differed between seek and the other tools** when a definition sat inside a docstring or template string. outline now drops definitions inside text that spans lines, and `outline.Multiline` gives seek the same classification (Prism 0.8.0, seek 0.5.0).
- **crux had no "rerun only the failed tests" suggestion.** `testcmd.Rerun` keeps the original command's flags and reruns exactly what failed, Go subtests included (Prism 0.8.0 and 0.8.1, crux 0.7.0).
- **recap hung on a symlink to a FIFO or device.** It now reads symlinks as git stores them and leaves special files out, and `outline.Read` never follows a symlink or blocks (Prism 0.8.0, recap 0.4.1).
- **glint didn't mark repositories nested in a folder it lists** (`glint -A ~/Projekte`). It now loads scoped, lock-free context for the projects the map opens up (glint 0.10.0).
- **glint** had a data race in directory listing that CI's race detector caught (glint#5).
- **v0.6.3** of Prism was tagged on the wrong commit (the same code as v0.6.2) and is retracted in go.mod; v0.6.4 is that release.

## Decisions taken

Each can be revisited; the reason is noted so it's clear what would change it.

| Decision | Why |
| --- | --- |
| outline moves into prism | one parser version for all tools; prism is the real core and glint becomes one of the tools |
| ranges start at the defining line; doc comments separate | matches compilers, editors and crux locations, and glint and recap already do it |
| revision as `@rev` suffix, duplicates as `#N` | keeps the common case (`path:Name`) short; both are rare |
| one JSON document per run, `snake_case`, `prism/1` | easiest to consume; budgets bound the size |
| context on by default, rank/mark/fill only | autonomy without hidden filtering |
| remembered context per worktree and session, 60 min expiry | parallel agents and worktree-based fix work |
| no `prism` binary in v1 | the value is in the tools connecting, not in a new entry point |

## Open questions

- **Expiry time.** 60 minutes is a guess. Revisit after a week of real use.
- **Session variables for other harnesses** (Codex, opencode). Add each one once it's seen on a real run.
- **One last run per worktree and session.** A passing `crux go vet` replaces the record of a failed `crux go test`, so the `[failing]` marks go away although the tests weren't rerun. That's the spec as written; keeping one record per command kind may be better.
- **What the `context:` line counts.** It counts every changed symbol in the repo ("68 changed symbols vs HEAD") even when the report shows one of them, or is a map of files. Counting only what's shown may read better.

## Non-goals

- A "do everything" command, or a workflow that replaces the individual tools.
- A daemon, index server or cache that needs invalidating. Everything is computed per run or stored briefly in the git dir.
- Compiler- or LSP-level precision. Refs are as good as outline's fast rules, and reports say what their evidence is.
- Prism running builds or tests itself. crux does that.
