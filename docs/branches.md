# Branches view

Status: implemented in M2 (2026-10-28).

## Why

The history list already showed branch and tag labels, but the only way to
change the checked-out branch was the terminal. The branch switcher gives the
sidebar a third view next to Changes and History.

## UX

- The sidebar has three tabs: **Changes**, **History**, **Branches**. Clicking
  the current branch name in the toolbar also opens the Branches tab.
- Branches lists local branches: the checked-out one first, the rest by name.
  Each row shows the branch icon, name and the tip's relative time; the tip
  subject is the row's tooltip. The checked-out row also shows its upstream
  sync counts (`↑2 ↓1`), which come from the status call the app already makes.
- Clicking a non-current row checks it out. Clicking the current row does
  nothing.
- **New branch** opens an inline name field. Enter (or the check button)
  creates the branch at HEAD and checks it out; Escape cancels.
- A trash button (shown on every non-current row) deletes a merged branch after
  a confirmation. `git branch -d` refuses unmerged branches, so the git message
  is shown instead of forcing a delete.
- A filter field narrows the list by name, like the changes panel.
- Detached HEAD: no row is current; a banner says "Detached HEAD" and invites a
  checkout.
- Unborn branch (no commit yet): the list is empty and branch creation is
  disabled, because a branch has no commit to point at.

## Data flow

```
RepoService.branches()            -> gitx.Branches       -> []BranchEntry
RepoService.checkoutBranch(name)  -> gitx.CheckoutBranch
RepoService.createBranch(name)    -> gitx.CreateBranch
RepoService.deleteBranch(name)    -> gitx.DeleteBranch
```

`internal/shell.App` keeps `branches`, `branchesLoaded`, `loadingBranches` and
`branchErr` next to the worktree state. Branch actions run through the write
serialization (`writing`), so a checkout cannot race a hunk apply. A failed
load keeps the last successful list and shows a retry row; only a first load
with no list shows the full error state.

## Git commands

| Call | Command |
|---|---|
| List | `git for-each-ref --format=%00%(refname:short)%1f%(HEAD)%1f%(committerdate:unix)%1f%(contents:subject) refs/heads` |
| Checkout | `git checkout --no-guess <name>` |
| Create | `git checkout -b <name>` |
| Delete | `git branch -d <name>` |

`%00` starts each record and `%1f` separates its fields (both are hex bytes
that `for-each-ref` expands). Ref names cannot contain either, but a commit
subject is arbitrary text and may contain `%1e` or `%1f`; only NUL is a safe
record separator, and the subject is the last field, so a `%1f` inside it
stays in the subject rather than splitting it.
`--no-guess` stops `git checkout` from inventing a branch from a remote ref:
the panel only lists local branches. The app rejects an empty name or one
starting with `-` (which git would read as an option) and leaves ref syntax to
git's own validation.

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
- **Other refs**: only `refs/heads` is listed. Remote-tracking branches and
  tags are not checkable rows.
- **Refresh**: checkout, create and delete reload the status, the first
  history page (labels live on commits) and the branch list. A commit goes
  through the same reload.
- **Watcher**: `.git` is ignored by the file watcher, so an external
  `git checkout` is seen through the worktree files it rewrites. Once the
  branch list has been loaded it follows every watcher batch, which costs one
  `for-each-ref` over local refs.

## Not in this change

Remote-branch checkout, rename, merge, rebase, and branch creation from a
commit in the history list.
