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

## Phase 2: JSON

- `prism/report`: envelope, Symbol, Location, Link, Next, Error, schema `prism/1`, golden tests.
- `--json` on seek, recap and crux. glint's `--json` moves into the envelope (`result.items`), with a note in its release notes.
- JSON error output with candidates for ambiguity and no-match.

Done when: every tool's JSON for the acceptance scenario validates against the golden shapes, and refs inside it round-trip.

## Phase 3: context and evidence

- `prism/changes`: uncommitted and branch changes as symbols, with the 200-file and ~50 ms limits.
- `prism/focus`: focus and last run, per worktree and session, atomic, expiring.
- `prism/evidence`: link types, basis levels, wording table.
- `prism/testcmd`: Go, pytest, cargo.
- Each tool implements its row of the table in [context](context.md#what-each-tool-does-with-it): marks, ranking, fill-in, `context:` line, up to three `next:` lines.
- `--no-context` and `PRISM_NO_CONTEXT=1`; the tools' golden tests set it.

Done when: the acceptance test below passes.

## v1 acceptance test

An automated end-to-end test in this repo. It builds a fixture Go module in a temp dir (a function `List`, a caller `run`, a test `TestList` that mentions `List`, and a commit that changes `List`), then runs the real binaries with `-A`. Each step's command is taken from the previous step's `next:` lines, never typed by the test.

1. `recap show <commit>`
   - lists `internal/files/files.go:List` as changed
   - `next:` includes `seek internal/files/files.go:List`
2. the seek suggestion
   - shows `List`'s definition (`73-…`, not including the doc comment), marked `[focus]`
   - shows the hit in `run` and the hit in `TestList`, the test labelled "mentions"
   - `next:` includes `crux go test -run '^TestList$' ./internal/files/` with the reason "runs the 1 test that mentions List"
3. the crux suggestion (with `List` broken on purpose in a second variant of the fixture)
   - reports the failure with `in internal/files/files_test.go:TestList`
   - never uses the word "covers" anywhere
4. every command again with `--json`
   - all documents parse, every ref in them resolves in every tool that takes symbols
5. the whole run again with `--no-context`
   - same results, minus marks, `context:` lines and fill-ins (proves rule 2: context never hides)

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

- **outline** doesn't recognise generic arrow functions in TypeScript (`export const card = <Message>(...) =>`), so lines inside them count as outside any definition. A parser change, so it should come with the outline diff check in CI (step 3 of the CI plan).
- **crux** doesn't parse `go vet`'s type-check errors (`vet: path:line:col: message`); they show as plain output lines, without a location or `in` line.
- **seek** has no agent skill (`skill/SKILL.md`), unlike glint, recap and crux, so agents only learn its ref mode from the README. Its README GIF still shows a range that includes the doc comment.
- **glint** had a data race in directory listing that CI's race detector caught (fixed in glint#5).

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

## Non-goals

- A "do everything" command, or a workflow that replaces the individual tools.
- A daemon, index server or cache that needs invalidating. Everything is computed per run or stored briefly in the git dir.
- Compiler- or LSP-level precision. Refs are as good as outline's fast rules, and reports say what their evidence is.
- Prism running builds or tests itself. crux does that.
