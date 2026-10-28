# Diff layout and context expansion

Status: implemented on `main` (2026-10-28). The React version's `DiffPanel`
offered a split/unified switch and expandable unchanged lines; the native port
kept only a unified view. This change brings both back, plus a choice between
a side-by-side and a stacked window arrangement.

## What the user can choose

- **Diff layout** — `Unified` draws one column of old and new line numbers;
  `Split` draws the old side on the left and the new side on the right. The
  choice lives in the diff header and applies to worktree and commit diffs
  alike.
- **Panel arrangement** — `Side by side` puts the file list left of the diff;
  `Stacked` puts it above. The choice lives in the toolbar.
- Both choices are per session only. They are not written to `state.json`;
  window bounds are still persisted by MyGo's `StateKey`.

## Context expansion

A patch carries only `-U3` context, so unchanged regions between hunks are
missing. Each such gap becomes one clickable row in the diff pane:

- The gap rows are computed from the hunk headers: the space between the
  previous hunk's last old line and the next hunk's first old line. The
  leading gap (before the first hunk) works the same way; the trailing gap
  below the last hunk is not offered, because the patch does not state the
  file's length. A pure insertion (`@@ -5,0 ...`) covers no old line itself:
  old line 5 belongs to the hidden region before it, so the next region starts
  at old line 6.
- A gap row reads `▸ N unchanged lines`; clicking it loads the missing lines
  and shows them as ordinary context rows, and the header becomes `▾`. A
  second click collapses them without dropping the loaded text.
- Expanded lines are **display-only**. Staging still sends the hunk indexes of
  the original `-U3` patch, so the hunk the user clicks is exactly the hunk git
  receives; expanding never re-runs `git diff` with a larger context, which
  would merge hunks and change their indexes.
- The text comes from the new side of the diff, read lazily on the first
  click: the worktree file for unstaged changes, the index (`:0:<path>`) for
  staged changes, and `<oid>:<path>` for a commit. Both sides of an unchanged
  region are identical, so one read fills both columns; only the line numbers
  differ, and they come from the hunk headers.
- Reading happens in a goroutine; the result lands on the main thread through
  the window's update hook. The patch's gap map is replaced whenever the patch
  is replaced, and a response is installed only if the gap object it started
  from is still the one in the current map. That single identity check drops a
  late response for another file, another commit or another repository.
- The last file read is kept while its patch is shown, so expanding a second
  gap in the same file reuses it instead of reading and splitting the file
  again. The cache is dropped with the gap map.
- The map is cleared when the patch is replaced (a refresh, another file, or
  another commit), so expanded gaps do not leak across files.

## Split pairing

Within a hunk, a run of `-` and `+` lines is paired by position: the first
deletion sits next to the first addition, the second next to the second, and
the longer side keeps its extra rows alone. Context lines appear in both
columns. Pairing is positional, not a content match: a block that rewrites
lines one for one pairs them in order, which can put two unrelated lines side
by side. This is the same pairing GitHub and IDEA show; it is purely
presentational and never reorders the patch passed to git.

## Edge cases

- **Binary files, pure renames, new and deleted files** have no hunks or no
  unchanged regions, so no gap row appears and the placeholder stays.
- **Renamed files**: gap text is read from the new path, which is the path the
  patch's hunks number.
- **Paths containing `:`** cannot be read through a `<rev>:<path>` spec. The
  expansion fails with a toast and leaves the gap collapsed; the patch itself
  is unaffected.
- **File changed after the patch was loaded**: the slice can fall outside the
  fresh content. The bounds check fails, a toast reports it, and the watcher's
  refresh replaces the patch and clears the expansions.
- **Huge gaps and huge files**: a gap above `maxGapLines` (20,000) is not
  expanded, so one click cannot build an unbounded row set; the limit is on
  the gap, not the file, so a long file with a small gap still expands. A file
  above 8 MiB is not read at all: its size is checked first (`os.Stat` for the
  worktree, `git cat-file -s` for a blob), so the content is never copied just
  to be rejected.
- **Narrow windows**: the split view gives each column half the pane; long
  lines are clipped, as in the unified view. There is still no horizontal
  scrollbar.

## Verification

- `go test ./...` covers gap computation, split pairing and the row builder
  (`internal/shell/diffrows_test.go`) and the worktree/index/commit reads
  (`internal/gitx/filetext_test.go`).
- Manual in the packaged app: both layouts, both pane arrangements, and
  expanding a gap in an unstaged worktree file. The staged and commit reads go
  through the same code path and are covered by the tests above.
