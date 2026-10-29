# Branches view

Status: implemented in M2 (2026-10-28); branch popover, prefix tree and remote
branches added 2026-10-29.

## Why

The history list already showed branch and tag labels, but the only way to
change the checked-out branch was the terminal. The branch switcher gives the
sidebar a third view next to Changes and History, and the toolbar chip gives the
same tree a quick popover.

## UX

- The sidebar has three tabs: **Changes**, **History**, **Branches**. Clicking
  the branch chip in the toolbar opens a popover with a search field, the same
  tree and New Branch / Manage Branches; Manage Branches switches to the tab.
  Escape or a press outside closes it; a successful checkout closes it too.
  Opening it reloads the list, because the file watcher ignores `.git` and a
  fetch or branch change made in a terminal is otherwise invisible.
- In the popover, typing filters and the search field keeps the focus: Up/Down
  move the choice through the matching branches and Enter checks the choice out
  (with no choice yet, Enter takes the first match).
- A click chooses a row; a double click, or Enter on a chosen row, acts on it.
  This is the list's own behavior: the row owns its press, so a click cannot
  also be a switch. A chosen folder opens or closes; a chosen branch is checked
  out. The search field's Enter does the same from the keyboard.
- The list has two groups, **Local** then **Remote**. Branch names split on `/`
  into a tree: `feature/login` and `feature/logout` sit under `feature`, and a
  branch named `feature` keeps its own row beside that folder. A remote's first
  segment is the remote's name, so `origin/main` and `upstream/main` do not
  collide; a remote whose own name contains `/` (`foo/bar`) is kept as one
  remote by asking `git remote` instead of guessing. `origin/HEAD` is a
  symbolic ref and is not listed.
- Each branch row shows the branch icon, its last path segment and the tip's
  relative time; the full name and tip subject are the row's tooltip. The
  checked-out row shows a check, "current" and its upstream sync counts
  (`↑2 ↓1`) instead of the time.
- Checking out a branch (double click, Enter, or the context menu) switches to
  it. A remote branch checks out its existing local branch when there is one;
  otherwise it creates the local branch from `refs/remotes/<name>` and sets
  that ref as upstream (best effort: the branch is created even when the remote
  is no longer configured).
- **New branch** opens an inline name field. Enter (or the check button)
  creates the branch at HEAD and checks it out; Escape cancels. The draft
  belongs to one surface: the popover and the tab never show the field at the
  same time, and closing the popover discards it.
- A trash button (shown while a non-current local row is hovered or selected,
  and repeated in the row's context menu) deletes a merged branch after a
  confirmation. `git branch -d` refuses unmerged branches, so the git message
  is shown instead of forcing a delete. Remote branches have no delete:
  removing one is a push.
- Context menus: Checkout (or Checkout as Local Branch for a remote) and, for a
  local branch, Delete Branch…. The current branch has no menu.
- A filter field narrows the tree by full name. When the filter changes, the
  ancestors of every match open once, so a deep branch is visible; after that
  the user can collapse folders even while the filter is active. The tab and
  the popover keep separate open state; each opens its folders on the first
  list. When HEAD moves to a branch in a new folder, that folder is revealed in
  both trees.
- Detached HEAD: no row is current; a banner says "Detached HEAD" and invites a
  checkout.
- Unborn branch (no commit yet): the list is empty and branch creation is
  disabled in both the tab and the popover, because a branch has no commit to
  point at.

## Data flow

```
RepoService.branches()                -> gitx.Branches               -> []BranchEntry
RepoService.checkoutBranch(name)      -> gitx.CheckoutBranch
RepoService.checkoutRemoteBranch(ref) -> gitx.CheckoutRemoteBranch
RepoService.createBranch(name)        -> gitx.CreateBranch
RepoService.deleteBranch(name)        -> gitx.DeleteBranch
```

`internal/shell.App` keeps `branches`, `branchesLoaded`, `loadingBranches` and
`branchErr` next to the worktree state, and two `ui.OutlineState[string]` (tab
and popover) for the trees. Branch actions run through the write serialization
(`writing`), so a checkout cannot race a hunk apply. A failed load keeps the
last successful list and shows a retry row; only a first load with no list
shows the full error state.

## Git commands

| Call | Command |
|---|---|
| List | `git remote` then `git for-each-ref --format=%00%(refname)%1f%(HEAD)%1f%(committerdate:unix)%1f%(symref)%1f%(contents:subject) refs/heads refs/remotes` |
| Checkout | `git checkout --no-guess <name>` |
| Checkout remote | `git checkout -b <local> refs/remotes/<remote>/<branch>` then `git branch --set-upstream-to=refs/remotes/<remote>/<branch> <local>` |
| Create | `git checkout -b <name>` |
| Delete | `git branch -d <name>` |

`%00` starts each record and `%1f` separates its fields (both are hex bytes
that `for-each-ref` expands). Ref names cannot contain either, but a commit
subject is arbitrary text and may contain `%1e` or `%1f`; only NUL is a safe
record separator, and the subject must be the **last** field, so a `%1f` inside
it stays in the subject rather than splitting it. `%(symref)` is non-empty only
for a symbolic ref such as `origin/HEAD`, which the parser drops.

The remote checkout does not use `checkout --track`: git refuses to set up
tracking when the remote ref exists but its remote is not configured (removed
remotes, test fixtures). Creating the branch first always works; the upstream
link is then best effort. The start point is the fully qualified
`refs/remotes/<name>`, which cannot be confused with a local branch or tag
named like `origin/main`. The app rejects an empty name or one starting with
`-` (which git would read as an option) and leaves ref syntax to git's own
validation.

The list deliberately omits `%(upstream:track)`: git answers it by walking
each branch's history, and the panel only needs counts for the checked-out
branch, which `Status` already reports.

## Edge cases

- **Dirty work tree**: checkout fails exactly when git would overwrite local
  changes; the git message is toasted and nothing is discarded.
- **Unmerged branch**: `-d` refuses and the message is shown; force deletion is
  not offered.
- **Detached HEAD**: every branch is non-current; checkout reattaches.
- **Sync counts**: only the checked-out branch shows them. Other branches
  would each need a rev-list walk, which a branch switcher does not need.
- **Remote with a local counterpart**: the local branch is checked out instead
  of creating another one, which git would refuse anyway.
- **Remote without a configured remote**: the local branch is still created;
  only the upstream link is skipped.
- **Remote name with `/`**: `git remote` gives the border, so `foo/bar/topic`
  becomes remote `foo/bar`, branch `topic`, and checks out as `topic`.
- **Ambiguous short name**: a local branch or tag named `origin/main` does not
  change which ref a remote checkout starts from.
- **Stale list**: opening the popover or the tab reloads it, and a failed
  checkout reloads it, so a branch created outside the app is seen before the
  retry. A failed load keeps the previous list and only the next attempt needs
  the Retry row.
- **Other refs**: tags are not rows; remote symbolic refs are dropped.
- **Refresh**: checkout, create and delete reload the status, the first
  history page (labels live on commits) and the branch list. A commit goes
  through the same reload.
- **Watcher**: `.git` is ignored by the file watcher, so an external
  `git checkout` is seen through the worktree files it rewrites. Once the
  branch list has been loaded it follows every watcher batch, which costs one
  `for-each-ref` over local and remote refs. A watcher refresh does not close
  the popover.

## Not in this change

Rename, merge, rebase, branch creation from a commit in the history list,
"new branch from here" in the popover, and copying a branch name.
