# Native UI migration

Status: M1–M5 complete on `feature/native-ui` (2026-10-28). The React
frontend is removed; the app builds and runs from Go alone.

## Why

The app is built on MyGo, but on MyGo's webview path: a React page talks to Go
through generated bindings. The Go side owns every repository operation, yet
the window, the commit graph and the diff rendering live in TypeScript. MyGo
also has a native UI path — `github.com/egoist/mygo/ui` — which draws the
window on the GPU from a Go view function; the author's terminal app (`kero`)
is built that way, with TypeScript left to its website.

This branch moves Twill to that path: one language, no Vite/TypeScript build,
no frontend dependencies (node_modules holds only the `mygo-cli` wrapper that
invokes the Go build), and a view that is a plain Go function of app state.

## Target architecture

```
main.go                  window, menu, startup open
internal/gitx            the only package that starts git
internal/service         repository actions + watcher + last-repo state,
                         called directly (no mygo.Bind, no typed events)
internal/shell           UI state, async loaders, view functions,
                         graph.go (commit lane layout, port of the old
                         src/lib/graph.ts) and patch.go (unified patch parser)
```

- `mygo.NewWindow(WindowOptions{Content: ui.View(app.View)})` replaces the
  web page. The view reads `shell.App` and returns widgets; it never runs git.
- Git calls run in goroutines and come back through the window's update hook,
  which runs on the main thread. Each projection (status, commits, branches,
  diff) keeps an epoch counter; a result whose epoch is stale is dropped, which
  is what the web store's request counters did.
- The watcher calls a shell callback with the same reasons as before
  (`open`, `refresh`, `watch`); `watch` skips the commit page but refreshes the
  status and the branch list (one `for-each-ref`), as `docs/branches.md` says.
  Every other reason also reloads the commit page.
- Last-repository restore is unchanged: `service.OpenInitial` reads
  `state.json` from MyGo's user-data directory.

## Screen mapping

| Today (React) | Native |
|---|---|
| `App.tsx` toolbar + resizable split | `ui` shell view with a split |
| `RepoPanel` Changes / History / Branches tabs | tab strip + per-tab list |
| `ChangesPanel` sections, filter, hover actions | section headers + rows in a scroll column grouped by directory, untracked folded, filter field, always-visible row actions |
| `CommitPanel` message + amend | text area, checkbox, buttons |
| `HistoryPanel` + `CommitGraph` | virtualized list with a filter field; lane graph drawn per row |
| `DiffPanel` + `@pierre/diffs` | virtualized patch lines, colored backgrounds, hunk headers |
| `BranchPanel` | Local/Remote tree grouped by `/`, inline create field, popover on the toolbar chip |
| `ConfirmDialog` / toasts | native window dialog / notification bar |
| `FilterInput` | plain text input with a placeholder (no leading icon) |

## Diff rendering

- `internal/shell/patch.go` parses the unified patch into lines:
  `{kind: context|add|del|hunk|meta, text, oldLine, newLine}` plus the hunk
  index each `@@` header starts, so stage/discard pass the same indices git
  expects.
- The pane is a virtualized list of monospace lines: added lines green-tinted,
  deleted red-tinted, hunk headers a muted bar with stage/discard actions,
  meta lines (diff/index/---/++) muted.
- The header switches between the unified layout and a split one that pairs
  old and new lines; the toolbar switches the file list between side by side
  and stacked above the diff. Both choices are per session.
- The unchanged lines a patch omits appear as expandable rows
  (`▸ N unchanged lines`); expanding reads the new side of the diff lazily and
  shows it as context. Details and limits: `docs/diff.md`.
- In the History tab the right panel holds, top to bottom, the selected
  commit's details, its changed files and the diff of the chosen file; the
  details and files sit above the diff behind a divider the user drags. The
  history list stays one row per commit.
- Binary files and pure renames reuse the backend's `binary` flag and empty
  hunk list, and show a placeholder instead of an empty viewer.
- Syntax highlighting, word-level highlighting and soft wrap are out of scope
  for the first native milestone; the backend already sends the exact patch,
  so they can be added without touching git.

## Milestones

1. **Shell** — native window, toolbar, tabs, status line, resizable sidebar,
   empty state, startup restore. No web page.
2. **Changes + diff** — sections, filter, selection, stage/unstage/discard,
   hunk actions, commit panel.
3. **History** — commit page, lane graph, ref chips, the commit's details and
   file list in the right panel, per-file patch.
4. **Branches** — local and remote tree, checkout (including tracking checkout
   of a remote branch), create at HEAD, delete merged, filter.
5. **Remove the web frontend** — delete `src/`, `index.html`, `vite.config.ts`,
   `tsconfig.json`, `components.json`, the web dependencies and lockfile, and
   update README and docs. The app must still build a signed `.app` and `.dmg`.
   **Done:** a minimal `package.json` keeps `mygo-cli` so `bun run build` still
   works; the generated TypeScript client disappeared with `src/`.

## Verification (2026-10-28)

Checked in the packaged app against a demo repository:

- changes list selection and diff, file stage/unstage, commit via button and
  via Cmd+Enter inside the message field;
- the changes tree: folders by path with their status letters and count,
  folding and unfolding, the untracked section closed until its header is
  clicked, and the more menu's "Group files by directory" switching to the
  flat list;
- folder rows: the hover Stage/Unstage button and the context menu stage or
  unstage exactly the files listed under the folder in one git call (covered
  against real repositories in `internal/shell`);
- the commit menu in History (copy hash, checkout revision, new branch and tag
  through the name dialog, cherry-pick, revert, soft/mixed/hard reset with the
  hard confirmation), against real repositories;
- the push dialog from the toolbar and from Commit and Push…: outgoing
  commits, Publish for a branch without an upstream, "Nothing to push", all
  against a real bare origin;
- stash: the Stash dialog (optional message, untracked files on), the Stashes
  tab with its files in the right panel, apply, pop and drop with the
  confirmation, against real repositories;
- keyboard: Up/Down through files, commits and stashes in the order drawn,
  Space to stage and unstage, Cmd+F to the filter, and the keys staying out of
  dialogs and the commit message (`docs/keyboard.md`);
- the history filter by subject, author, hash and ref name, and its
  "No matching commits" state;
- the divider between the changes list and the commit box: dragging it
  resizes both, the arrow keys move it once it has the focus, and the split
  keeps 40 DIPs for each pane in a small window (a test drives the drag);
- the divider between a selected commit's details and files and its diff: the
  details start at the header plus six file rows, dragging up gives the diff
  the room, and the file list scrolls inside what it has (a test drives the
  drag);
- Unified/Split diff layouts, Side by side/Stacked panes, and expanding a
  worktree gap (a commit gap is covered by `gitx` and row-builder tests);
- history columns (avatar, author, time, lane graph, message, chips) checked
  in the packaged app; the commit's details, file list and diff in the right
  panel are covered by `internal/shell` tests;
- branch list and popover trees, checkout by choosing a row and pressing Enter
  or double-clicking (a list row owns its press, so a single click only
  chooses), search filtering and Down/Enter in the popover, create from the
  inline field, delete with the confirmation dialog;
- remote branch checkout through the popover against a demo repository with
  an `origin` (the local tracking branch is created);
- Fetch/Pull/Push in the toolbar and the Git menu; Fetch was run against a real
  repository without an error toast, Pull and Push only in tests (running them
  against the open repository would move its branches);
- screen refresh after an external file edit (watcher), startup restore;
- `bun run build` after `src/` was deleted: signed `.app` and `.dmg` still
  produced (the build no longer runs a frontend step).

Keyboard: Cmd+1/2/3/4 switch Changes/History/Branches/Stashes, Cmd+O opens the repository
picker, Cmd+Enter commits (including from the focused message field).

## Known gaps

- No syntax or word-level highlighting, and long lines are clipped
  horizontally without a horizontal scrollbar in both diff layouts.
- Clickable rows are plain views: they respond to the mouse but are not
  exposed to the accessibility tree as buttons.
- Hunk-level stage/unstage/discard is implemented and unit-tested through the
  patch parser, but was not exercised by a real click in the packaged app.

## Edge cases

- **Never block the view**: status, log, for-each-ref and patches are async;
  a slow repository keeps the window responsive.
- **Repo switch**: an epoch per projection drops responses for the repository
  the user already left; selections and the selected commit reset.
- **Unborn branch**: empty history, empty branch list, creation disabled;
  **detached HEAD**: no current branch row and a warning line.
- **Dirty worktree checkout**: git's refusal is shown as-is; nothing is
  discarded by the app.
- **Watcher echo**: the existing self-write window still suppresses the
  refresh a write causes, so a stage does not repaint the same diff twice.
- **Open Repository twice**: the second request is ignored while the picker is
  up. The panel is a sheet on the window, and AppKit refuses a second sheet
  there; the caller would wait forever for a dialog that never shows. The
  panel can take a second or two to appear the first time, as its file browser
  starts.
- **Commit draft**: the message survives refreshes and tab switches, and is
  cleared only after a successful commit.
- **Theme**: light/dark follows the system; diff colors must stay legible in
  both.

## Risks

- The native toolkit is younger than the web stack: diff readability,
  keyboard navigation and accessibility need hands-on verification.
- Everything is a rewrite, so the web UI stays in the tree until milestone 5
  and the two UIs must not drift in behaviour.
- `@pierre/diffs` and shadcn are gone; what they gave for free (hunk chrome,
  focus rings, dialogs) has to be rebuilt.
