# Output: readable reports and `--json`

Every tool has two output formats:

- **Text** (the default). Colored on a terminal, plain with `-A` or when piped. This is what people *and agents* read. It stays budgeted and token-efficient.
- **JSON** (`--json`). For programs: other tools, the future `prism` command, scripts, tests. Agents should not read JSON into their context to save tokens; the text report is shorter.

Owned by `prism/report`. The tools fill in their `result`; the envelope and shared objects come from the library.

## Text conventions

These apply to all four tools, on top of each tool's own layout.

1. **Refs as specified in [refs](refs.md).** Full refs, or grouped output where header + `:` + name is a valid ref.
2. **A `context:` line** at the top whenever context changed anything (order, marks, a filled-in gap). One line, for example:
   ```
   context: 3 changed symbols vs HEAD; focus: commit b681ef1 (recap, 4 min ago)
   ```
   No line when context changed nothing. See [context](context.md).
3. **Marks** on symbols that context knows about, in brackets after the name: `[changed]` (uncommitted), `[branch]` (changed on this branch, committed), `[focus]` (changed in the commit or range recap last showed), `[failing]` (failed in the last crux run).
4. **`next:` lines** at the bottom: at most three suggested commands, runnable exactly as printed, with full refs and a short reason:
   ```
   next: seek internal/files/files.go:List        definition and callers
   next: crux go test -run '^TestList$' ./internal/files/   runs the 1 test that mentions List
   ```
   seek and recap already print hints like this; Prism makes them consistent and context-aware. Suggestions follow the wording rules in [evidence](evidence.md).
5. **Budgets stay, and context never costs a result.** Context lines, marks and `next:` lines fit inside the tool's line budget only by giving up detail (an extra hit line, a glance line, a longer excerpt), never an item that would be listed without context (a file, a symbol, an error, a failure). A tool that has no detail to give up puts its context lines on top of the budget instead. So with a tight budget, `--no-context` output can show a little more detail, never more items.

## JSON envelope

One JSON document per run (not JSON Lines): budgets already bound the size, and a single document is easiest for `jq` and for Go's decoder. It goes to stdout, also on failure, and the tool keeps its usual exit code.

The shape is defined in Go in [`prism/report`](../report): the envelope and shared objects in `report.go`, each tool's result in `results.go`. Golden files in [`report/testdata`](../report/testdata) show a full example per tool. Programs reading the output can use `report.Check`, which also rejects a schema version they don't know, and `report.Refs`, which collects every ref in a document.

```json
{
  "schema": "prism/1",
  "tool": "seek",
  "version": "0.3.0",
  "args": ["-d", "List"],
  "repo": {
    "root": "/home/ransom/Projekte/seek",
    "git": true,
    "git_dir": "/home/ransom/Projekte/seek/.git",
    "head": "b681ef1c…",
    "branch": "main",
    "default_branch": "main",
    "dirty": false
  },
  "context": null,
  "result": { },
  "links": [ ],
  "next": [ ],
  "truncated": null
}
```

| Field | Meaning |
| --- | --- |
| `schema` | `prism/<major>`. See [versioning](#versioning). |
| `tool`, `version` | which tool and release produced it |
| `args` | the arguments after the tool name, as given |
| `repo` | from `prism/repo`. Always present: outside git, `git` is false and `root` is the working directory, so paths can always be made absolute |
| `context` | what context the run had and what it did with it (`applied`: any of `rank`, `mark`, `fill`, `suggest`), below; `null` with `--no-context` ([context](context.md)) |
| `result` | the tool's result, below; left out when `error` is set |
| `error` | why the run failed, instead of `result` |
| `links` | relations between symbols, each with a basis (see [evidence](evidence.md)) |
| `next` | the same suggestions as the `next:` lines: `{ "cmd": "...", "why": "..." }` |
| `truncated` | `null`, or `{ "shown": 120, "total": 431 }` when a limit cut the result |

All names are `snake_case`. Lines are 1-based, ends inclusive. Paths are repo-relative like in text output; `repo.root` makes them absolute. Lists that can be empty are always present (`[]`); optional values are left out when they don't apply. Times are RFC 3339 with the offset.

Fields named `ref`, `in`, `test`, `target`, `from`, `to` and `old_ref` always hold refs, and no other field does, so a program can find every ref without knowing each tool's result.

### `context`

```json
"context": {
  "applied": ["rank", "mark"],
  "changed": { "base": "HEAD", "branch_base": "6e1c2d0…", "files": 3, "uncommitted": 2, "branch": 5 },
  "focus": { "range": "b681ef1", "old": "1f0e…", "new": "b681ef1c…", "symbols": 4, "by": "recap", "at": "2026-11-08T14:02:11+01:00" },
  "last_run": { "cmd": "go test ./...", "exit": 1, "failing": 2, "by": "crux", "at": "2026-11-08T14:05:40+01:00" },
  "notes": ["nothing matches in the working tree; showing focus commit b681ef1"]
}
```

`applied` is always there: any of `rank` (reordered results), `mark` (marked results), `fill` (filled a result that would have been empty), `suggest` (a `next:` line only context gave a reason for, such as crux suggesting `recap --show` for the changed symbol an error is in); empty when context was available but changed nothing. `changed` is there in a git repo for tools that read changes, `focus` and `last_run` when the tool reads them and a live record exists. `changed.skipped` says why symbol detail was skipped (`"812 changed files"`). `notes` are the sentences the `context:` line adds beyond the layers. Counts, not lists: the marks on the result's own symbols say which ones context touched.

## Shared objects

**Symbol**

```json
{
  "ref": "internal/files/files.go:List",
  "path": "internal/files/files.go",
  "name": "List",
  "kind": "func",
  "line": 73,
  "end": 107,
  "doc": [70, 72],
  "sig": "func List(roots []string, o Options) ([]string, error)",
  "marks": ["changed"]
}
```

`doc` (the doc comment's lines), `sig`, `rev` (set for a symbol as of a commit) and `marks` are optional.

**Location**: a place in a file, plus the symbol containing it (if any).

```json
{ "path": "internal/group/group.go", "line": 312, "col": 9, "in": "internal/group/group.go:multiline", "marks": ["changed"] }
```

`marks` are the context marks of the symbol in `in`, left out when it has none.

**Link**: see [evidence](evidence.md).

```json
{ "from": "internal/files/files_test.go:TestList", "to": "internal/files/files.go:List", "rel": "mentions", "basis": "reference", "at": "internal/files/files_test.go:12" }
```

**Error**: on failure the envelope carries `error` instead of `result`. Codes: `no_match` (a ref or name named nothing), `ambiguous` (it named more than the command can take), `usage` (bad arguments), `failed` (anything else). No-match and ambiguity include candidate refs, so a program can pick one without parsing text.

```json
{ "error": { "code": "ambiguous", "message": "\"Open\" matches 2 definitions", "candidates": ["a.go:Repo.Open", "b.go:File.Open"] } }
```

## Tool results

Summaries; the Go types in `report/results.go` are authoritative.

- **seek** (`report.Seek`): `pattern`, `target` (in ref mode), `query` (whole word, ignore case, plain text), `definitions` (Symbols), `groups` (each: `path`, `file_kind`, `symbol` containing the hits or `null`, `hits` with `line`, `col`, `text`, `label`, `writes`), `files` (with `-l`, and only then), `skipped` (what wasn't searched), `counts` over the whole search (hits, files with hits, definitions, files searched).
- **recap** (`report.Recap`): `old` and `new` (the sides as named), `commit` (with `recap show`), `files` (each: `path`, `old_path`, `status`, `added`, `removed`, `similarity`, `line_endings`, `mode_change`, …, `changes` with `what`, `symbol`, `old_ref`, `old_sig`/`new_sig`, `from`, `moved_to`, `old_line`/`old_end`, `differs`, `members`, `detail`; `outside` for lines outside definitions; `defs` of new and deleted files, each with `symbol`, `from`, `moved_to`), `commits` and `range` (recap log, each commit with its files), `show` (with `--show`: each symbol's change, or a whole `file`, with all diff lines), `notes`.
- **crux** (`report.Crux`): `command`, `exit`, `signal`, `duration_ms`, `tools`, `tests` (with `counts_unknown` when go test didn't report per-test counts), `errors`/`failures`/`warnings` (each: `kind`, `title`, `loc` with `in`, `test`, `pkg` for a go test failure without a line, `missing`, `body`, `count`, `locs`, `names`), `notes`, `tail`, `lines`, `log`. `loc.in` and `test` are the refs from [evidence](evidence.md#crux-failures-and-symbols).
- **glint** (`report.Glint`): `items` and `dir` for a listing (the entries glint's `--json` printed as a bare array before; their paths stay as listed, relative to `dir`, the working directory, like `ls`; a symlink's target is `link_target`), or `file` for one file (`path`, `lines`, `size`, `modified`, `mode`, `kind`, `git`, `last_commit`, `symbols`). Moving the listing into the envelope breaks consumers of the old bare array; glint is pre-1.0, and its release notes say so.

## Budgets and limits in JSON

The text line budget does not apply to `--json` unless `-b` is given explicitly: programs want complete results. A hard safety cap (10,000 items per list) still applies, reported in `truncated`.

## Versioning

- `schema` is `prism/1` until a breaking change.
- Adding fields, marks, link types or tool result fields is not breaking. Consumers must ignore unknown fields.
- Removing or renaming a field, changing a field's meaning, or changing how refs are formed (including what `outline.Qualified` returns) is breaking: `prism/2`.
- The JSON shape is defined once in `prism/report` and covered by golden tests there, so tools can't drift.
