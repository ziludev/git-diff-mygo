package shell

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// historyRow is one line of the history list: a commit, a file under an
// expanded commit, or the "load older" footer.
type historyRow struct {
	key    string
	index  int // the commit's position in App.commits
	commit *gitx.Commit
	file   *gitx.FileEntry
	more   bool
	note   string
}

func (a *App) historyView(c *ui.Context, p palette) {
	t := c.Theme()
	if a.status.Root != "" {
		a.ensureCommits()
	}
	switch {
	case a.historyErr != "":
		emptyState(c, "Could not load history", a.historyErr, "", nil)
		return
	case !a.commitsLoaded || (a.loadingCommits && len(a.commits) == 0):
		emptyState(c, "Loading…", "", "", nil)
		return
	case len(a.commits) == 0:
		emptyState(c, "No commits yet", "Commits on this branch will show up here.", "", nil)
		return
	}

	rows := a.historyRows()
	indent := float32(a.graphLanes)*graphLaneWidth + 8
	ui.List(c, &a.historyList, len(rows), func(i int) {
		r := rows[i]
		switch {
		case r.more:
			ui.Row(c).FillWidth().Height(36).Center().Children(func() {
				btn := ui.Button(c, "Load older commits").Height(24).FontSize(11).
					Disabled(a.loadingCommits)
				btn.OnClick(func() { a.loadCommits(true) })
			})
		case r.note != "":
			ui.Row(c).FillWidth().Height(24).PaddingStart(indent).Center().Children(func() {
				ui.Text(c, r.note).FontSize(10).TextColor(t.TextMuted)
			})
		case r.file != nil:
			a.commitFileRow(c, p, *r.file, indent)
		case r.commit != nil:
			a.commitRow(c, p, *r.commit, r, indent)
		}
	}).ItemKey(func(i int) any {
		if i < len(rows) {
			return rows[i].key
		}
		return i
	})
}

// historyRows flattens commits, the files of the expanded commit and the
// footer into the rows a virtual list builds.
func (a *App) historyRows() []historyRow {
	rows := make([]historyRow, 0, len(a.commits)+len(a.commitFiles)+1)
	for i := range a.commits {
		commit := &a.commits[i]
		rows = append(rows, historyRow{key: "c:" + commit.OID, index: i, commit: commit})
		if a.expanded == nil || a.expanded.OID != commit.OID {
			continue
		}
		if a.loadingFiles {
			rows = append(rows, historyRow{key: "f:loading", note: "Loading files…"})
			continue
		}
		if len(a.commitFiles) == 0 {
			rows = append(rows, historyRow{key: "f:none", note: "No files changed."})
			continue
		}
		for i := range a.commitFiles {
			file := &a.commitFiles[i]
			rows = append(rows, historyRow{
				key:  "f:" + commit.OID + ":" + file.Path,
				file: file,
			})
		}
	}
	if a.hasMoreCommits {
		rows = append(rows, historyRow{key: "more", more: true})
	}
	return rows
}

// commitRow draws the lane graph and one commit. The graph is a plain
// box with a painter: lines and a round node, as the web view drew it.
func (a *App) commitRow(c *ui.Context, p palette, commit gitx.Commit, r historyRow, indent float32) {
	t := c.Theme()
	expanded := a.expanded != nil && a.expanded.OID == commit.OID
	row := ui.Row(c).FillWidth().Height(graphRowHeight).Gap(2).PaddingEnd(8).Clip().
		Cursor(ui.CursorPointer)
	if expanded {
		row.Background(t.Background)
	}
	row.Tooltip(commit.Subject)
	row.OnClick(func() { a.selectCommit(commit) })
	row.Children(func() {
		var g graphRow
		if r.index >= 0 && r.index < len(a.graph) {
			g = a.graph[r.index]
		}
		a.drawGraph(c, a.graphLanes, g)
		ui.Column(c).Grow(1).Gap(2).ClipX().Children(func() {
			ui.Row(c).FillWidth().Gap(4).Children(func() {
				chevron := "▸"
				if expanded {
					chevron = "▾"
				}
				ui.Text(c, chevron).FontSize(9).TextColor(t.TextMuted)
				subject := commit.Subject
				if subject == "" {
					subject = "(no message)"
				}
				ui.Text(c, subject).FontSize(12).Grow(1).SingleLine()
				a.commitRefs(c, p, commit.Refs)
			})
			ui.Row(c).FillWidth().Gap(8).PaddingStart(12).Children(func() {
				ui.Text(c, commit.Short).Font(mono()).FontSize(10).TextColor(t.TextMuted).SingleLine()
				ui.Text(c, commit.Author).FontSize(10).TextColor(t.TextMuted).Grow(1).SingleLine()
				ui.Text(c, relativeTime(commit.Time)).FontSize(10).TextColor(t.TextMuted).SingleLine()
			})
		})
	})
}

// commitRefs shows at most two branch or tag chips, then a count.
func (a *App) commitRefs(c *ui.Context, p palette, refs []gitx.Ref) {
	t := c.Theme()
	if len(refs) == 0 {
		return
	}
	shown := refs
	hidden := 0
	if len(shown) > 2 {
		hidden = len(shown) - 2
		shown = shown[:2]
	}
	ui.Row(c).Shrink(0).Gap(3).Children(func() {
		for _, ref := range shown {
			fg := t.TextMuted
			border := t.Border
			if ref.Kind == "tag" {
				fg, border = p.warnText, p.warnText
			}
			chip := ui.Row(c).Shrink(0).Height(15).PaddingX(3).Radius(3).
				Border(1, border).ClipX()
			chip.Children(func() {
				ui.Text(c, ref.Name).FontSize(9).TextColor(fg).SingleLine()
			})
		}
		if hidden > 0 {
			ui.Text(c, "+"+strconv.Itoa(hidden)).FontSize(9).TextColor(t.TextMuted)
		}
	})
}

func (a *App) commitFileRow(c *ui.Context, p palette, file gitx.FileEntry, indent float32) {
	t := c.Theme()
	selected := a.commitFile != nil && a.commitFile.Path == file.Path &&
		a.commitFile.OrigPath == file.OrigPath
	row := ui.Row(c).FillWidth().Height(24).PaddingStart(indent).PaddingEnd(8).Gap(6).Clip().
		Cursor(ui.CursorPointer)
	if selected {
		row.Background(t.Background)
	}
	tip := pathLabel(file.Path, file.OrigPath)
	row.Tooltip(tip)
	row.OnClick(func() { a.selectCommitFile(file) })
	row.Children(func() {
		ui.Text(c, file.Status).Font(mono()).FontSize(10).Bold().
			Width(12).TextAlign(ui.Center).TextColor(statusColor(p, file.Status))
		ui.Text(c, file.Path).Font(mono()).FontSize(11).Grow(1).SingleLine()
		if file.Binary {
			ui.Text(c, "bin").FontSize(9).TextColor(t.TextMuted)
		} else {
			ui.Text(c, "+"+strconv.Itoa(file.Additions)).Font(mono()).FontSize(9).TextColor(p.addText)
			ui.Text(c, "−"+strconv.Itoa(file.Deletions)).Font(mono()).FontSize(9).TextColor(p.delText)
		}
	})
}

func (a *App) drawGraph(c *ui.Context, lanes int, g graphRow) {
	width := float32(lanes)*graphLaneWidth + 14
	ui.Box(c).Width(width).Height(graphRowHeight).Shrink(0).
		Draw(func(pt *ui.Painter, r ui.Rect) {
			laneX := func(lane int) float32 {
				return r.X + 10 + float32(lane)*graphLaneWidth
			}
			top, mid, bottom := r.Y, r.Y+r.H/2, r.Y+r.H
			for _, seg := range g.Segments {
				color := graphColors[seg.Color%len(graphColors)]
				pt.Line(laneX(seg.Lane), top+seg.From*r.H, laneX(seg.Lane),
					top+seg.To*r.H, 1.5, color)
			}
			for _, edge := range g.Edges {
				color := graphColors[edge.Color%len(graphColors)]
				if edge.To == edge.From {
					pt.Line(laneX(edge.From), mid, laneX(edge.To), bottom, 1.5, color)
				} else {
					pt.Line(laneX(edge.From), mid, laneX(edge.To), mid+r.H*0.3, 1.5, color)
					pt.Line(laneX(edge.To), mid+r.H*0.3, laneX(edge.To), bottom, 1.5, color)
				}
			}
			if lanes > g.Lane {
				color := graphColors[g.Color%len(graphColors)]
				x := laneX(g.Lane)
				pt.Fill(ui.Rect{X: x - 4, Y: mid - 4, W: 8, H: 8}, color, 4)
			}
		})
}
