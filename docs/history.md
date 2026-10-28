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
- The file list is capped at six rows and scrolls inside the cap, so a commit
  with many files cannot push the diff off screen.
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
- **Refresh**: a commit or a manual refresh reloads the first page, so a new
  commit appears at the top. Watcher events (`reason: "watch"`) do not touch
  the commit list: they fire for every edit and the list does not change.

## Not in this change

A browsable project file tree, ref filtering (`--branches`, paths, authors),
and checkout/cherry-pick/reset from a commit.
