package shell

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

// The commit row's columns, left to right: avatar, author, time, lane graph,
// expander, message and ref chips. The fixed widths keep the graph and the
// messages at one x down the list, as IDEA's log does.
const (
	// historyAvatarSize is the diameter of the author's avatar disc.
	historyAvatarSize float32 = 20
	// historyAuthorWidth and historyTimeWidth are the author and time columns.
	historyAuthorWidth float32 = 84
	historyTimeWidth   float32 = 58
	// historyChevronWidth fixes the expand marker so the message column has
	// the same left edge on every row.
	historyChevronWidth float32 = 8
	// historyRowGap is the space between two columns.
	historyRowGap float32 = 6
	// historyRowPad is the space the row leaves on each side.
	historyRowPad float32 = 6
)

// avatarColors are the disc colors an author's initial sits on. They are dark
// enough for white text in both themes.
var avatarColors = []ui.Color{
	ui.Hex("#4f8cff"),
	ui.Hex("#8b5cf6"),
	ui.Hex("#0ea5e9"),
	ui.Hex("#0d9488"),
	ui.Hex("#d97706"),
	ui.Hex("#e11d48"),
	ui.Hex("#db2777"),
	ui.Hex("#64748b"),
}

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
	// A commit's files and notes line up with its message column, so they read
	// as its children.
	indent := a.historyMessageX()
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
			a.commitRow(c, p, *r.commit, r)
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

// commitRow draws one commit as a single line of columns: the author's avatar,
// the author, the commit time, the lane graph, the expander, the message and
// the ref chips on the right. The graph is a plain box with a painter: lines
// and a round node, as the web view drew it.
func (a *App) commitRow(c *ui.Context, p palette, commit gitx.Commit, r historyRow) {
	t := c.Theme()
	expanded := a.expanded != nil && a.expanded.OID == commit.OID
	row := ui.Row(c).FillWidth().Height(graphRowHeight).PaddingX(historyRowPad).
		Gap(historyRowGap).Clip().Cursor(ui.CursorPointer)
	bg := t.Surface
	if expanded {
		bg = t.Background
		row.Background(bg)
	}
	subject := commit.Subject
	if subject == "" {
		subject = "(no message)"
	}
	// The row is one line, so the hash and the full time live in tooltips.
	row.Tooltip(commit.Short + " " + subject)
	row.OnClick(func() { a.selectCommit(commit) })
	row.Children(func() {
		a.commitAvatar(c, commit.Author)
		author := commit.Author
		if author == "" {
			author = "unknown"
		}
		ui.Text(c, author).FontSize(10).TextColor(t.TextMuted).
			Width(historyAuthorWidth).SingleLine().Tooltip(commit.Author)
		ui.Text(c, relativeTime(commit.Time)).FontSize(10).TextColor(t.TextMuted).
			Width(historyTimeWidth).SingleLine().
			Tooltip(time.Unix(commit.Time, 0).Format("2006-01-02 15:04"))
		var g graphRow
		if r.index >= 0 && r.index < len(a.graph) {
			g = a.graph[r.index]
		}
		a.drawGraph(c, a.graphLanes, g, bg)
		chevron := "▸"
		if expanded {
			chevron = "▾"
		}
		ui.Text(c, chevron).FontSize(9).TextColor(t.TextMuted).Width(historyChevronWidth)
		ui.Text(c, subject).FontSize(12).Grow(1).SingleLine()
		a.commitRefs(c, p, commit.Refs)
	})
}

// historyMessageX is where a commit's message starts: past the row padding,
// the avatar, the author and time columns, the lane graph and the expander,
// with one gap between each of those six parts. File rows use it as their
// indent so they sit under their commit's message.
func (a *App) historyMessageX() float32 {
	graph := float32(a.graphLanes)*graphLaneWidth + graphLaneWidth
	return historyRowPad + historyAvatarSize + historyAuthorWidth + historyTimeWidth +
		historyChevronWidth + graph + 5*historyRowGap
}

// commitAvatar draws the author's initials on a colored disc. The color comes
// from the name, so one person keeps one color down the list and across
// sessions. It is decoration, not identity: no image is fetched and the name
// is never sent anywhere.
func (a *App) commitAvatar(c *ui.Context, author string) {
	initial := "?"
	if name := strings.TrimSpace(author); name != "" {
		initial = string(unicode.ToUpper([]rune(name)[0]))
	}
	disc := ui.Box(c).Width(historyAvatarSize).Height(historyAvatarSize).
		Radius(historyAvatarSize / 2).Background(avatarColor(author)).Shrink(0).Center()
	disc.Children(func() {
		ui.Text(c, initial).FontSize(9).Bold().TextColor(ui.Hex("#ffffff"))
	})
}

// avatarColor picks an author's disc color from a hash of the name.
func avatarColor(author string) ui.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(author)))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
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

// drawGraph paints one row's lanes. Segments and elbows meet at the row
// boundary, so a lane stays one continuous line down the list; an elbow leaves
// its commit's lane vertically and arrives vertically in the parent's lane,
// like IDEA's log, rather than cutting across the row at an angle. bg is the
// row background under the graph, used to punch a halo around the node so
// crossing lanes do not run through it.
func (a *App) drawGraph(c *ui.Context, lanes int, g graphRow, bg ui.Color) {
	width := float32(lanes)*graphLaneWidth + graphLaneWidth
	ui.Box(c).Width(width).Height(graphRowHeight).Shrink(0).
		Draw(func(pt *ui.Painter, r ui.Rect) {
			laneX := func(lane int) float32 {
				return r.X + graphLaneWidth/2 + float32(lane)*graphLaneWidth
			}
			top, mid, bottom := r.Y, r.Y+r.H/2, r.Y+r.H
			for _, seg := range g.Segments {
				color := graphColors[seg.Color%len(graphColors)]
				// Half a pixel past each end hides the seam between two rows
				// that antialias their meeting edges separately.
				pt.Line(laneX(seg.Lane), top+seg.From*r.H-0.5, laneX(seg.Lane),
					top+seg.To*r.H+0.5, 2, color)
			}
			for _, edge := range g.Edges {
				color := graphColors[edge.Color%len(graphColors)]
				if edge.From == edge.To {
					pt.Line(laneX(edge.From), mid, laneX(edge.To), bottom, 2, color)
					continue
				}
				drop := bottom - mid
				path := (&ui.Path{}).MoveTo(laneX(edge.From), mid).
					CubeTo(laneX(edge.From), mid+drop*0.55,
						laneX(edge.To), bottom-drop*0.55,
						laneX(edge.To), bottom)
				pt.StrokePath(path, 2, color)
			}
			if lanes > g.Lane {
				color := graphColors[g.Color%len(graphColors)]
				x := laneX(g.Lane)
				pt.Fill(ui.Rect{X: x - 5, Y: mid - 5, W: 10, H: 10}, bg, 5)
				pt.Fill(ui.Rect{X: x - 4, Y: mid - 4, W: 8, H: 8}, color, 4)
			}
		})
}
