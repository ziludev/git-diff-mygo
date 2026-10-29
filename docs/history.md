# History view

Status: implemented in M1 (2026-10-28).

## Why

A clean working tree left the window completely empty: the app only rendered
the working tree changes, so opening a recently committed project looked like
nothing had loaded. IDEA answers this with the Log tab next to Local Changes;
this app now does the same.

## UX

The left sidebar gets two tabs: **Changes** (the existing panel, unchanged)
and **History**.

- History lists the commits of every ref (`--all`), newest first. A commit is
  one line of fixed columns, left to right: the author's avatar, the author,
  the relative commit time, the lane graph, the message and its ref chips on
  the right. 100 commits per page with a "Load older commits" button.
- A filter field above the list narrows the commits by subject, author, short
  hash or a ref name, case-insensitively. It filters the pages already loaded
  and also applies to the next page; a refresh keeps it. With no match the
  list says "No matching commits". Beside it, an author button opens a menu
  of the authors in the loaded pages (by name, "All authors" first); picking
  one keeps only that author's commits, on top of the text filter. The author
  is matched by exact name, so two people sharing a name are one entry; the
  choice is cleared when another repository opens, and an author whose
  commits are all on pages not yet loaded appears once those load. Filtering hides rows only: a selected
  commit that no longer matches keeps its details and diff in the right
  panel, so the filter never blanks the right panel by itself.
- Each row carries up to two branch/tag chips (tags in amber, branches muted)
  plus a `+N` count for the rest, so a tag or a side branch is visible without
  selecting the commit.
- Clicking a commit selects it; the list does not grow a file list. The right
  panel shows, top to bottom: the commit's details (subject, message body,
  author, exact time, short hash and ref chips), its changed files with status
  letter and `+N −M` counts, and the diff of the selected file. This is IDEA's
  log arrangement, minus the tree: the list stays one row per commit.
- Selecting a commit selects its first file immediately, so the diff area
  never sits empty behind a selection. Clicking another file in the list
  loads that file's patch. The hunk stage/discard buttons are hidden there: a
  commit's patch is history, not a worktree write.
- A pure rename or mode change has a patch header but no hunks; the panel says
  "No textual changes" instead of rendering an empty viewer.
- Merge commits are diffed against their first parent, which is what a reader
  expects from a linear log.
- The tab remembers no state across repositories: opening another repository
  clears the loaded pages.

### Commit columns

- The avatar is a colored disc with the author's initial (or `?`). Its color is
  a hash of the name, so one person keeps one color across sessions. No image
  is fetched and no name or address leaves the machine.
- The author and time columns have fixed widths so the graph and the messages
  line up down the list. A long name truncates; the tooltips carry the full
  name and the exact time, and the details panel carries the hash.
- The author column shows git's author (`%an`), the person who wrote the
  change. A rebase, amend or cherry-pick can make the committer differ; git's
  own default log shows the author too.

### Details panel

- The subject is one bold line; the body follows, at most four lines, and a
  longer body ends its last line with an ellipsis. The meta line holds the
  author, the exact time and the short hash.
- The body is read only when a commit is selected (`git show -s --format=%b`),
  so a page of 100 commits does not carry message bodies. A failed read only
  drops the body: the subject, the files and the diff still show.
- The details and their file list share the right panel with the diff through
  a divider the user drags, or moves with ↑/↓ once it has the focus. It starts
  at the details header plus about six file rows, so a commit with many files
  cannot push the diff off screen; dragging up gives the diff the room, and
  the file list scrolls inside whatever it has.
- A file click is tied to the selected commit: selecting another commit drops
  the old file, its patch and any read still in flight (the per-projection
  epoch and the OID check do that).

### Commit graph

Every row draws a colored lane column whose layout comes from `layoutGraph`
(`internal/shell/graph.go`), painted by `drawGraph` in
`internal/shell/history.go`:

- A lane is a promise to draw one object id next. The lane already waiting for
  a commit receives it; its first parent continues in the same lane, and every
  extra parent either joins a lane that already waits for it or opens a free
  one. Lanes with no commit left to draw end at the row.
- Colors are assigned when a lane opens and cycle through eight palette
  entries, so a branch keeps its color down the list.
- `Parents` on `Commit` is what feeds this; it is the only reason the log
  carries `%P`. The log runs with `--topo-order`, so a parent never appears
  above its child and lanes never run backwards.
- Labels come from one `for-each-ref` call per page, mapped onto commits by
  object id. Local branches sort before remote-tracking branches, then tags;
  `origin/HEAD` is dropped. A failed ref listing only drops the labels, never
  the list.
- Rows are a fixed 30 px tall so consecutive rows line up; all rows share the
  widest lane count so the graph and the messages start at one x. The layout is
  a pure function of the loaded commits, so appending a page recomputes it in
  one pass.
- `drawGraph` strokes segments 2 px wide and overlaps each end by half a pixel
  so two rows' antialiased edges leave no seam. A merge edge is a cubic
  S-curve: it leaves the node vertically and arrives in the parent lane
  vertically, like IDEA's log, instead of cutting diagonally across the row.
  The node is drawn over a halo in the row's own background color, so lanes
  crossing that row stop at the halo rather than touching the circle.

### Commit actions

Right-clicking a commit row opens a menu. It acts on the row clicked, whichever
commit the right panel shows:

- **Copy Hash** copies the full object id.
- **Checkout Revision** detaches HEAD at the commit (`git checkout --detach`).
- **New Branch from Here…** and **Tag Here…** ask for a name in a small
  dialog (Enter confirms, Escape cancels, an empty name disables the button).
  The branch is checked out; the tag is lightweight.
- **Cherry-pick** applies the commit onto the current branch; **Revert** adds a
  commit that undoes it. Both are omitted for merge commits (they need a
  mainline choice the app cannot ask for yet).
- **Reset Current Branch to Here** has Soft, Mixed and Hard. Only Hard asks
  for confirmation, because it throws away uncommitted work; the others keep
  the working tree and stay recoverable through the reflog.

All of them run through the write slot, so they never overlap another write or
a fetch/pull/push, and the menu items are disabled while one runs.

Cherry-pick and revert can stop on a conflict, and the app has no conflict
editor yet. Rather than leave the repository in a half-applied state, the app
runs the matching `--abort` and says so; the working tree is as it was. A
failure that is not a conflict (a dirty tree git refuses to overwrite) leaves
nothing to abort and shows git's message.

## Data flow

```
RepoService.history({limit, skip})   -> gitx.Log        -> []Commit
RepoService.commitBody(oid)          -> gitx.CommitBody -> string
RepoService.commitFiles(oid)         -> gitx.CommitFiles -> []FileEntry
RepoService.commitDiff({oid, path})  -> gitx.CommitDiff  -> DiffResult
```

`internal/shell.App` owns the view state (`view`, `commits`, `commitFiles`,
`commitPatch`) next to the worktree state, so the diff panel has one source to
read. The selected worktree file and the selected commit file are kept
separately, so switching tabs does not reload or lose a diff.

## Git commands

| Call | Command |
|---|---|
| Log | `git log --all --topo-order --max-count=N --skip=M --format=%H%x1f%h%x1f%an%x1f%at%x1f%P%x1f%s%x00 --` |
| Labels | `git for-each-ref --format=%(objectname)%09%(refname)` |
| Body | `git show -s --format=%b <oid>` |
| Files | `git show --format= --name-status -z -M --first-parent <oid>` and `git show --format= --numstat -z -M --first-parent <oid>` |
| Checkout revision | `git checkout --detach <oid>` |
| New branch here | `git checkout -b <name> <oid>` |
| Tag here | `git tag <name> <oid>` |
| Cherry-pick | `git cherry-pick <oid>`; on a conflict `git cherry-pick --abort` |
| Revert | `git revert --no-edit <oid>`; on a conflict `git revert --abort` |
| Reset | `git reset --soft\|--mixed\|--hard <oid>` |
| Patch | `git show --format= --no-color --no-ext-diff --no-textconv --no-show-signature --first-parent -M -U3 <oid> -- <path> [<origPath>]` |

`-z` keeps paths byte-exact (no quoting, spaces and newlines are safe). The
log uses `%x00` between records and `%x1f` between fields: a subject is
arbitrary text and may contain any other control byte, and it is the last
field, so a `%x1f` inside it stays in the subject. `%x1e` is not safe as a
record separator for the same reason.

## Edge cases

- **Unborn branch** (`HEAD` has no commit): `Log` returns an empty list, not an
  error, so a fresh repository shows "No commits yet".
- **Root commit**: `git show` compares against the empty tree; no special case.
- **Merge commit**: `--first-parent` produces the diff against parent 1, so the
  file list is never empty just because a file matches another parent.
- **Renames**: the new path is the row; `origPath` travels with it and the
  patch call passes both sides so git emits the rename header. Only `-M` is
  passed, so a copy is reported as an addition, not as `C`.
- **Binary files**: numstat reports `-`; the entry is marked `binary` and the
  patch reports "Binary files … differ", which the viewer already renders as
  a binary placeholder.
- **Empty commit**: both commands return nothing; the commit's panel shows an
  empty file list rather than an error.
- **Slow repositories**: history is loaded lazily when the tab is first opened,
  never on the watcher's status refreshes; a stale response is dropped by a
  request counter.
- **Revision argument**: the object id comes from the app's own log, and is
  still checked to be hex before it reaches git, so it can never be read as an
  option.
- **Abort after a conflict**: detected through `CHERRY_PICK_HEAD` /
  `REVERT_HEAD`. An empty cherry-pick (already applied) also stops with
  `CHERRY_PICK_HEAD` set and is aborted the same way.
- **Detached HEAD**: reset moves the detached HEAD; the menu says "Current
  Branch" only because that is the usual case.
- **After a reset or checkout**: the history reloads from the top; the right
  panel keeps showing the chosen commit, which is still a valid object.
- **Refresh**: a commit or a manual refresh reloads the first page, so a new
  commit appears at the top. Watcher events (`reason: "watch"`) do not touch
  the commit list: they fire for every edit and the list does not change.

## Not in this change

A browsable project file tree, branch and path filtering, cherry-pick and
revert of merge commits, multi-select, and a conflict editor.
