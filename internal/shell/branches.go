package shell

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *App) branchView(c *ui.Context, p palette) {
	a.ensureBranches()
	unborn := a.branchUnborn()

	ui.Column(c).FillHeight().FillWidth().Children(func() {
		ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(6).Gap(4).Children(func() {
			ui.Box(c).Grow(1).Children(func() {
				a.takeFilterFocus(ViewBranches, a.filterInput(c, &a.branchTab.filter, "Filter branches"))
			})
			iconButton(c, iconPlus, "New branch", "New branch from HEAD",
				!a.canCreateBranch(), func() { a.startNewBranch(branchCreateTab) })
		})

		if a.creating == branchCreateTab {
			a.newBranchRow(c)
		}

		needle := filterNeedle(a.branchTab.filter)
		switch {
		case a.branchErr != "" && len(a.branches) == 0:
			emptyState(c, "Could not load branches", a.branchErr, "Retry", a.loadBranches)
		case !a.branchesLoaded || (a.loadingBranches && len(a.branches) == 0):
			emptyState(c, "Loading…", "", "", nil)
		default:
			if a.branchErr != "" {
				// A refresh failed but the old list is still usable.
				ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(4).Gap(6).Children(func() {
					ui.Text(c, "Could not refresh branches.").FontSize(10).Grow(1).SingleLine().
						TextColor(p.warnText)
					btn := ui.Button(c, "Retry").Height(20).FontSize(10)
					btn.OnClick(a.loadBranches)
				})
			}
			if a.status.Branch.Detached {
				ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(4).Children(func() {
					ui.Text(c, "Detached HEAD — check out a branch to continue.").
						FontSize(10).TextColor(p.warnText).Wrap()
				})
			}
			if !a.branchTreeView(c, &a.branchTab, 0) {
				title, detail := branchEmpty(needle, unborn)
				emptyState(c, title, detail, "", nil)
			}
		}
	})
}

// branchCreateArea says which surface's inline new-branch field is open. The
// tab and the popover share the draft but must not show it at the same time:
// only one of them is the place the user is working in.
type branchCreateArea int

const (
	branchCreateNone branchCreateArea = iota
	branchCreateTab
	branchCreatePopup
)

// branchUnborn reports a repository with no commit yet: a branch would have
// nothing to point at, so creation is disabled.
func (a *App) branchUnborn() bool {
	b := a.status.Branch
	return a.status.Root != "" && b.OID == "" && !b.Detached
}

// canCreateBranch reports whether a branch can be created from HEAD. An unborn
// repository has no commit to point at, and a write or sync in flight would
// race the switch.
func (a *App) canCreateBranch() bool { return !a.busy() && !a.branchUnborn() }

// branchEmpty explains why the tree shows no branches. The popover shows only
// the title; the tab also has room for the detail.
func branchEmpty(needle string, unborn bool) (title, detail string) {
	switch {
	case needle != "":
		return "No matching branches", ""
	case unborn:
		return "No branches yet", "Branches appear after the first commit."
	default:
		return "No branches", ""
	}
}

// startNewBranch opens the inline name field of one surface, empty and
// focused.
func (a *App) startNewBranch(area branchCreateArea) {
	if !a.canCreateBranch() {
		return
	}
	a.creating = area
	a.newName = ""
	a.focusNew = true
}

// cancelNewBranch closes the inline name field without creating anything.
func (a *App) cancelNewBranch() {
	a.creating = branchCreateNone
	a.newName = ""
	a.focusNew = false
}

// newBranchRow is the inline name field. Enter creates, Escape cancels.
func (a *App) newBranchRow(c *ui.Context) {
	ui.Row(c).FillWidth().Shrink(0).PaddingX(8).PaddingY(5).Gap(4).Children(func() {
		input := ui.TextInput(c, &a.newName).Grow(1).Height(26).FontSize(11).
			PaddingX(6).Font(mono()).Placeholder("new-branch-name")
		if a.focusNew {
			input.Focus()
			a.focusNew = false
		}
		input.OnSubmit(func() { a.submitNewBranch() })
		input.OnShortcut(0, ui.KeyEscape, a.cancelNewBranch)
		iconButton(c, iconCheck, "Create branch", "Create branch",
			!a.canCreateBranch() || strings.TrimSpace(a.newName) == "",
			func() { a.submitNewBranch() })
		iconButton(c, iconX, "Cancel", "Cancel", false, a.cancelNewBranch)
	})
}

func (a *App) submitNewBranch() {
	name := strings.TrimSpace(a.newName)
	if name == "" || !a.canCreateBranch() {
		return
	}
	a.cancelNewBranch()
	a.createBranch(name)
}

func boolColor(on bool, yes, no ui.Color) ui.Color {
	if on {
		return yes
	}
	return no
}
