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
5. **Budgets stay.** Context lines, marks and `next:` lines count against the tool's line budget.

## JSON envelope

One JSON document per run (not JSON Lines): budgets already bound the size, and a single document is easiest for `jq` and for Go's decoder.

```json
{
  "schema": "prism/1",
  "tool": "seek",
  "version": "0.2.0",
  "args": ["-d", "List"],
  "repo": {
    "root": "/home/ransom/Projekte/seek",
    "git_dir": "/home/ransom/Projekte/seek/.git",
    "head": "b681ef1c…",
    "branch": "main",
    "default_branch": "main",
    "dirty": false
  },
  "context": {
    "changed": { "base": "HEAD", "symbols": 0 },
    "focus": null,
    "last_run": null,
    "applied": []
  },
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
| `repo` | from `prism/repo`; `null` outside a git repo |
| `context` | what context was available and what it was used for (`applied`: any of `rank`, `mark`, `fill`) |
| `result` | tool-specific, below |
| `links` | relations between symbols, each with a basis (see [evidence](evidence.md)) |
| `next` | the same suggestions as the `next:` lines: `{ "cmd": "...", "why": "..." }` |
| `truncated` | `null`, or `{ "shown": 120, "total": 431 }` when a limit cut the result |

All names are `snake_case`. Lines are 1-based, ends inclusive. Paths are repo-relative like in text output; `repo.root` makes them absolute.

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
  "rev": null,
  "marks": ["changed"]
}
```

**Location**: a place in a file, plus the symbol containing it (if any).

```json
{ "path": "internal/group/group.go", "line": 312, "col": 9, "in": "internal/group/group.go:multiline" }
```

**Link**: see [evidence](evidence.md).

```json
{ "from": "internal/files/files_test.go:TestList", "to": "internal/files/files.go:List", "rel": "mentions", "basis": "reference", "at": "internal/files/files_test.go:12" }
```

**Error**: on failure the envelope carries `error` instead of `result`. Ambiguity and no-match include candidates, so a program can pick one without parsing text.

```json
{ "error": { "code": "ambiguous", "message": "\"Open\" matches 2 definitions", "candidates": ["a.go:Repo.Open", "b.go:File.Open"] } }
```

## Tool results

Drafts for v1. Each tool's `result` reuses the shared objects wherever it can.

**seek**

```json
{
  "pattern": "List",
  "target": "internal/files/files.go:List",
  "definitions": [ "Symbol…" ],
  "groups": [
    { "in": "Symbol… (or null for lines outside definitions)", "path": "main.go",
      "hits": [ { "line": 104, "text": "paths, err := files.List(o.paths, files.Options{…})", "writes": false } ] }
  ],
  "counts": { "hits": 6, "files": 2, "definitions": 1 }
}
```

**recap**

```json
{
  "from": "b681ef1^", "to": "b681ef1",
  "commit": { "rev": "b681ef1c…", "author": "Vaspyyy", "date": "2026-11-05T00:56:00+01:00", "message": "Use glint 0.6.1" },
  "files": [
    { "path": "internal/files/files.go", "old_path": null, "status": "modified", "added": 42, "removed": 2,
      "changes": [ { "what": "changed", "symbol": "Symbol…", "old_name": null, "old_sig": null, "new_sig": null, "added": 1, "removed": 1 } ] }
  ]
}
```

**crux**

```json
{
  "command": ["go", "test", "./..."],
  "exit": 1,
  "tools": ["go test"],
  "tests": { "passed": 41, "failed": 1, "skipped": 0 },
  "errors": [ ],
  "failures": [ { "kind": "fail", "title": "TestList", "test": "internal/files/files_test.go:TestList",
                  "loc": "Location…", "body": ["files_test.go:31: got 2 files, want 3"] } ],
  "warnings": [ ],
  "log": "/home/ransom/.local/state/crux/logs/…"
}
```

crux's existing `digest.Report` and `digest.Item` map onto this almost one to one. New: `loc.in` (the enclosing symbol) and `test` (the failing test's ref), see [evidence](evidence.md#crux-failures-and-symbols).

**glint**: the listing (`items`), the project card, or the outline (`symbols`, a list of Symbol). glint's current `--json` prints a bare array; it moves into `result.items` in the same release that adopts the envelope (glint is pre-1.0, so this is allowed, but the release notes must say so).

## Budgets and limits in JSON

The text line budget does not apply to `--json` unless `-b` is given explicitly: programs want complete results. A hard safety cap (10,000 items per list) still applies, reported in `truncated`.

## Versioning

- `schema` is `prism/1` until a breaking change.
- Adding fields, marks, link types or tool result fields is not breaking. Consumers must ignore unknown fields.
- Removing or renaming a field, changing a field's meaning, or changing how refs are formed (including what `outline.Qualified` returns) is breaking: `prism/2`.
- The JSON shape is defined once in `prism/report` and covered by golden tests there, so tools can't drift.
