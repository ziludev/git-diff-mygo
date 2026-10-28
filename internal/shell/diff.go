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

// patchView draws the unified patch as a virtual list. Each hunk header is a
// row with its own stage, unstage or discard actions.
func (a *App) patchView(c *ui.Context, p palette, patch patchFile, editable bool) {
	t := c.Theme()
	ui.List(c, &a.diffList, len(patch.Lines), func(i int) {
		line := patch.Lines[i]
		if line.Kind == lineHunk {
			a.hunkRow(c, p, patch, line, editable)
			return
		}
		row := ui.Row(c).FillWidth().Height(18).Clip().Gap(6).PaddingX(6)
		switch line.Kind {
		case lineAdd:
			row.Background(p.addBg)
		case lineDel:
			row.Background(p.delBg)
		case lineMeta:
			row.Background(p.hunkBg)
		}
		row.Children(func() {
			numColor := t.TextMuted
			ui.Text(c, gutter(line.OldNo)).Font(mono()).FontSize(9).Width(30).
				TextAlign(ui.End).TextColor(numColor)
			ui.Text(c, gutter(line.NewNo)).Font(mono()).FontSize(9).Width(30).
				TextAlign(ui.End).TextColor(numColor)
			fg := t.Text
			if line.Kind == lineMeta {
				fg = p.metaText
			}
			ui.Text(c, line.Text).Font(mono()).FontSize(11).TextColor(fg).SingleLine()
		})
	}).ItemKey(func(i int) any { return i })
}

func (a *App) hunkRow(c *ui.Context, p palette, patch patchFile, line patchLine, editable bool) {
	t := c.Theme()
	hunk := patch.Hunks[line.Hunk]
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
