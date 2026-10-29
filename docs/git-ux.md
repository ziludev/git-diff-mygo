# Git UX polish (IDEA-style)

Status: plan approved 2026-10-29. Batches A–D implemented (D without branch
filter and multi-select); E partly (push dialog, commit and push, stash).

## Decisions

- **Branch entry**: the toolbar chip opens a popover for quick search and
  checkout, and the Branches tab stays as the full management tree (local and
  remote refs). This mirrors IDEA's branch popup plus its Git Branches tool
  window.
- **Refs**: local and remote branches. Checking out a remote branch creates a
  local branch from it and attaches the upstream as a best-effort second step
  (`git checkout -b`, then `git branch --set-upstream-to`), because `--track`
  refuses a ref whose remote is not configured.
- **Trees**: branches group by their `/` prefix and changed files group by
  directory. Branches use `ui.Outline`, which keeps open items by key and
  rebuilds only the rows in view. Changed files are a plain column: a file row
  opens its diff on a single click, while an Outline row owns its press and
  only selects it, which would cost one click per file. The directory trees
  are cached per status and filter, so a frame only walks them.
- **Order**: A (visual base) first, then B (branch), C (changes), D (history).

## Batches

| Batch | Scope | Why first |
|---|---|---|
| A | icon set, toolbar, hover actions, row context menus | every later batch builds on it |
| B | branch popover, prefix tree, local/remote groups, tracking checkout | the main "IDEA feeling" gap |
| C | changes directory tree, untracked section closed, flat/tree toggle; folder stage/unstage delivered | makes large worktrees readable |
| D | history text/author filter and commit context menu (delivered); branch filter, multi-select planned | history becomes actionable |
| E | commit-and-push and push dialog (delivered; see `docs/sync.md`), stash (delivered; see `docs/stash.md`); pull strategy, conflict editor, blame, background fetch | not scheduled |

## Batch A: visual base

### Icons

`internal/shell/icons.go` inlines a small subset of Lucide (ISC) as SVG
documents parsed once at startup. `ui.Icon` tints the artwork with the text
color, so icons follow the theme and hover states without a second asset set.
No icon dependency is added.

- Icons mark frequent or unambiguous actions only: refresh, fetch, pull, push,
  branch, chevron, more, stage, unstage, discard, delete, confirm, cancel.
- Every icon button carries a short `Label` (Stage, Fetch, Delete branch) and
  a tooltip that explains it. Icon-only rows have no visible text to click, so
  the label is also what tests and assistive technology find.
- Action buttons no longer use emoji or box-drawing characters: they render
  differently per platform and ignore the text color. The diff's expand and
  collapse arrows (`▸`/`▾`) stay text because they label a state in a row, not
  a button.

### Toolbar

Left to right: app name, repository name, blank space, the sync state text, the
branch chip, fetch/pull/push icons, a divider, refresh, the more menu, and the
Open Repository button.

- Sync and refresh are icon buttons with tooltips. The state text
  ("Fetching…", "Pulling…", "Refreshing…") stays: an icon cannot say which
  operation is running.
- The branch chip gains a branch icon and a chevron. It still opens the
  Branches tab in batch A; batch B replaces that with the popover.
- The pane arrangement (Side by side / Stacked) moves from a segmented control
  into the more menu as two checked items, so the toolbar stops competing for
  width. The diff layout switch stays in the diff header, where it applies.

### Row actions

- Working-tree rows and branch rows show their action icons while hovered or
  selected, as IDEA does. Actions still exist in the context menu, so a hidden
  button is never the only path to an operation.
- Row hover gets the theme's `SurfaceHover`; selection keeps `Background`.
- Right-click menus:
  - working-tree file: Show Diff, Stage / Unstage, Discard Changes… (the
    items that do not apply are not added);
  - non-current branch: Checkout, Delete Branch…;
  - the current branch has no menu.
- Menus and hidden buttons respect `busy()`, the same guard as before: a
  network operation and a worktree write never overlap.

### Timing and edge cases

- Hovering a row asks for a frame (the row looks at `Hovered()`), so the action
  icons appear without a data load; no git call runs on hover.
- A context menu is built when it opens; the frame after a choice rebuilds it
  and runs the chosen action on the main thread, like a button's `OnClick`.
- A row whose action is unavailable (conflicted, untracked discard, unborn
  branch) omits the item instead of showing a disabled one, except Stage and
  Unstage, which stay disabled while busy to explain why a click does nothing.
- Icon names are typed constants, so a missing icon is a compile error rather
  than a blank glyph.
- Row actions are mouse-first for now: rows are not focusable, so Tab cannot
  reach the hover-only buttons and the context menu has no keyboard entry.
  Row focus and keyboard navigation come with a later batch.

## Batch B: branch popover and tree

- `gitx.Branches` lists `refs/heads` and `refs/remotes`; `BranchEntry.Remote`
  tells the two apart. Records carry the full ref name, and a symbolic ref
  (`%(symref)` non-empty, as `origin/HEAD`) is dropped. The subject is the
  last field so a `%1f` inside it cannot split the record. `git remote` names
  the remotes, longest first, so a remote whose name contains `/`
  (`foo/bar/main`) still splits at the right border instead of at the first
  segment.
- The chip popover: search field, the same tree as the tab (Local / Remote
  groups, folders by `/` segment), and a footer with New Branch and Manage
  Branches. Escape or a press outside closes it; a successful checkout closes
  it; a watcher refresh does not. Opening it reloads the list, because the
  watcher ignores `.git` and a terminal fetch or `git branch` is otherwise
  invisible; a failed checkout also reloads, so a local branch created outside
  is picked up on the next attempt.
- The search field keeps the focus: Up/Down move the choice through the
  matches, Enter checks the choice out, and typing keeps filtering. A click
  chooses a row; a double click or Enter on it acts (a list row owns its press,
  so a click cannot switch by itself). The Branches tab keeps the same tree
  with the filter and inline New Branch field; it is the management view. Both
  trees open their folders on their first list, then keep the user's open state
  by row key. A filter opens every ancestor of a match once per filter change,
  so folders can still be collapsed while it is active; a moved HEAD reveals
  its folder in both trees.
- Row actions: checkout (double click, Enter, or the menu), delete (local,
  non-current, shown while hovered or selected, and in the menu). A remote
  branch with a local counterpart checks that branch out; otherwise it creates
  the local branch from `refs/remotes/<name>` and attaches the upstream as a
  best-effort second step (`git checkout -b`, then `git branch
  --set-upstream-to`), because `--track` refuses a ref whose remote is not
  configured, and the qualified start point avoids a local branch or tag named
  like `origin/main`.
- The New Branch draft belongs to one surface at a time; closing the popover
  discards it, and both New Branch buttons are disabled while a write runs or
  on an unborn branch, where there is no commit to branch from.
- The current branch is highlighted and sorted first within its folder rather
  than pinned to the top: a pin would break the prefix tree.

## Batch C: changes directory tree (delivered 2026-10-29)

- The four sections stay. Every section's rows group by directory: a folder
  row shows a chevron, the folder's name, the status letters of the files
  below it and their count; clicking it opens or closes the folder. A file row
  shows its name (the tooltip keeps the full path) and keeps its hover actions
  and context menu.
- **Untracked starts collapsed.** A fresh checkout or a build can leave
  thousands of new files behind, and they are rarely what a commit starts
  with; the header's chevron opens the section and the state lasts the
  session. The other three sections stay open.
- The more menu's "Group files by directory" item switches between the tree
  (default) and the flat list of full paths. Folder open state lasts the
  session: it is kept while the user switches tabs and across repositories.
- The filter narrows paths and, on each new filter, opens every folder and the
  untracked section once, so a match is never hidden; closing a folder while
  the filter stays works, and the closes stay until the filter changes again.
- **Folder actions (delivered).** A folder row shows Stage (in Changes and
  Untracked) or Unstage (in Staged) while hovered, and the same items in its
  context menu. The action covers exactly the files listed under that folder
  in that section, not the directory on disk: `git add -- <dir>` would also
  stage untracked files that the Changes section does not show. Conflicted
  folders have no action.
- `gitx` gains `StagePaths` and `UnstagePaths`, which send the explicit file
  list on stdin (`--pathspec-from-file=- --pathspec-file-nul`, with
  `--literal-pathspecs`): a folder can hold thousands of files, which would
  overflow the argument limit, and a name like `*.go` or `:(top)x` must not be
  read as a pattern. Both are one `git` call, so the index is written once.
  Renames carry both sides; unborn branches unstage with `git rm --cached`.
  They are still serialized by `busy()`.
- If the status changed between the frame and the click, a listed path may no
  longer exist and git refuses the whole call; nothing is staged, the error
  shows as a toast and the refresh that follows redraws the folder.
- Renames group under their new path; the original path stays in the tooltip.
- Folder-level discard is not offered: it is the one destructive folder action,
  and it is left until it has a confirmation that names how many files go.

## Batch D: history actions (filters and commit menu delivered 2026-10-29)

- **Commit context menu (delivered).** Copy hash, checkout revision, new
  branch here, tag here, cherry-pick, revert and reset (soft/mixed/hard);
  details and edge cases in `docs/history.md`. Hard reset asks through the
  existing confirmation dialog; name entry uses the same dialog with a text
  field. A conflicting cherry-pick or revert is aborted automatically, since
  there is no conflict editor yet; a dirty worktree surfaces git's refusal
  rather than discarding anything.
- **Text filter (delivered).** A field above the commit list narrows it by
  subject, author, short hash or a ref name, case-insensitively. It filters
  the pages already loaded: "Load older commits" still adds the next page,
  which is filtered as it arrives. A commit that does not match is not a row,
  but its details stay in the right panel if it was selected, so filtering
  never blanks the diff by itself.
- **Author filter (delivered).** An author button next to the text field lists
  the authors of the loaded pages and keeps one author's commits (single
  choice, exact name; a menu closes after each pick, so no multi-select). It
  combines with the text filter and resets when another repository opens.
- Branch filtering and multi-select, with the outgoing commits of the
  checked-out branch marked, are still planned.
