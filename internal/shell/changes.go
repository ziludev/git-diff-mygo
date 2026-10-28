package shell

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// changeSection is one bucket of the changes list.
type changeSection struct {
	title string
	items []gitx.Change
}

// changesView is the working tree: conflicts, the index and the worktree,
// with the commit box under the list.
func (a *App) changesView(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Column(c).FillHeight().FillWidth().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(6).Children(func() {
			a.filterInput(c, &a.changeFilter, "Filter files")
		})

		needle := filterNeedle(a.changeFilter)
		match := func(ch gitx.Change) bool { return nameMatches(needle, ch.Path) }
		sections := []changeSection{
			{"Conflicts", filterChanges(a.status.Conflicted, match)},
			{"Staged", filterChanges(a.status.Staged, match)},
			{"Changes", filterChanges(a.status.Unstaged, match)},
			{"Untracked", filterChanges(a.status.Untracked, match)},
		}
		empty := true
		for _, s := range sections {
			if len(s.items) > 0 {
				empty = false
				break
			}
		}

		a.scrollColumn(c, func() {
			if empty {
				if needle != "" {
					emptyState(c, "No matching files", "", "", nil)
				} else {
					emptyState(c, "Working tree clean", "", "", nil)
				}
				return
			}
			for _, section := range sections {
				if len(section.items) == 0 {
					continue
				}
				ui.Row(c).FillWidth().Height(24).PaddingX(8).Gap(6).Children(func() {
					ui.Text(c, strings.ToUpper(section.title)).FontSize(9).Bold().
						TextColor(t.TextMuted)
					ui.Text(c, strconv.Itoa(len(section.items))).FontSize(9).TextColor(t.TextMuted)
				})
				for _, ch := range section.items {
					a.changeRow(c, p, ch)
				}
			}
		})

		ui.Divider(c)
		a.commitBox(c, p)
	})
}

func filterChanges(items []gitx.Change, keep func(gitx.Change) bool) []gitx.Change {
	out := make([]gitx.Change, 0, len(items))
	for _, ch := range items {
		if keep(ch) {
			out = append(out, ch)
		}
	}
	return out
}

// changeRow is a clickable path plus its stage/unstage/discard buttons. The
// buttons sit outside the clickable part, so pressing one does not also
// select the row.
func (a *App) changeRow(c *ui.Context, p palette, ch gitx.Change) {
	t := c.Theme()
	selected := a.selection != nil && sameChange(*a.selection, ch)
	code := changeLetter(ch)
	tip := pathLabel(ch.Path, ch.OrigPath)

	row := ui.Row(c).FillWidth().Height(28).PaddingX(8).Gap(6).Clip()
	if selected {
		row.Background(t.Background)
	}
	row.Tooltip(tip)
	row.Children(func() {
		left := ui.Row(c).Grow(1).Gap(6).ClipX().Cursor(ui.CursorPointer)
		left.Children(func() {
			ui.Text(c, code).Font(mono()).FontSize(10).Bold().
				Width(12).TextAlign(ui.Center).TextColor(statusColor(p, code))
			ui.Text(c, ch.Path).Font(mono()).FontSize(11).SingleLine()
		})
		left.OnClick(func() { a.loadDiff(ch) })

		ui.Row(c).Shrink(0).Gap(2).Children(func() {
			// A remote operation blocks writes too, so its buttons disable with
			// the same flag instead of silently doing nothing.
			if !ch.Staged && !ch.Conflicted {
				iconButton(c, "+", "Stage", a.busy(), func() { a.stage(ch) })
			}
			if ch.Staged {
				iconButton(c, "−", "Unstage", a.busy(), func() { a.unstage(ch) })
			}
			if !ch.Staged && !ch.Untracked && !ch.Conflicted {
				iconButton(c, "↺", "Discard changes", a.busy(), func() { a.discard(ch) })
			}
		})
	})
}

// commitBox is the message and the commit button. The message survives a
// failed commit and every refresh; only a successful commit clears it.
func (a *App) commitBox(c *ui.Context, p palette) {
	t := c.Theme()
	staged := len(a.status.Staged)
	target := a.status.Branch.Head
	if target == "" {
		if a.status.Branch.Detached {
			target = "detached HEAD"
		} else {
			target = "branch"
		}
	}
	ui.Column(c).FillWidth().Shrink(0).Padding(8).Gap(6).Children(func() {
		message := ui.TextArea(c, &a.message).FillWidth().Height(64).FontSize(12).
			Placeholder("Commit message").Disabled(a.writing).Padding(6)
		// Cmd+Enter commits from the text area itself: a window shortcut does
		// not see the key while the input has the focus.
		message.OnTextKey(func(k ui.TextKey) bool {
			if k.Key != ui.KeyEnter || k.Mods != ui.Cmd {
				return false
			}
			a.message = k.Text()
			a.commit()
			return true
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Checkbox(c, &a.amend, "Amend").FontSize(10).
				TextColor(t.TextMuted).Disabled(a.writing)
			ui.Spacer(c)
			label := "Commit to " + target
			btn := ui.PrimaryButton(c, label).Height(26).FontSize(11).
				Disabled(!a.canCommit())
			btn.Tooltip(label)
			btn.OnClick(a.commit)
		})
		hint := "Nothing staged"
		if staged > 0 {
			if staged == 1 {
				hint = "1 staged file"
			} else {
				hint = strconv.Itoa(staged) + " staged files"
			}
		}
		ui.Text(c, hint+" · Cmd+Enter").FontSize(9).TextColor(t.TextMuted)
	})
}

func changeLetter(ch gitx.Change) string {
	code := ch.Y
	if ch.Staged {
		code = ch.X
	}
	if code == "" || code == "." {
		return "?"
	}
	return code
}

func statusColor(p palette, code string) ui.Color {
	switch code {
	case "A":
		return p.addText
	case "M":
		return p.warnText
	case "D":
		return p.delText
	case "R", "C":
		return p.coolText
	default:
		return p.metaText
	}
}

func iconButton(c *ui.Context, glyph, tip string, disabled bool, onClick func()) ui.Element {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(20, 20).Radius(4).Center().
		Cursor(ui.CursorPointer).Disabled(disabled)
	btn.Children(func() {
		ui.Text(c, glyph).FontSize(12).TextColor(t.TextMuted)
	})
	btn.Tooltip(tip)
	btn.Label(tip)
	btn.OnClick(onClick)
	return btn
}
