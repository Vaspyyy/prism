# Prism

The shared layer that lets **glint**, **crux**, **seek** and **recap** build on each other's results.

Prism is not a fifth tool that wraps the other four. It is a small Go library that all four import, plus (later) a thin `prism` command for the few things no single tool should own. The goal: after `recap` shows a commit, an agent can take a changed function straight to `seek` (definition and callers) and on to `crux` (the tests that mention it) without retyping a path or a symbol, and every step says honestly what it knows.

Status (2026-11-08): v1 is done: phases 0 to 3 of the [roadmap](docs/roadmap.md). glint 0.9.0, seek 0.4.0, recap 0.4.0 and crux 0.6.0 print and accept the same refs, have `--json` in the shared `prism/1` format, and share context: recap's focus, crux's last run and the current changes show up as marks in every tool, each report says what context changed, and `next:` lines chain the tools without retyping a path or a symbol. `e2e/` runs the v1 acceptance test on every PR. Next: the items under "After v1" and "Found along the way" in the roadmap.

```sh
go get github.com/Vaspyyy/prism
```

## Principles

1. **Every tool stays standalone and quick.** A simple search is still just `seek setSpeed`. Prism makes results connect; it never adds a workflow you have to go through.
2. **Agents get the best result without being told how.** If good results depend on the user, or on an opt-in an agent has to remember, that is a tooling bug. Smart behaviour is on by default.
3. **One name for one thing.** A symbol or file printed by one tool is accepted by all the others, in exactly the form it was printed. See [refs](docs/refs.md).
4. **Context helps, never hides.** Tools use what they know about the repo and the current work to rank, annotate and fill gaps, but never drop a result, and they say what context they used. See [context](docs/context.md).
5. **Claims match evidence.** A search hit can suggest a test; only measured data can say the test covers the change. Every link between results records its basis. See [evidence](docs/evidence.md).
6. **Readable by default, structured on request.** The text reports stay the default (and what agents read). `--json` gives programs a stable, versioned format. See [output](docs/output.md).
7. **Library first.** Shared behaviour lives in one module with one version, so the tools can't drift apart.

## Architecture

```
github.com/Vaspyyy/prism          (Go, stdlib only, GPL-3.0 like the tools)
├── outline/   symbol finder, moved here from glint (one parser version for everyone)
├── ref/       parse, format and match refs ("internal/files/files.go:List")
├── repo/      repo root, worktree, git dir, HEAD, branch, default branch, path handling
├── changes/   changed line ranges -> changed symbols (light; recap keeps its rich analysis)
├── focus/     the small amount of remembered context (per worktree, per agent session)
├── work/      one run's context: loads changes and focus, gives marks, the context: line, the JSON context
├── evidence/  link types, basis levels and the wording each basis allows
├── testcmd/   build a test command for a set of test refs (go test -run, pytest, cargo test)
└── report/    the JSON envelope and shared objects (Symbol, Location, Link, Next)

glint ──┐
seek  ──┼──> prism        prism never imports a tool
recap ──┤
crux  ──┘
```

`cmd/prism` comes after v1 (see [roadmap](docs/roadmap.md)).

A change to outline's rules shows up as a diff of `outline/testdata/corpus.golden`, every symbol it finds in a file of each language it supports; `go test ./outline -update` rewrites it.

## Where the tools started

Before Prism, checked 2026-11-08 against glint 0.6.1, seek 0.1.0, recap 0.1.0, crux 0.3.1. Kept as the record of what v1 set out to fix.

| | glint | seek | recap | crux |
| --- | --- | --- | --- | --- |
| Symbol names | outline shows signatures (`func (g globSet) match(...)`), not qualified names | `outline.Qualified` (`Repo.Open`) | `outline.Qualified` | none, only `file:line` |
| Line ranges | definition line (73) | includes the doc comment (`files.go:70-107`) | definition line (`L73-107`) | n/a |
| Range format | `73 func List(...)` | `path:70-107`, or `L73-107` under a file header | `L73-107` under a file header | `file:line[:col]` |
| Accepts a symbol | n/a | regex pattern (`Repo.Open` as text) | `--show file:name`, `name`, or a file | n/a |
| Structured output | `--json` (bare array, listings only) | none | none | none |
| glint dependency | itself | glint v0.6.1 | glint v0.6.0 (parser can differ from seek's) | none |

The good news: seek and recap already share `glint/outline` and its `Qualified` names, so the names agree. What's missing is one printed form, one range convention, one version, and structured output.

## Spec

- [docs/refs.md](docs/refs.md): the identifier format every tool prints and accepts
- [docs/output.md](docs/output.md): text conventions and the `--json` envelope
- [docs/context.md](docs/context.md): what context is detected, derived and remembered, and the rules for using it
- [docs/evidence.md](docs/evidence.md): links between results and what each may claim
- [docs/roadmap.md](docs/roadmap.md): order of work, per-tool changes, the v1 acceptance test, decisions and non-goals
