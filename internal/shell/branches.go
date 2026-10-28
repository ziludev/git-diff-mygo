package shell

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

func (a *App) branchView(c *ui.Context, p palette) {
	a.ensureBranches()
	unborn := a.status.Root != "" && a.status.Branch.OID == "" && !a.status.Branch.Detached

	ui.Column(c).FillHeight().FillWidth().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(6).Gap(4).Children(func() {
			ui.Box(c).Grow(1).Children(func() {
				a.filterInput(c, &a.branchFilter, "Filter branches")
			})
			iconButton(c, "+", "New branch from HEAD", a.writing || unborn, func() {
				a.creating = true
				a.newName = ""
				a.focusNew = true
			})
		})

		if a.creating {
			a.newBranchRow(c, p)
		}

		needle := filterNeedle(a.branchFilter)
		visible := make([]gitx.BranchEntry, 0, len(a.branches))
		for _, b := range a.branches {
			if nameMatches(needle, b.Name) {
				visible = append(visible, b)
			}
		}

		switch {
		case a.branchErr != "" && len(a.branches) == 0:
			emptyState(c, "Could not load branches", a.branchErr, "Retry", func() {
				a.branchErr = ""
				a.loadBranches()
			})
		case !a.branchesLoaded || (a.loadingBranches && len(a.branches) == 0):
			emptyState(c, "Loading…", "", "", nil)
		default:
			if a.branchErr != "" {
				// A refresh failed but the old list is still usable.
				ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(4).Gap(6).Children(func() {
					ui.Text(c, "Could not refresh branches.").FontSize(10).Grow(1).SingleLine().
						TextColor(p.warnText)
					btn := ui.Button(c, "Retry").Height(20).FontSize(10)
					btn.OnClick(func() {
						a.branchErr = ""
						a.loadBranches()
					})
				})
			}
			a.scrollColumn(c, func() {
				if a.status.Branch.Detached {
					ui.Row(c).FillWidth().PaddingX(8).PaddingY(4).Children(func() {
						ui.Text(c, "Detached HEAD — check out a branch to continue.").
							FontSize(10).TextColor(p.warnText).Wrap()
					})
				}
				if len(visible) == 0 {
					if needle != "" {
						emptyState(c, "No matching branches", "", "", nil)
					} else if unborn {
						emptyState(c, "No branches yet", "Branches appear after the first commit.", "", nil)
					} else {
						emptyState(c, "No branches", "", "", nil)
					}
					return
				}
				for _, b := range visible {
					a.branchRow(c, p, b)
				}
			})
		}
	})
}

// newBranchRow is the inline name field. Enter creates, Escape cancels.
func (a *App) newBranchRow(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(5).Gap(4).Children(func() {
		input := ui.TextInput(c, &a.newName).Grow(1).Height(26).FontSize(11).
			PaddingX(6).Font(mono()).Placeholder("new-branch-name")
		if a.focusNew {
			input.Focus()
			a.focusNew = false
		}
		input.OnSubmit(func() { a.submitNewBranch() })
		input.OnShortcut(0, ui.KeyEscape, func() {
			a.creating = false
			a.newName = ""
		})
		iconButton(c, "✓", "Create branch", a.writing || strings.TrimSpace(a.newName) == "",
			func() { a.submitNewBranch() })
		iconButton(c, "✕", "Cancel", false, func() {
			a.creating = false
			a.newName = ""
		})
	})
}

func (a *App) submitNewBranch() {
	name := strings.TrimSpace(a.newName)
	if name == "" || a.writing {
		return
	}
	a.creating = false
	a.newName = ""
	a.createBranch(name)
}

// branchRow checks out on a click in its left part; delete is a separate
// button, so pressing trash cannot also check the branch out.
func (a *App) branchRow(c *ui.Context, p palette, b gitx.BranchEntry) {
	t := c.Theme()
	row := ui.Row(c).FillWidth().Height(30).PaddingX(8).Gap(4).Clip()
	if b.Current {
		row.Background(t.Background)
	}
	row.Children(func() {
		left := ui.Row(c).Grow(1).Gap(6).ClipX().Cursor(ui.CursorPointer)
		left.Children(func() {
			ui.Text(c, "⎇").FontSize(11).
				TextColor(boolColor(b.Current, t.Text, t.TextMuted))
			ui.Text(c, b.Name).Font(mono()).FontSize(11).Grow(1).SingleLine()
			if b.Current {
				ui.Text(c, "✓ current").FontSize(9).TextColor(t.TextMuted)
				if a.status.Branch.Ahead > 0 || a.status.Branch.Behind > 0 {
					ui.Text(c, syncLabel(a.status.Branch)).Font(mono()).FontSize(9).
						TextColor(t.TextMuted)
				}
			}
			ui.Text(c, relativeTime(b.Time)).FontSize(9).TextColor(t.TextMuted)
		})
		left.Tooltip(b.Subject)
		if !b.Current {
			left.OnClick(func() {
				if !a.writing {
					a.checkoutBranch(b.Name)
				}
			})
		}

		if !b.Current {
			iconButton(c, "🗑", "Delete branch "+b.Name, a.writing, func() {
				a.deleteBranch(b.Name)
			})
		}
	})
}

func boolColor(on bool, yes, no ui.Color) ui.Color {
	if on {
		return yes
	}
	return no
}
