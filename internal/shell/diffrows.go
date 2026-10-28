package shell

import (
	"context"
	"fmt"

	"twill/internal/gitx"
)

// maxGapLines bounds the rows one expansion may add. The cap applies to the
// gap, not the file: a long file with a small gap still expands.
const maxGapLines = 20000

// diffLayout is how the diff pane draws a patch: one column with both line
// numbers, or the old and new sides side by side.
type diffLayout int

const (
	diffUnified diffLayout = iota
	diffSplit
)

// diffRowKind is what one row of the diff list draws.
type diffRowKind int

const (
	rowLines diffRowKind = iota // a line (or a paired old/new line)
	rowMeta                     // a patch header line, full width
	rowHunk                     // an @@ header with its actions, full width
	rowGap                      // hidden unchanged lines, full width
)

// diffSourceID names the patch the diff pane shows, so a tab switch or another
// file is told apart from a rebuild of the same patch. It is comparable, so the
// view compares it without building a string on every frame.
type diffSourceID struct {
	history bool
	oid     string
	path    string
	rev     string
}

// diffGap is one unchanged region a patch omits. Both sides hold the same
// text, so one side's numbers are enough to read it; OldStart and NewStart
// only number the expanded rows.
type diffGap struct {
	Hunk     int // the hunk this gap precedes, and its identity in the map
	OldStart int // first hidden old line; the first new line is NewStart
	NewStart int
	Count    int
	// Expanded is the user's choice; Loading is an in-flight read; Lines is
	// the loaded text, Count entries once Expanded is true.
	Expanded bool
	Loading  bool
	Lines    []string
}

// diffRow is one row of the diff list. Line rows carry Left and Right: unified
// uses Left alone, split uses both, and either may be nil when that side has no
// line in the pair.
type diffRow struct {
	Kind  diffRowKind
	Meta  string
	Hunk  int
	Gap   *diffGap
	Left  *patchLine
	Right *patchLine
}

// gapRead caches the last file text a gap read returned. Gaps of one file
// share it, so expanding a second gap does not read and split the file again.
type gapRead struct {
	req   gitx.FileTextRequest
	lines []string
}

// gapsFor finds the unchanged regions between a patch's hunks. Git emits only
// -U3 context, so these are exactly the lines a reader cannot see. The region
// after the last hunk is not included: the patch does not say how long the
// file is.
func gapsFor(p patchFile) []*diffGap {
	if len(p.Hunks) == 0 {
		return nil
	}
	var gaps []*diffGap
	// prevEnd is the first old line after the previous hunk; the file starts
	// at line 1.
	prevEnd := 1
	for i, h := range p.Hunks {
		count := h.OldStart - prevEnd
		if h.OldCount == 0 {
			// A zero-length range starts at the line the insertion follows,
			// so that line is hidden too. "-0,0" (a new file) still yields
			// nothing: 0-1+1 is 0.
			count++
		}
		if count > 0 {
			// Map the gap's first old line onto the new side with the offset
			// the hunk header states. A pure insertion covers no old lines:
			// its line before the inserted block is old OldStart, which maps
			// to NewStart-1, not to NewStart.
			delta := h.NewStart - h.OldStart
			if h.OldCount == 0 {
				delta = h.NewStart - 1 - h.OldStart
			}
			if newStart := prevEnd + delta; newStart >= 1 {
				gaps = append(gaps, &diffGap{
					Hunk:     i,
					OldStart: prevEnd,
					NewStart: newStart,
					Count:    count,
				})
			}
		}
		// An insertion covers no old lines: git's "-5,0" means "after line
		// 5", so the next region starts at line 6.
		if h.OldCount == 0 {
			prevEnd = h.OldStart + 1
		} else {
			prevEnd = h.OldStart + h.OldCount
		}
	}
	return gaps
}

// buildDiffRows turns a patch into the rows the diff list draws, inserting a
// gap row before every hunk whose unchanged lines the patch omits. gaps is the
// per-patch map of gap state (expanded, loaded lines): a gap already in it is
// reused so expanding survives a rebuild for a layout change.
func buildDiffRows(p patchFile, layout diffLayout, gaps map[int]*diffGap) []diffRow {
	hidden := gapsFor(p)
	rows := make([]diffRow, 0, len(p.Lines)+len(hidden)+len(p.Hunks))
	gapIdx := 0
	for i := 0; i < len(p.Lines); i++ {
		line := p.Lines[i]
		switch line.Kind {
		case lineMeta:
			rows = append(rows, diffRow{Kind: rowMeta, Meta: line.Text})
		case lineHunk:
			for gapIdx < len(hidden) && hidden[gapIdx].Hunk == line.Hunk {
				g := hidden[gapIdx]
				if known, ok := gaps[g.Hunk]; ok {
					g = known
				} else {
					gaps[g.Hunk] = g
				}
				rows = append(rows, diffRow{Kind: rowGap, Gap: g})
				if g.Expanded && len(g.Lines) == g.Count {
					for k := 0; k < g.Count; k++ {
						// Expanded rows are ordinary context lines with no hunk
						// of their own, so no hunk action attaches to them.
						pl := patchLine{
							Kind:  lineContext,
							Text:  " " + g.Lines[k],
							OldNo: g.OldStart + k,
							NewNo: g.NewStart + k,
							Hunk:  -1,
						}
						rows = append(rows, diffRow{Kind: rowLines, Left: &pl, Right: &pl})
					}
				}
				gapIdx++
			}
			rows = append(rows, diffRow{Kind: rowHunk, Hunk: line.Hunk, Meta: line.Text})
		default:
			if layout == diffUnified {
				rows = append(rows, diffRow{Kind: rowLines, Left: &p.Lines[i]})
				continue
			}
			if line.Kind == lineContext {
				rows = append(rows, diffRow{Kind: rowLines, Left: &p.Lines[i], Right: &p.Lines[i]})
				continue
			}
			// A change block: git prints every deletion of the block before
			// its additions, so pair them by position and leave the longer
			// side's extra rows alone.
			start := i
			for i < len(p.Lines) && p.Lines[i].Kind == lineDel {
				i++
			}
			delEnd := i
			for i < len(p.Lines) && p.Lines[i].Kind == lineAdd {
				i++
			}
			dels, adds := p.Lines[start:delEnd], p.Lines[delEnd:i]
			for k := 0; k < len(dels) || k < len(adds); k++ {
				row := diffRow{Kind: rowLines}
				if k < len(dels) {
					row.Left = &dels[k]
				}
				if k < len(adds) {
					row.Right = &adds[k]
				}
				rows = append(rows, row)
			}
			// i now points at the block's end or at a non-change line; step
			// back so the outer loop consumes that line.
			i--
		}
	}
	return rows
}

// diffSource names the patch the pane shows: its identity, the new side a gap
// read needs, and whether anything is selected. One place maps the worktree
// and history selections onto patches, so the header, the rows and the gap
// reads cannot disagree.
func (a *App) diffSource() (id diffSourceID, req gitx.FileTextRequest, ok bool) {
	if a.view == ViewHistory {
		if a.selectedCommit == nil || a.commitFile == nil {
			return diffSourceID{}, gitx.FileTextRequest{}, false
		}
		req = gitx.FileTextRequest{Path: a.commitFile.Path, Rev: a.selectedCommit.OID}
		return diffSourceID{history: true, oid: a.selectedCommit.OID, path: req.Path}, req, true
	}
	if a.selection == nil {
		return diffSourceID{}, gitx.FileTextRequest{}, false
	}
	req = gitx.FileTextRequest{Path: a.selection.Path}
	if a.selection.Staged {
		req.Rev = "index"
	}
	return diffSourceID{path: req.Path, rev: req.Rev}, req, true
}

// syncDiffRows rebuilds the diff rows from the patch the pane shows when the
// source or the layout changed. A different source also drops the gap state:
// its keys are hunk indexes, which mean nothing for another patch.
func (a *App) syncDiffRows(p patchFile) {
	id, _, _ := a.diffSource()
	if id != a.diffKey {
		a.diffKey = id
		a.resetDiffGaps()
	}
	if !a.diffDirty {
		return
	}
	a.diffDirty = false
	if a.diffGaps == nil {
		a.diffGaps = map[int]*diffGap{}
	}
	a.diffRows = buildDiffRows(p, a.diffLayout, a.diffGaps)
}

// resetDiffGaps forgets the expansions and the cached text of a patch that is
// being replaced. Reads still in flight are dropped by the identity check in
// toggleGap: they hold a gap object the new map does not contain.
func (a *App) resetDiffGaps() {
	a.diffGaps = nil
	a.gapText = gapRead{}
	a.diffDirty = true
}

// toggleGap expands or collapses one unchanged region. The first expansion
// reads the new side of the diff; the text is cached for the file's other
// gaps.
func (a *App) toggleGap(g *diffGap) {
	if g.Loading {
		return
	}
	if g.Expanded || len(g.Lines) == g.Count {
		// Collapsing keeps the text, so re-expanding is instant.
		g.Expanded = !g.Expanded
		a.diffDirty = true
		return
	}
	if g.Count > maxGapLines {
		a.showError(fmt.Errorf("%d unchanged lines is more than this view expands", g.Count))
		return
	}
	_, req, ok := a.diffSource()
	if !ok {
		return
	}
	if a.gapText.req == req && a.gapText.lines != nil {
		if a.attachGapLines(g, req, a.gapText.lines) {
			a.diffDirty = true
		}
		return
	}
	g.Loading = true
	a.diffDirty = true
	go func() {
		lines, err := a.svc.FileLines(context.Background(), req)
		a.apply(func() {
			// A replaced patch builds a new gap map; this gap object is no
			// longer in it, so its read is stale.
			if a.diffGaps[g.Hunk] != g {
				return
			}
			g.Loading = false
			a.diffDirty = true
			if err != nil {
				a.showError(err)
				return
			}
			a.gapText = gapRead{req: req, lines: lines}
			a.attachGapLines(g, req, lines)
		})
	}()
}

// attachGapLines slices the lines a gap hides out of the file text. It refuses
// a file that changed since the patch was loaded.
func (a *App) attachGapLines(g *diffGap, req gitx.FileTextRequest, lines []string) bool {
	end := g.NewStart - 1 + g.Count
	if g.NewStart < 1 || end > len(lines) {
		// The text no longer maps onto the patch; drop it so a retry after
		// the refresh reads the file again.
		a.gapText = gapRead{}
		a.showError(fmt.Errorf("%s changed on disk; the diff will refresh", req.Path))
		return false
	}
	g.Lines = lines[g.NewStart-1 : end]
	g.Expanded = true
	return true
}
