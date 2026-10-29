package shell

import (
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// paneLayout arranges the file list and the diff pane.
type paneLayout int

const (
	panesSideBySide paneLayout = iota
	panesStacked
)

// palette holds the colors the diff and status letters need, which the theme
// does not name: they follow the theme's light or dark appearance.
type palette struct {
	addBg    ui.Color
	delBg    ui.Color
	hunkBg   ui.Color
	metaText ui.Color
	addText  ui.Color
	delText  ui.Color
	warnText ui.Color
	coolText ui.Color
}

func paletteFor(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			addBg:    ui.RGBA(34, 197, 94, 0.16),
			delBg:    ui.RGBA(239, 68, 68, 0.16),
			hunkBg:   ui.RGBA(255, 255, 255, 0.06),
			metaText: t.TextMuted,
			addText:  ui.Hex("#4ade80"),
			delText:  ui.Hex("#f87171"),
			warnText: ui.Hex("#fbbf24"),
			coolText: ui.Hex("#38bdf8"),
		}
	}
	return palette{
		addBg:    ui.RGBA(22, 163, 74, 0.12),
		delBg:    ui.RGBA(220, 38, 38, 0.10),
		hunkBg:   ui.RGBA(0, 0, 0, 0.04),
		metaText: t.TextMuted,
		addText:  ui.Hex("#15803d"),
		delText:  ui.Hex("#b91c1c"),
		warnText: ui.Hex("#b45309"),
		coolText: ui.Hex("#0369a1"),
	}
}

var (
	monoOnce sync.Once
	monoName string
)

// mono returns a monospaced family for paths and patches. The first lookup
// walks the system's fonts, so it happens once.
func mono() string {
	monoOnce.Do(func() {
		preferred := []string{"SF Mono", "Menlo", "Monaco", "JetBrains Mono", "Fira Code", "Cascadia Mono", "Consolas"}
		families := ui.FontFamilies()
		for _, want := range preferred {
			for _, f := range families {
				if strings.EqualFold(f.Name, want) {
					monoName = f.Name
					return
				}
			}
		}
		for _, f := range families {
			if f.Monospaced {
				monoName = f.Name
				return
			}
		}
	})
	return monoName
}

// View builds one frame of the window.
func (a *App) View(c *ui.Context) {
	t := c.Theme()
	p := paletteFor(t)
	ui.Column(c).Fill().Background(t.Background).Children(func() {
		a.toolbar(c, p)
		ui.Divider(c)
		sidebar := func() { a.sidebarView(c, p) }
		diff := func() { a.diffView(c, p) }
		if a.panes == panesStacked {
			ui.SplitVertical(c, &a.sidebarHeight, sidebar, diff).Grow(1)
		} else {
			ui.Split(c, &a.sidebar, sidebar, diff).Grow(1)
		}
		if a.toast != "" {
			a.toastBar(c, p)
		}
	})
	if a.confirmOpen {
		a.modal(c, p)
	}
	if a.pushOpen {
		a.pushModal(c)
	}
	a.shortcuts(c)
}

func (a *App) shortcuts(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyEnter, a.commit)
	// Cmd+O belongs to the native menu's Open Repository item; registering it
	// here too would open two pickers.
	c.OnShortcut(ui.Cmd, ui.Key1, func() { a.show(ViewChanges) })
	c.OnShortcut(ui.Cmd, ui.Key2, func() { a.show(ViewHistory) })
	c.OnShortcut(ui.Cmd, ui.Key3, func() { a.show(ViewBranches) })
	c.OnShortcut(ui.Cmd, ui.Key4, func() { a.show(ViewStashes) })
	a.navShortcuts(c)
}

func (a *App) toolbar(c *ui.Context, p palette) {
	t := c.Theme()
	hasRepo := a.status.Root != ""
	ui.Row(c).FillWidth().Shrink(0).Height(42).PaddingX(10).Gap(8).
		Background(t.Surface).Children(func() {
		ui.Text(c, "Twill").Bold().FontSize(13)
		if hasRepo {
			ui.Text(c, repoName(a.status.Root)).FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			a.syncState(c)
			a.branchChip(c, p)
			a.syncButtons(c)
			ui.Divider(c)
			iconButton(c, iconRefresh, "Refresh", "Refresh the status and history", a.actions == nil, func() {
				if a.actions != nil {
					a.actions.RefreshFromMenu()
				}
			})
			a.moreMenu(c)
		} else {
			ui.Spacer(c)
		}
		open := ui.PrimaryButton(c, "Open Repository")
		open.Height(26).FontSize(12).OnClick(a.openPicker)
	})
}

// syncState names the operation in flight, which an icon alone cannot say. A
// network operation wins over a status reload: it is the one the user just
// started, and a pull reloads the status anyway.
func (a *App) syncState(c *ui.Context) {
	t := c.Theme()
	switch {
	case a.syncing:
		ui.Text(c, syncText(a.syncOp)).FontSize(11).TextColor(t.TextMuted)
	case a.loadingStatus:
		ui.Text(c, "Refreshing…").FontSize(11).TextColor(t.TextMuted)
	}
}

// syncText is the toolbar's label while a network operation runs.
func syncText(op string) string {
	switch op {
	case "fetch":
		return "Fetching…"
	case "pull":
		return "Pulling…"
	case "push":
		return "Pushing…"
	}
	return "Syncing…"
}

// syncButtons draws Fetch, Pull and Push as icons. They share one slot, so all
// three are disabled while any of them runs or a worktree write is in flight;
// pull and push also need a checked-out branch with a commit, which a detached
// HEAD and an unborn branch do not have.
func (a *App) syncButtons(c *ui.Context) {
	branch := a.hasSyncBranch()
	busy := a.busy()
	ui.Row(c).Shrink(0).Gap(2).Children(func() {
		for _, op := range []struct {
			icon    iconName
			label   string
			tooltip string
			enabled bool
			run     func()
		}{
			{iconFetch, "Fetch", "Fetch from the remote and prune deleted refs", true, a.fetchRemote},
			{iconPull, "Pull", "Pull: fast-forward the current branch from its upstream", branch, a.pullRemote},
			{iconPush, "Push", "Review the commits to push, then push the current branch", branch, a.openPush},
		} {
			iconButton(c, op.icon, op.label, op.tooltip, busy || !op.enabled, op.run)
		}
	})
}

// moreMenu holds the actions that do not deserve toolbar width. The pane
// arrangement is a checked pair so the current one is visible.
func (a *App) moreMenu(c *ui.Context) {
	btn := iconButton(c, iconMore, "More actions", "Pane layout and other actions", false, nil)
	btn.Menu(func(m *ui.Menu) {
		if m.Item("Side by side panes").Checked(a.panes == panesSideBySide).Chosen() {
			a.panes = panesSideBySide
		}
		if m.Item("Stacked panes").Checked(a.panes == panesStacked).Chosen() {
			a.panes = panesStacked
		}
		m.Separator()
		if m.Item("Group files by directory").Checked(a.changesTree).Chosen() {
			a.changesTree = !a.changesTree
		}
	})
}

// branchPopupTreeHeight caps the popover's tree; more branches scroll inside it
// instead of growing past the window.
const branchPopupTreeHeight float32 = 320

// branchChip shows the checked-out branch and its upstream counts; clicking it
// opens the branch popover for a quick switch. The Branches tab stays the
// full management view.
func (a *App) branchChip(c *ui.Context, p palette) {
	t := c.Theme()
	b := a.status.Branch
	name := b.Head
	if name == "" {
		if b.Detached {
			name = "detached HEAD"
		} else {
			name = "no branch"
		}
	}
	chip := ui.ButtonBase(c).Height(26).PaddingX(8).Radius(6).
		Background(t.Background).Border(1, t.Border).Gap(5).Cursor(ui.CursorPointer)
	chip.Children(func() {
		icon(c, iconBranch).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, name).FontSize(11)
		if s := syncLabel(b); s != "" {
			ui.Text(c, s).FontSize(10).TextColor(t.TextMuted)
		}
		icon(c, iconChevronDown).FontSize(10).TextColor(t.TextMuted)
	})
	chip.Tooltip("Switch, create or delete a branch")
	chip.Label("Current branch: " + name)
	chip.OnClick(func() {
		a.branchPopupOpen = !a.branchPopupOpen
		if !a.branchPopupOpen {
			a.cancelNewBranch()
			return
		}
		// The search, the choice and a draft start fresh; the folders stay
		// where the user left them. The watcher ignores .git, so an external
		// fetch or branch change is only visible after a reload.
		a.branchPopupTree.filter = ""
		a.branchPopupTree.row = -1
		a.focusBranchFilter = true
		a.cancelNewBranch()
		a.refreshBranches()
	})
	wasOpen := a.branchPopupOpen
	ui.Popover(c, chip, &a.branchPopupOpen, func() {
		a.branchPopupPanel(c, p)
	})
	if wasOpen && !a.branchPopupOpen {
		// Escape or a press outside closed it; a half-typed branch name does
		// not belong to the next place the user goes.
		a.cancelNewBranch()
	}
}

// branchPopupPanel is the quick switcher: search, the same tree as the tab,
// and the two ways out of a switch (create a branch, or manage branches).
func (a *App) branchPopupPanel(c *ui.Context, p palette) {
	t := c.Theme()
	unborn := a.branchUnborn()
	ui.Column(c).Width(340).Gap(6).Children(func() {
		input := a.filterInput(c, &a.branchPopupTree.filter, "Search branches")
		if a.focusBranchFilter {
			input.Focus()
			a.focusBranchFilter = false
		}
		// The field keeps the focus while the arrows choose a branch, so
		// typing and choosing never fight; Enter switches to the choice.
		input.OnShortcut(0, ui.KeyDown, func() { a.moveBranchPopupRow(1) })
		input.OnShortcut(0, ui.KeyUp, func() { a.moveBranchPopupRow(-1) })
		input.OnSubmit(a.activateBranchPopupRow)
		if a.creating == branchCreatePopup {
			a.newBranchRow(c)
		}
		needle := filterNeedle(a.branchPopupTree.filter)
		if !a.branchTreeView(c, &a.branchPopupTree, branchPopupTreeHeight) {
			switch {
			case a.branchErr != "" && len(a.branches) == 0:
				ui.Row(c).FillWidth().Padding(8).Gap(6).Children(func() {
					ui.Text(c, "Could not load branches").FontSize(11).Grow(1).
						TextColor(p.warnText).SingleLine()
					retry := ui.Button(c, "Retry").Height(20).FontSize(10)
					retry.OnClick(a.loadBranches)
				})
			case !a.branchesLoaded || (a.loadingBranches && len(a.branches) == 0):
				ui.Text(c, "Loading…").FontSize(11).TextColor(t.TextMuted).Padding(8)
			default:
				title, _ := branchEmpty(needle, unborn)
				ui.Text(c, title).FontSize(11).TextColor(t.TextMuted).Padding(8)
			}
		}
		ui.Divider(c)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			create := ui.Button(c, "New Branch").Height(24).FontSize(10).
				Disabled(!a.canCreateBranch())
			create.OnClick(func() { a.startNewBranch(branchCreatePopup) })
			ui.Spacer(c)
			manage := ui.Button(c, "Manage Branches").Height(24).FontSize(10)
			manage.OnClick(func() {
				a.cancelNewBranch()
				a.branchPopupOpen = false
				a.show(ViewBranches)
			})
		})
	})
}

func syncLabel(b gitx.Branch) string {
	var s string
	if b.Ahead > 0 {
		s += "↑" + strconv.Itoa(b.Ahead)
	}
	if b.Behind > 0 {
		if s != "" {
			s += " "
		}
		s += "↓" + strconv.Itoa(b.Behind)
	}
	return s
}

func (a *App) toastBar(c *ui.Context, p palette) {
	t := c.Theme()
	bg, fg := t.Inverse, t.InverseText
	if a.toastIsError {
		bg, fg = t.Danger, ui.Hex("#ffffff")
	}
	ui.Row(c).FillWidth().Shrink(0).Height(30).PaddingX(10).Gap(8).Background(bg).Children(func() {
		ui.Text(c, a.toast).FontSize(11).TextColor(fg).Grow(1)
		btn := ui.ButtonBase(c).Height(20).PaddingX(6).Radius(4)
		btn.Children(func() { ui.Text(c, "Dismiss").FontSize(10).TextColor(fg) })
		btn.OnClick(func() { a.toast = ""; a.toastIsError = false })
	})
}

func (a *App) modal(c *ui.Context, p palette) {
	t := c.Theme()
	conf := a.confirm
	if conf == nil {
		a.confirmOpen = false
		return
	}
	ui.Modal(c, &a.confirmOpen, func() {
		ui.Column(c).Width(380).Gap(10).Padding(16).Background(t.Background).
			Radius(t.Radius+2).Border(1, t.Border).Children(func() {
			ui.Text(c, conf.title).Bold().FontSize(13)
			ui.Text(c, conf.detail).FontSize(11).TextColor(t.TextMuted).Wrap()
			// submit closes the dialog and runs what it asked for. A prompt
			// with an empty field does nothing; a busy app does nothing either,
			// as the write it would start is refused anyway.
			submit := func() {
				if conf.runInput == nil {
					a.confirmOpen = false
					a.confirm = nil
					conf.run()
					return
				}
				name := strings.TrimSpace(a.promptValue)
				if (name == "" && !conf.optional) || a.busy() {
					return
				}
				a.confirmOpen = false
				a.confirm = nil
				conf.runInput(name, a.promptOption)
			}
			if conf.runInput != nil {
				input := ui.TextInput(c, &a.promptValue).FillWidth().Height(28).FontSize(12).
					PaddingX(6).Font(mono()).Placeholder(conf.input)
				if a.focusPrompt {
					input.Focus()
					a.focusPrompt = false
				}
				input.OnSubmit(submit)
				if conf.option != "" {
					ui.Checkbox(c, &a.promptOption, conf.option).FontSize(11)
				}
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				cancel := ui.Button(c, "Cancel")
				cancel.OnClick(func() { a.confirmOpen = false; a.confirm = nil })
				run := ui.PrimaryButton(c, conf.label)
				if conf.runInput == nil {
					run.Background(t.Danger)
				} else {
					run.Disabled((strings.TrimSpace(a.promptValue) == "" && !conf.optional) || a.busy())
				}
				run.OnClick(submit)
			})
		})
	})
}

func (a *App) sidebarView(c *ui.Context, p palette) {
	t := c.Theme()
	ui.Column(c).FillHeight().FillWidth().Background(t.Surface).Children(func() {
		if a.status.Root == "" {
			emptyState(c, "Open a repository", "Choose a folder, or drop one on the window.",
				"Open Repository", a.openPicker)
			return
		}
		a.tabs(c, p)
		ui.Divider(c)
		switch a.view {
		case ViewHistory:
			a.historyView(c, p)
		case ViewBranches:
			a.branchView(c, p)
		case ViewStashes:
			a.stashView(c, p)
		default:
			a.changesView(c, p)
		}
	})
}

// show switches the sidebar and loads that view's data on first use.
func (a *App) show(v View) {
	// A stash and a commit share the right panel; one must not be shown as
	// the other when the tab changes.
	if (v == ViewStashes) != (a.view == ViewStashes) {
		a.resetCommitSelection()
	}
	// A focus request belongs to the tab that made it; leaving that tab drops
	// it instead of surprising the user when they come back.
	if a.view != v {
		a.focusFilter = false
	}
	a.view = v
	switch v {
	case ViewHistory:
		a.ensureCommits()
	case ViewBranches:
		a.refreshBranches()
	}
}

func (a *App) tabs(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().Shrink(0).Height(34).PaddingX(6).Gap(4).Children(func() {
		count := len(a.status.Conflicted) + len(a.status.Staged) +
			len(a.status.Unstaged) + len(a.status.Untracked)
		a.tab(c, "Changes", strconv.Itoa(count), a.view == ViewChanges, func() {
			a.show(ViewChanges)
		})
		a.tab(c, "History", "", a.view == ViewHistory, func() {
			a.show(ViewHistory)
		})
		a.tab(c, "Branches", "", a.view == ViewBranches, func() {
			a.show(ViewBranches)
		})
		stashCount := ""
		if n := len(a.stashes); n > 0 {
			stashCount = strconv.Itoa(n)
		}
		a.tab(c, "Stashes", stashCount, a.view == ViewStashes, func() {
			a.show(ViewStashes)
		})
	})
}

func (a *App) tab(c *ui.Context, label, badge string, active bool, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Height(24).PaddingX(8).Radius(6).Gap(4).Cursor(ui.CursorPointer)
	if active {
		btn.Background(t.Background)
	}
	btn.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.Text)
		if badge != "" && badge != "0" {
			ui.Text(c, badge).FontSize(10).TextColor(t.TextMuted)
		}
	})
	btn.Label(label)
	btn.OnClick(onClick)
}

// rowTint gives a selectable row its selection or hover color and reports
// whether it is hovered: row actions show while hovered or selected.
// Selection wins over hover.
func rowTint(row ui.Element, t *ui.Theme, selected bool) bool {
	hovered := row.Hovered()
	switch {
	case selected:
		row.Background(t.Background)
	case hovered:
		row.Background(t.SurfaceHover)
	}
	return hovered
}

// segmentedEnum binds a named int enum to a segmented control and reports
// whether the user picked another value.
func segmentedEnum[T ~int](c *ui.Context, value *T, labels ...string) bool {
	picked := int(*value)
	ui.Segmented(c, &picked, labels...).Height(24).FontSize(10)
	if T(picked) == *value {
		return false
	}
	*value = T(picked)
	return true
}

// filterNeedle normalizes a filter box so callers share one matching rule.
func filterNeedle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// nameMatches reports whether name passes a normalized filter; an empty
// filter keeps everything.
func nameMatches(needle, name string) bool {
	return needle == "" || strings.Contains(strings.ToLower(name), needle)
}

// pathLabel is the row tooltip for a file, showing both sides of a rename.
func pathLabel(path, orig string) string {
	if orig != "" {
		return orig + " → " + path
	}
	return path
}

// filterInput is the small search field the side panels and the popover share.
func (a *App) filterInput(c *ui.Context, value *string, placeholder string) ui.Element {
	return ui.TextInput(c, value).FillWidth().Height(26).FontSize(11).PaddingX(8).
		Placeholder(placeholder)
}

func (a *App) scrollColumn(c *ui.Context, fn func()) {
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().PaddingY(4).Children(fn)
	})
}

func emptyState(c *ui.Context, title, detail, actionLabel string, action func()) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Gap(8).Padding(20).Children(func() {
		ui.Text(c, title).Bold().FontSize(13)
		if detail != "" {
			ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted).Wrap().TextAlign(ui.Center)
		}
		if actionLabel != "" && action != nil {
			btn := ui.PrimaryButton(c, actionLabel)
			btn.OnClick(action)
		}
	})
}
