package shell

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

func (a *App) diffView(c *ui.Context, p palette) {
	t := c.Theme()
	history := a.view == ViewHistory

	var (
		path        string
		origPath    string
		res         gitx.DiffResult
		has         bool
		patch       patchFile
		loading     bool
		errText     string
		placeholder string
		editable    bool
	)

	if history {
		if a.expanded == nil {
			placeholder = "Select a commit to see its files."
		} else if a.loadingFiles {
			placeholder = "Loading files…"
		} else if a.commitFile == nil {
			placeholder = "This commit changed no files."
		}
		if a.commitFile != nil {
			path = a.commitFile.Path
			origPath = a.commitFile.OrigPath
		}
		res, has, patch = a.commitDiff, a.hasCommitDiff, a.commitPatch
		loading = a.loadingCommitDiff
		errText = a.commitErr
	} else {
		if a.selection == nil {
			placeholder = "Select a file to see its diff."
		}
		if a.selection != nil {
			path, origPath = a.selection.Path, a.selection.OrigPath
			// Editable only while no write or reload is in flight: hunk
			// indices are relative to the patch that is on screen.
			editable = !a.selection.Untracked && !a.selection.Conflicted &&
				a.selection.OrigPath == "" && !a.writing &&
				!a.loadingStatus && !a.loadingDiff
		}
		res, has, patch = a.diff, a.hasDiff, a.worktreePatch
		loading = a.loadingDiff
		errText = a.errorText
	}

	ui.Column(c).FillHeight().FillWidth().Background(t.Background).Children(func() {
		a.diffHeader(c, p, history, path, origPath, has, patch)
		ui.Divider(c)

		switch {
		case errText != "":
			emptyState(c, "Could not load the diff", errText, "Dismiss", func() {
				if history {
					a.commitErr = ""
				} else {
					a.errorText = ""
				}
			})
		case placeholder != "":
			emptyState(c, placeholder, "", "", nil)
		case loading:
			emptyState(c, "Loading…", "", "", nil)
		case !has:
			emptyState(c, "No diff to show", "Select the file again to retry.", "", nil)
		case res.TooLarge:
			emptyState(c, "File is too large to show",
				"Open it in another tool, or stage it directly from the file list.", "", nil)
		case res.Binary:
			emptyState(c, "Binary file", "Binary changes are not rendered yet.", "", nil)
		case len(patch.Hunks) == 0 || len(patch.Lines) == 0:
			detail := "The file has no line changes."
			if origPath != "" {
				detail = "The file was renamed or moved without content changes."
			}
			emptyState(c, "No textual changes", detail, "", nil)
		default:
			a.patchView(c, p, patch, editable)
		}
	})
}

func (a *App) diffHeader(c *ui.Context, p palette, history bool, path, origPath string,
	has bool, patch patchFile) {
	t := c.Theme()
	ui.Row(c).FillWidth().Shrink(0).Height(36).PaddingX(10).Gap(8).Children(func() {
		if path == "" {
			ui.Text(c, "No file selected").Font(mono()).FontSize(11).TextColor(t.TextMuted)
		} else {
			ui.Text(c, path).Font(mono()).FontSize(11).Bold()
			if origPath != "" {
				ui.Text(c, "← "+origPath).Font(mono()).FontSize(10).TextColor(t.TextMuted)
			}
		}
		// Binary files and pure renames have no hunks; "+0 −0" would read as
		// "nothing changed" next to the placeholder that explains why.
		if has && len(patch.Hunks) > 0 {
			ui.Text(c, "+"+strconv.Itoa(patch.Additions)).Font(mono()).FontSize(10).TextColor(p.addText)
			ui.Text(c, "−"+strconv.Itoa(patch.Deletions)).Font(mono()).FontSize(10).TextColor(p.delText)
		}
		ui.Spacer(c)
		a.diffBadge(c, p, history)
		if segmentedEnum(c, &a.diffLayout, "Unified", "Split") {
			// The rows pair old and new lines differently per layout.
			a.diffDirty = true
		}
	})
}

func (a *App) diffBadge(c *ui.Context, p palette, history bool) {
	t := c.Theme()
	label, fg, border := "", t.TextMuted, t.Border
	if history {
		if a.expanded != nil {
			label = a.expanded.Short
		}
	} else if a.selection != nil {
		switch {
		case a.selection.Conflicted:
			label, fg, border = "Conflict", p.delText, p.delText
		case a.selection.Staged:
			label = "Staged"
		case a.selection.Untracked:
			label = "Untracked"
		default:
			label = "Working tree"
		}
	}
	if label == "" {
		return
	}
	chip := ui.Row(c).Shrink(0).Height(18).PaddingX(5).Radius(4).Border(1, border)
	chip.Children(func() { ui.Text(c, label).FontSize(9).TextColor(fg) })
}

// patchView draws the patch as a virtual list of diff rows, in the layout the
// header selected. Each hunk header is a row with its own stage, unstage or
// discard actions.
func (a *App) patchView(c *ui.Context, p palette, patch patchFile, editable bool) {
	// The header above may have changed the layout; rebuild before drawing.
	a.syncDiffRows(patch)
	split := a.diffLayout == diffSplit
	ui.List(c, &a.diffList, len(a.diffRows), func(i int) {
		row := a.diffRows[i]
		switch row.Kind {
		case rowHunk:
			a.hunkRow(c, p, patch, row.Hunk, editable)
		case rowGap:
			a.gapRow(c, p, row.Gap)
		case rowMeta:
			a.metaRow(c, p, row.Meta)
		default:
			if split {
				a.splitRow(c, p, row)
			} else {
				a.lineRow(c, p, row.Left)
			}
		}
	}).ItemKey(func(i int) any { return i })
}

// diffGutterWidth keeps line numbers aligned in both layouts.
const diffGutterWidth float32 = 30

// diffNumber draws a right-aligned line number; 0 draws an empty gutter.
func diffNumber(c *ui.Context, num int) {
	ui.Text(c, gutter(num)).Font(mono()).FontSize(9).Width(diffGutterWidth).
		TextAlign(ui.End).TextColor(c.Theme().TextMuted)
}

// diffText draws one numbered line at the font both layouts share.
func diffText(c *ui.Context, num int, text string) {
	diffNumber(c, num)
	ui.Text(c, text).Font(mono()).FontSize(11).SingleLine()
}

// lineTint is the background a changed line gets, if any. A split row's old
// half only ever holds deletions and context, its new half additions and
// context, so the line's own kind is enough on both sides.
func lineTint(p palette, line *patchLine) (ui.Color, bool) {
	if line == nil {
		return ui.Color{}, false
	}
	switch line.Kind {
	case lineAdd:
		return p.addBg, true
	case lineDel:
		return p.delBg, true
	}
	return ui.Color{}, false
}

// diffLineBand starts one full-width row at the diff's line height.
func diffLineBand(c *ui.Context) ui.Element {
	return ui.Row(c).FillWidth().Height(18).Clip().Gap(6).PaddingX(6)
}

// lineRow draws one line of the unified layout: two gutters and the text.
func (a *App) lineRow(c *ui.Context, p palette, line *patchLine) {
	if line == nil {
		return
	}
	row := diffLineBand(c)
	if bg, ok := lineTint(p, line); ok {
		row.Background(bg)
	}
	row.Children(func() {
		diffNumber(c, line.OldNo)
		diffText(c, line.NewNo, line.Text)
	})
}

// metaRow draws a patch header line (diff/git, index, ---/+++) full width.
// Empty gutters keep it aligned with the numbered lines below it.
func (a *App) metaRow(c *ui.Context, p palette, text string) {
	diffLineBand(c).Background(p.hunkBg).Children(func() {
		diffNumber(c, 0)
		diffNumber(c, 0)
		ui.Text(c, text).Font(mono()).FontSize(11).TextColor(p.metaText).SingleLine()
	})
}

// splitRow draws one paired row: the old side, a divider, the new side.
func (a *App) splitRow(c *ui.Context, p palette, row diffRow) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(18).Clip().Children(func() {
		a.splitHalf(c, p, row.Left, true)
		ui.Box(c).Width(1).FillHeight().Background(t.Border)
		a.splitHalf(c, p, row.Right, false)
	})
}

// splitHalf draws one side of a split row. An empty half keeps the columns
// aligned where the other side has no matching line.
func (a *App) splitHalf(c *ui.Context, p palette, line *patchLine, oldSide bool) {
	half := ui.Row(c).Grow(1).FillHeight().Clip().Gap(6).PaddingX(6)
	if line == nil {
		return
	}
	if bg, ok := lineTint(p, line); ok {
		half.Background(bg)
	}
	num := line.NewNo
	if oldSide {
		num = line.OldNo
	}
	half.Children(func() { diffText(c, num, line.Text) })
}

// gapRow is the expand/collapse control for the unchanged lines git omitted
// between two hunks.
func (a *App) gapRow(c *ui.Context, p palette, g *diffGap) {
	t := c.Theme()
	noun := "lines"
	if g.Count == 1 {
		noun = "line"
	}
	var label string
	switch {
	case g.Loading:
		label = "Loading " + strconv.Itoa(g.Count) + " unchanged " + noun + "…"
	case g.Expanded:
		label = "▾ " + strconv.Itoa(g.Count) + " unchanged " + noun
	default:
		label = "▸ " + strconv.Itoa(g.Count) + " unchanged " + noun
	}
	row := ui.Row(c).FillWidth().Height(20).Center().Cursor(ui.CursorPointer).
		Background(p.hunkBg)
	row.Tooltip("Show the lines this patch omitted")
	row.OnClick(func() { a.toggleGap(g) })
	row.Children(func() {
		ui.Text(c, label).FontSize(10).TextColor(t.TextMuted)
	})
}

func (a *App) hunkRow(c *ui.Context, p palette, patch patchFile, index int, editable bool) {
	t := c.Theme()
	hunk := patch.Hunks[index]
	staged := a.selection != nil && a.selection.Staged
	ui.Row(c).FillWidth().Height(26).PaddingX(6).Gap(6).Background(p.hunkBg).Children(func() {
		ui.Text(c, "Hunk "+strconv.Itoa(hunk.Index+1)).FontSize(10).Bold().TextColor(t.TextMuted)
		ui.Spacer(c)
		if editable {
			if staged {
				smallAction(c, "− Unstage hunk", func() {
					a.applyHunk(gitx.ApplyUnstage, hunk.Index)
				})
			} else {
				smallAction(c, "+ Stage hunk", func() {
					a.applyHunk(gitx.ApplyStage, hunk.Index)
				})
				smallAction(c, "↺ Discard hunk", func() {
					name := "this hunk"
					if a.selection != nil {
						name = a.selection.Path
					}
					a.confirmAction("Discard hunk "+strconv.Itoa(hunk.Index+1)+" of "+name+"?",
						"This cannot be undone. Only this hunk is reverted.",
						"Discard", func() { a.applyHunk(gitx.ApplyDiscard, hunk.Index) })
				})
			}
		}
	})
}

func smallAction(c *ui.Context, label string, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Height(20).PaddingX(6).Radius(4).Cursor(ui.CursorPointer)
	btn.Children(func() { ui.Text(c, label).FontSize(9).TextColor(t.TextMuted) })
	btn.Label(label)
	btn.OnClick(onClick)
}

func gutter(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}
