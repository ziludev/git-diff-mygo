# Stash

Status: implemented 2026-10-29.

## Why

Switching branches or pulling with a dirty work tree fails, and the only way out
was a terminal. A stash puts the uncommitted work aside and brings it back
later; IDEA offers the same as "Stash Changes" and its Stashes tab.

## UX

- A fourth sidebar tab, **Stashes** (Cmd+4), lists the stashes newest first:
  the message and the relative time. The list is read on every refresh (one
  cheap `git stash list`, also after a change the watcher sees), so a stash made
  in a terminal appears, and the tab shows the count when there is one.
- The Changes tab gets a **Stash** icon button next to the filter. It opens the
  name dialog: an optional message, and an **Include untracked files**
  checkbox (on). Enter or the button stashes. The button is disabled when
  there is nothing to stash or a write is running.
- A stash row shows **Apply**, **Pop** and **Drop** while hovered or chosen,
  and the same items in its context menu. Apply puts the changes back and
  keeps the stash; Pop does that and then drops the stash; Drop asks first
  and cannot be undone from the app (the commit stays in git's object store
  until it is garbage collected, which `git fsck --unreachable` can find).
- Choosing a stash shows its files and diff in the right panel, as a commit's
  details do. The panel shows the tracked changes: files that were stashed as
  untracked are in the stash but not in that diff.

## Git commands

| Call | Command |
|---|---|
| List | `git stash list --format=%H%x1f%ct%x1f%gs%x00` |
| Stash | `git stash push [--include-untracked] [-m <message>]` |
| Apply | `git stash apply <oid>` |
| Pop | `git stash apply <oid>`, then `git stash drop stash@{N}` |
| Drop | `git stash drop stash@{N}` |

A stash is identified by its commit id, not by its position: positions shift
when another stash is pushed or dropped, and a click made on a stale list must
not apply or drop a different stash. `apply` accepts the commit id; `drop`
needs the reflog position, so it is looked up again from the id right before
the drop, and the operation fails with "stash no longer exists" when the id is
gone.

## Edge cases

- **Nothing to stash**: `git stash push` exits successfully and prints "No
  local changes to save". The app compares the top stash before and after and
  reports "No local changes to save" as an error instead of pretending.
- **Unborn branch**: git refuses ("You do not have the initial commit yet");
  the message is shown.
- **Apply conflicts**: git leaves the conflicts in the work tree, and unlike a
  cherry-pick there is no `--abort` that restores the earlier state without
  risking uncommitted work. The conflicts show in the Conflicts section, the
  stash is kept (Pop only drops after a clean apply), and the toast says so.
  Resolving them is done outside the app until it has a conflict editor.
- **Dirty work tree on apply**: git refuses when local changes overlap, with
  its message; nothing changes.
- **Stale rows**: apply, pop and drop use the commit id; a stash dropped
  elsewhere gives "stash no longer exists" and the list reloads.
- **Pop after a clean apply, drop fails**: the changes are back and the stash
  remains; the error says the drop failed so the user can drop it by hand.
- **Busy**: stash, apply, pop and drop run through the write slot and are
  disabled while a write or a remote operation runs.
- **Repository switch**: the list and the chosen stash are cleared.

## Not in this change

Keep-index stashes, stashing selected files only, stash branches, and the
untracked files' diff in the right panel.
