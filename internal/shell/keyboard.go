package shell

import (
	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// navShortcuts registers the list keys. They are window shortcuts, so they only
// see keys the focused control left: a text field keeps Up, Down and Space.
// docs/keyboard.md lists them.
func (a *App) navShortcuts(c *ui.Context) {
	c.OnShortcut(0, ui.KeyDown, func() { a.moveChoice(1) })
	c.OnShortcut(0, ui.KeyUp, func() { a.moveChoice(-1) })
	c.OnShortcut(0, ui.KeySpace, a.toggleStageChosen)
	c.OnShortcut(ui.Cmd, ui.KeyF, a.requestFilterFocus)
}

// keysIdle reports whether the list keys may act: not while a dialog or the
// branch popover has the keyboard, and only with a repository open.
func (a *App) keysIdle() bool {
	return a.status.Root != "" && !a.confirmOpen && !a.pushOpen && !a.branchPopupOpen
}

// step is the index delta moves to in a list of n rows from cur, where cur is
// -1 when nothing in the list is chosen: the first row on Down, the last on Up,
// and the edges hold instead of wrapping. -1 means there is nowhere to go.
func step(cur, delta, n int) int {
	if n == 0 {
		return -1
	}
	if cur < 0 {
		if delta > 0 {
			return 0
		}
		return n - 1
	}
	return min(max(cur+delta, 0), n-1)
}

// moveChoice moves the chosen row of the current tab and shows it, as a click
// on that row would.
func (a *App) moveChoice(delta int) {
	if !a.keysIdle() {
		return
	}
	switch a.view {
	case ViewChanges:
		cur := -1
		if a.selection != nil {
			for i, ch := range a.changeOrder {
				if sameChange(ch, *a.selection) {
					cur = i
				}
			}
		}
		if next := step(cur, delta, len(a.changeOrder)); next >= 0 {
			a.revealRow = true
			a.loadDiff(a.changeOrder[next])
		}
	case ViewHistory:
		rows := a.historyRows(filterNeedle(a.commitFilter))
		cur := -1
		for i, r := range rows {
			if a.selectedCommit != nil && r.commit.OID == a.selectedCommit.OID {
				cur = i
			}
		}
		if next := step(cur, delta, len(rows)); next >= 0 {
			a.selectCommit(*rows[next].commit)
			a.historyList.ScrollIntoView(next)
		}
	case ViewStashes:
		cur := -1
		for i, s := range a.stashes {
			if a.selectedCommit != nil && s.OID == a.selectedCommit.OID {
				cur = i
			}
		}
		if next := step(cur, delta, len(a.stashes)); next >= 0 {
			a.revealRow = true
			a.selectCommit(stashCommit(a.stashes[next]))
		}
	}
}

// toggleStageChosen stages the chosen file, or unstages it when it is staged.
// It goes through stage and unstage, which refuse while a write runs.
func (a *App) toggleStageChosen() {
	if !a.keysIdle() || a.view != ViewChanges || a.selection == nil {
		return
	}
	ch := *a.selection
	// A selection hidden by a fold or the filter is not what the user is
	// looking at; Space only acts on a row this frame drew.
	drawn := false
	for _, row := range a.changeOrder {
		if sameChange(row, ch) {
			drawn = true
			break
		}
	}
	if !drawn {
		return
	}
	switch {
	case canUnstage(ch):
		a.unstage(ch)
	case canStage(ch):
		a.stage(ch)
	}
}

// requestFilterFocus asks the current tab's filter field to take the focus the
// next time it is drawn. The Stashes tab has no filter.
func (a *App) requestFilterFocus() {
	if !a.keysIdle() || a.view == ViewStashes {
		return
	}
	a.focusFilterOf = a.view
	a.focusFilter = true
}

// takeFilterFocus gives the focus to a tab's filter field when Cmd+F asked for
// it. Each tab calls it with its own view, so a request made on one tab is not
// answered by another.
func (a *App) takeFilterFocus(v View, input ui.Element) {
	if a.focusFilter && a.focusFilterOf == v {
		input.Focus()
		a.focusFilter = false
	}
}

// rememberChange records a file row as drawn, so Up and Down visit exactly the
// rows on screen, in their order.
func (a *App) rememberChange(ch gitx.Change) {
	a.changeOrder = append(a.changeOrder, ch)
}
