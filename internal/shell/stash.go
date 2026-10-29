package shell

import (
	"context"
	"log"
	"strconv"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// showsCommitPanel reports whether the right panel shows a commit's details,
// files and diff: the History tab, and the Stashes tab, where a stash is read
// as the commit it is.
func (a *App) showsCommitPanel() bool {
	return a.view == ViewHistory || a.view == ViewStashes
}

// loadStashes reads the stash list. It is one cheap command, so every refresh
// does it and the tab's count stays right after a stash made in a terminal.
func (a *App) loadStashes() {
	root := a.status.Root
	if root == "" {
		return
	}
	epoch := a.epochs.stashes.Add(1)
	a.loadingStashes = true
	go func() {
		list, err := a.svc.Stashes(context.Background())
		a.apply(func() {
			if epoch != a.epochs.stashes.Load() || a.status.Root != root {
				return
			}
			a.loadingStashes = false
			a.stashesLoaded = true
			if err != nil {
				log.Printf("shell: list stashes for %q failed: %v", root, err)
				a.stashErr = err.Error()
				return
			}
			a.stashErr = ""
			a.stashes = list
		})
	}()
}

// stashCommit presents a stash as the commit it is, so the right panel can
// show its files and diff. Its first parent is the commit the work was
// stashed on, which makes the diff exactly the tracked changes.
func stashCommit(s gitx.StashEntry) gitx.Commit {
	return gitx.Commit{OID: s.OID, Short: s.Short, Subject: s.Message, Author: "stash", Time: s.Time}
}

// stashView lists the stashes, newest first.
func (a *App) stashView(c *ui.Context, p palette) {
	switch {
	case a.stashErr != "" && len(a.stashes) == 0:
		emptyState(c, "Could not read the stashes", a.stashErr, "", nil)
	case !a.stashesLoaded:
		emptyState(c, "Loading…", "", "", nil)
	case len(a.stashes) == 0:
		emptyState(c, "No stashes", "Stash Changes in the Changes tab puts uncommitted work aside.", "", nil)
	default:
		a.scrollColumn(c, func() {
			for _, s := range a.stashes {
				a.stashRow(c, p, s)
			}
		})
	}
}

// stashRow is one stash: its message and age, with Apply, Pop and Drop while
// the row is hovered or chosen. The menu keeps them reachable otherwise.
func (a *App) stashRow(c *ui.Context, p palette, s gitx.StashEntry) {
	t := c.Theme()
	selected := a.selectedCommit != nil && a.selectedCommit.OID == s.OID
	row := ui.Row(c).FillWidth().Height(30).PaddingX(8).Gap(6).Clip()
	hovered := rowTint(row, t, selected)
	if selected && a.revealRow {
		row.ScrollIntoView()
		a.revealRow = false
	}
	row.Tooltip(s.Message)
	row.Children(func() {
		left := ui.Row(c).Grow(1).Gap(8).ClipX().Cursor(ui.CursorPointer)
		left.Children(func() {
			icon(c, iconArchive).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, s.Message).FontSize(12).Grow(1).SingleLine()
			ui.Text(c, relativeTime(s.Time)).FontSize(10).TextColor(t.TextMuted)
		})
		left.OnClick(func() { a.selectCommit(stashCommit(s)) })
		if hovered || selected {
			ui.Row(c).Shrink(0).Gap(2).Children(func() {
				iconButton(c, iconCheck, "Apply", "Apply the stash and keep it", a.busy(), func() { a.applyStash(s) })
				iconButton(c, iconPull, "Pop", "Apply the stash and drop it", a.busy(), func() { a.popStash(s) })
				iconButton(c, iconTrash, "Drop", "Delete the stash", a.busy(), func() { a.dropStash(s) })
			})
		}
	})
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("Apply").Disabled(a.busy()).Chosen() {
			a.applyStash(s)
		}
		if m.Item("Pop").Disabled(a.busy()).Chosen() {
			a.popStash(s)
		}
		m.Separator()
		if m.Item("Drop…").Disabled(a.busy()).Chosen() {
			a.dropStash(s)
		}
	})
}

// canStash mirrors the Stash button: there is something to put aside, on a
// branch that has a commit, and nothing else is writing.
func (a *App) canStash() bool {
	return !a.busy() && a.status.Branch.OID != "" &&
		len(a.status.Conflicted)+len(a.status.Staged)+len(a.status.Unstaged)+len(a.status.Untracked) > 0
}

// stashChanges asks for an optional message and whether new files go along,
// then stashes.
func (a *App) stashChanges() {
	if !a.canStash() {
		return
	}
	a.promptAction(promptSpec{
		title:       "Stash changes",
		detail:      "Uncommitted changes are put aside and the work tree returns to the last commit.",
		label:       "Stash",
		placeholder: "message (optional)",
		option:      "Include untracked files",
		optional:    true,
	}, func(message string, untracked bool) {
		a.write(func() error {
			return a.svc.StashPush(context.Background(), message, untracked)
		})
	})
}

func (a *App) applyStash(s gitx.StashEntry) {
	a.write(func() error { return a.svc.StashApply(context.Background(), s.OID) })
}

func (a *App) popStash(s gitx.StashEntry) {
	a.write(func() error { return a.svc.StashPop(context.Background(), s.OID) })
	a.forgetStash(s)
}

func (a *App) dropStash(s gitx.StashEntry) {
	a.confirmAction("Drop "+quoteShort(s.Message)+"?",
		"The stash is deleted. Its changes can only be recovered with git fsck.",
		"Drop", func() {
			a.write(func() error { return a.svc.StashDrop(context.Background(), s.OID) })
			a.forgetStash(s)
		})
}

// forgetStash clears the right panel when it shows a stash that is going away.
func (a *App) forgetStash(s gitx.StashEntry) {
	if a.selectedCommit != nil && a.selectedCommit.OID == s.OID {
		a.resetCommitSelection()
	}
}

// quoteShort quotes a stash message for a dialog title, shortened.
func quoteShort(s string) string {
	if len(s) > 40 {
		s = s[:40] + "…"
	}
	return strconv.Quote(s)
}
