# Context: detected, derived, remembered

Context is what the tools know about the repo and the current work beyond the arguments of one command. It is **on by default**. Nobody, neither the user nor an agent, has to ask for it, type a placeholder, or remember a flag. If better results would depend on that, it's a tooling bug.

The risks of automatic context (stale state, agents overwriting each other, runs that can't be reproduced) are handled by the tools themselves, with the rules below.

Owned by `prism/repo`, `prism/changes` and `prism/focus`; `prism/work` loads them together for a run and turns them into marks, the `context:` line and the JSON `context`.

## Three layers

### 1. Detected: the repo

Read fresh on every run by `prism/repo`, in one shared implementation:

- repo root (`git rev-parse --show-toplevel`), worktree, git dir (per worktree: linked worktrees get their own)
- HEAD, branch, default branch (`origin/HEAD`, falling back to `main` or `master`)
- dirty or clean

"Detect the repo once" means one implementation that all tools share, so they agree on root, paths and branch. It's a few milliseconds of git per run, so there's no cross-process cache to go stale.

### 2. Derived: what changed

Computed fresh on every run by `prism/changes`, from git:

- **Uncommitted**: working tree and index vs HEAD, untracked files included (what plain `recap` shows). Marked `[changed]`.
- **Branch**: on a branch other than the default one, commits since the merge base with the default branch. Marked `[branch]`.

`prism/changes` turns changed line ranges (`git diff -U0`) into changed symbols with outline. It's deliberately light: renames, moves and signature changes stay recap's job. Both use the same outline, so the names agree.

Derived context can't go stale and can't collide between agents, because it isn't stored. **Most context should be derived.** Only store what can't be.

What it costs (measured 2026-11-08): about 5 ms on a typical repo. In a 31,000-file repo it costs what `git status` costs there, about 60 ms, nearly all of it listing untracked files. The changed files come from one `git status`, which, like any `git status` run in the foreground, writes a refreshed index back when nothing else holds the index lock. With a stale index (file times changed, contents not) the first run pays the refresh once, 2 s in that repo, instead of every run paying 0.6 s. The line diffs that map changes to symbols only look at the changed files, and nothing runs when nothing changed.

### 3. Remembered: focus and last run

Two small records that can't be derived from git:

- **Focus**: the commit or range recap last showed explicitly (`recap show abc123`, `recap main...feature`). It holds the range plus the list of changed symbol refs (capped at 500), so other tools don't recompute them. Plain `recap` (the working tree) doesn't set focus, because that's already derived context. Marked `[focus]`.
- **Last run**: crux's last verdict in this worktree: command, exit code, failing tests and failure locations as refs. Marked `[failing]`.

## Rules

1. **Explicit wins.** A revision, path or ref given on the command line always beats context. Context only fills what the command left open.
2. **Rank, mark, fill. Never hide.** Context may reorder results, mark them, fill a result that would otherwise be empty, and add a `next:` suggestion it gives a reason for. It never drops a result that would have appeared without context. A wrong guess costs ordering, never a missed hit.
3. **Fill only what's empty, and say so.** Example: `recap --show List` finds nothing changed in the working tree, but `List` changed in the focus commit, so recap shows that and says `context: nothing matches in the working tree; showing focus commit b681ef1`.
4. **Say what was used.** Whenever context changed anything, the text report has one `context:` line and the JSON has `context.applied`. Runs stay reproducible from the output alone: anyone can repeat one with the stated revision. The line names only the layers whose marks the report shows, then any notes (such as a fill-in), for example `context: 1 changed symbol vs HEAD; focus: commit b681ef1 (recap, 4 min ago); last run: crux go test ./... failed (2 min ago)`. A range focus is named with the commits it resolved to, `focus: HEAD~1..HEAD (1f0e2d3..b681ef1)`, because names like HEAD move.
5. **Stay fast.** Context may add about 50 ms on a typical repo. When it would cost more (for example more than 200 changed files), the tool skips the expensive part and says so: `context: 812 changed files, symbol detail skipped`.
6. **Safe with parallel agents.** Derived context is per run. Remembered context is per worktree and per agent session (below), written atomically, and expires.

## What each tool does with it

| Tool | Reads | Writes | Behaviour |
| --- | --- | --- | --- |
| seek | changed, focus, last run | | Hits inside marked symbols come first and are marked. A bare name with several definitions is resolved toward the changed one first (all still shown). Suggests the crux command for tests that mention the symbol. |
| recap | focus, last run | focus | `--show` falls back to the focus range when the working tree has no match. Marks changed symbols that failed in the last crux run. Suggests seek for the most significant changed symbol. |
| crux | changed, focus | last run | Maps each failure location to its enclosing symbol and marks it if it's changed or in focus: `group.go:312 in internal/group/group.go:multiline [changed]`. |
| glint | changed | | `-A` and `--outline` mark changed symbols and files. |

## Storage of remembered context

- Location: `$(git rev-parse --git-dir)/prism/`. Inside the git dir, so it's never committed, and per worktree, so fix work in a separate worktree has its own context.
- Files: `focus.json` and `last-run.json`, or `focus-<hash>.json` and `last-run-<hash>.json` when a session is known (`<hash>` is the first 12 hex digits of the session's SHA-256, so any session string makes a short, safe name).
- Each record carries `by` (tool), `at` (time), `head` (HEAD when written) and `session`.
- Written atomically (temp file plus rename). Concurrent writers: last one wins, which is harmless because of rule 2.
- Expires 60 minutes after it was last written or read, or when the branch changes. An expired record is ignored and deleted.

### Sessions

Several agents in one worktree should not steer each other. The session is detected automatically, first match wins:

1. `PRISM_SESSION` (an explicit override for harnesses Prism doesn't know yet)
2. `CLAUDE_CODE_SESSION_ID` (Claude Code; verified present 2026-11-08)
3. `T3CODE_THREAD_ID` (T3 Code; verified present 2026-11-08)

Further harness variables get added once they're confirmed on a real run, not guessed. With no session found, records are shared per worktree: still safe because of rule 2, just less precise.

## Turning it off

`--no-context` on every tool, or `PRISM_NO_CONTEXT=1` (any value but empty or `0`), gives output that depends only on the arguments and the files. Nothing is read or written: recap doesn't set focus and crux doesn't record its run. The JSON `context` is `null`. It's meant for the tools' own golden tests and for scripts that diff outputs. No one should need it to get correct results.

## What context never does

- change *which* results exist, only their order, marks and empty-result fallbacks
- override an explicit argument
- persist anything outside the git dir, or anything that can be derived
- claim a relation it has no evidence for (see [evidence](evidence.md))
