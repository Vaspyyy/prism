# Refs: one identifier for every tool

A **ref** names a file, a symbol in a file, or a location in a file. Every tool prints refs and accepts them, in exactly the form they were printed. This is the foundation for everything else in Prism, so it comes first.

Owned by `prism/ref`. No tool parses or formats refs on its own.

## Grammar

```
ref       = path [ ":" target ] [ "@" rev ]
target    = location | name [ "#" ordinal ]
location  = line [ ":" col ] | line "-" line
name      = qualified symbol name, as outline.Qualified produces it
ordinal   = positive integer (only for duplicate names, see below)
rev       = any git revision without spaces (abc123f, HEAD~2, main)
```

Examples:

| Ref | Means |
| --- | --- |
| `internal/files/files.go` | the file |
| `internal/files/files.go:List` | the function `List` |
| `internal/files/files.go:globSet.match` | the method `match` on `globSet` |
| `src/shape.rs:Circle.area` | `area` in `impl Circle` or `impl Shape for Circle` |
| `README.md:For coding agents` | a Markdown heading (spaces and colons are fine) |
| `main.go:104` | line 104; resolves to the innermost symbol containing it (`main.go:run`) |
| `main.go:104:12` | line and column, as compilers print them |
| `main.go:81-200` | a line range |
| `main.go:run@b681ef1` | `run` as it was in commit b681ef1 |
| `lib.cpp:parse#2` | the second definition named `parse` in the file (overloads) |

## Parsing rules

1. **Split at the first `:`.** The left side is the path, the right side the target. Paths containing `:` are not supported in refs; such files can still be named by path alone.
2. **A target that is digits** (`104`, `104:12`, `81-200`) is a location. Anything else is a name. (A Markdown heading that is only a number, like `## 2024`, can't be named; that's an accepted gap.)
3. **The last `@` starts a revision** only if the text after it has no spaces and is a valid revision in this repo. Otherwise the `@` is part of the name (`README.md:Contact @me` still works).
4. **`#N` picks among duplicates** only if the name before it exists more than once in the file. Otherwise `#` is part of the name.
5. **Text without `:` is a file or a bare name.** Tools try it as a file first, then as a name (`List`, `Repo.Open`).
6. **Names that contain `:` themselves** (C++ `Foo::bar`, Lua `obj:method`, Ruby `A::B`) work after a path: `lib.cpp:Foo::bar`. Without a path, tools fall back to reading the whole text as a name when the part before the first `:` isn't a file.

`prism/ref` implements these rules. Its round-trip test passes on 230,794 symbols in 31,742 files of real Rust, TypeScript, Python, GDScript and Go repositories (2026-11-08).

## Names

Names are exactly what `outline.Qualified` returns, which seek and recap already use:

| Language | Example definition | Name |
| --- | --- | --- |
| Go | `func (r *Repo) Open(...)` | `Repo.Open` |
| Rust | `fn area` inside `impl Shape for Circle` | `Circle.area` |
| Python, JS/TS, GDScript | `def inner` inside `def outer` | `outer.inner` |
| Java, Kotlin, C# | method `run` in class `Job` | `Job.run` |
| Markdown | `## Install` under `# seek` | `Install` (headings don't nest into names) |

Changing what `Qualified` returns changes every ref, so it needs a schema version bump (see [output](output.md#versioning)).

## Paths

- **Printed:** always relative to the repo root, with `/`, even when the tool runs in a subdirectory. Outside a git repo: relative to the working directory.
- **Accepted:** repo-relative, cwd-relative, absolute, or a unique path suffix (`files.go`, `files/files.go`), as recap already does. If both the repo-relative and the cwd-relative reading exist and differ, the repo-relative one wins (it's what tools print) and the tool says so in its context line.

## Line ranges

A symbol's range runs from its **defining line** (where the signature is) to its last line, as outline reports it. This matches what compilers, editors and crux report as the location.

Doc comments and decorators are a separate range (`doc` in JSON). seek keeps printing them with `-s`, but reports `List` as `73-107`, not `70-107`.

Line numbers are for display. They are never part of a symbol's identity, because every edit shifts them.

## Duplicates

- Rust `struct Circle` and `impl Circle` share a name: the type wins over the impl when a ref says `Circle`. Members of the impl are `Circle.area` anyway.
- Real duplicates in one file (C++ overloads, TS overload signatures, redefinitions) get `#1`, `#2`, … in file order. Tools print the ordinal only when a name is duplicated.

## Resolving

`prism/ref` parses and matches. **Each tool supplies the candidates**, because each tool's scope differs: recap matches against the symbols in its comparison, seek against the repo, glint against one file.

Matching order:

1. Exact qualified name in the given file.
2. Qualified name ending in the given name at a `.` boundary (`match` finds `globSet.match`).
3. With no path: the same two steps across the tool's whole candidate set.

A type or class name matching its members (`recap --show Repo` shows all changed `Repo.*`) stays tool-specific behaviour.

**More than one match:** show all of them when they fit the tool's budget, ranked by context (changed symbols first, see [context](context.md)). If they don't fit, list them as refs, the context's best guess first. Never fail with just "ambiguous".

**No match:** say so, and list the nearest candidates as refs (recap already does this well: `changed: internal/files/files.go:List, …`).

## Printing

Every ref a tool prints must be valid input to every other tool. Two forms are allowed:

- **Full:** `internal/files/files.go:List`. Required in JSON, in `next:` suggestions, and anywhere a symbol appears outside a file group.
- **Grouped:** a file header followed by indented symbol lines, to save tokens. Allowed only when *header path* + `:` + *name as shown* is a valid ref. Example: a header `internal/files/files.go` and a line `changed func List L73-107` combine to `internal/files/files.go:List`.

Consequence for glint: `glint -A --outline` shows the qualified name whenever the signature doesn't already show it as-is (methods, nested definitions, impl members), for example `187 globSet.match  func (g globSet) match(p string) bool`.

## Locations from other tools

Locations printed by compilers and test runners (`file:line`, `file:line:col`) are refs too. Any tool that takes a symbol accepts them and resolves them to the innermost enclosing symbol: `recap --show internal/group/group.go:312` shows the changed symbol containing line 312. This is what lets crux output flow into recap and seek with no translation.

## Required tests

- **Round trip:** for every symbol outline finds in a corpus of real repos (the four tools, plus a Rust, a Python and a TS project), `format`, then `parse`, then `match` returns that same symbol, or an ambiguity result listing it.
- **Cross-tool:** every ref in any tool's output (text and JSON) of the acceptance scenario resolves in every other tool that takes symbols.
