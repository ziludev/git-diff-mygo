# Keyboard

Status: implemented 2026-10-29.

## Why

Rows are drawn as plain views, so Tab never reached them and the file list,
the history and the stashes could only be worked with the mouse. These keys
cover the common loop (move through the list, look at the diff, stage it)
without adding focus stops to every row.

## Keys

All of them are window shortcuts: they act only on keys no focused control took.
Space is text in any text field, and Up/Down move the caret in the multi-line
commit message, so those keep them; a focused button keeps Enter and Space, and
the split divider keeps the arrows while it has the focus. A one-line field such
as a filter has no use for Up/Down, so they pass through it: type a filter,
then press Down to start walking the matches, as in the branch popover. None of
them act while a dialog or the branch popover is open.

| Key | Changes | History | Stashes | Branches |
|---|---|---|---|---|
| Up / Down | previous / next file | previous / next commit | previous / next stash | the tree's own keys when it has focus |
| Space | stage the chosen file, or unstage it when it is staged | | | |
| Cmd+F | focus the file filter | focus the commit filter | | focus the branch filter |
| Cmd+1…4 | switch tab (already existed) | | | |
| Cmd+Enter | commit (already existed) | | | |

- **Up / Down** move the choice and show its diff, exactly like clicking the
  row. The order is the one on screen: the sections top to bottom, folders in
  the tree's order, files inside closed folders and the closed Untracked
  section skipped, and only the commits that pass the filter. With nothing
  chosen, Down takes the first row and Up the last; at either end the choice
  stays put (no wrap-around). The chosen row scrolls into view.
- **Space** stages an unstaged or untracked file and unstages a staged one; a
  conflicted file does nothing, and so does a selection the list no longer
  draws (its folder was closed or the filter hides it). It goes through the same guard as the buttons,
  so it does nothing while a write or a remote operation runs. The choice
  follows the file into its new section.
- **Cmd+F** puts the caret in the current tab's filter field; Stashes has none.
  Escape is left to the field: it does not clear the text.

## How the order is known

A file list is derived from the status, with folders and sections that may be
closed. The sections and their trees are rebuilt only when the status, the
filter or the layout changes, so a frame walks them instead of rebuilding them.
The view records the files it drew, in order, and the Up and Down handlers read
that list, so what the keys visit is what the user sees, with no second copy of
the folding rules to keep in step.

## Edge cases

- **A chosen row that is no longer drawn** (its folder was closed, or the
  filter hides it): Down takes the first drawn row and Up the last.
- **An empty list**: the keys do nothing.
- **A load in flight**: keys keep working; each choice supersedes the last
  diff load through its epoch, so holding Down never shows a stale patch.
- **Selection by object id in History**: moving through commits uses the same
  `selectCommit`, so details, files and diff reload together.

## Not in this change

Left/Right to close and open folders, Home/End and Page Up/Down, discard by
key (the one destructive row action stays a click and a confirmation),
focusable rows and a Tab order, and clearing a filter with Escape.
