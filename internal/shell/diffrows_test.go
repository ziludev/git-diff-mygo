package shell

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

const twoHunkPatch = "diff --git a/f b/f\n" +
	"index 111..222 100644\n" +
	"--- a/f\n" +
	"+++ b/f\n" +
	"@@ -5,3 +5,3 @@\n" +
	" ctx\n" +
	"-old\n" +
	"+new\n" +
	" ctx\n" +
	"@@ -20,3 +20,3 @@\n" +
	" ctx\n" +
	"-a\n" +
	"+b\n" +
	" ctx\n"

func TestGapsForFindsOmittedLines(t *testing.T) {
	p := parsePatch(twoHunkPatch)
	if len(p.Hunks) != 2 {
		t.Fatalf("hunks = %+v", p.Hunks)
	}
	if p.Hunks[0].OldCount != 3 {
		t.Fatalf("hunk 0 counts = %+v", p.Hunks[0])
	}
	gaps := gapsFor(p)
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v", gaps)
	}
	if g := gaps[0]; g.Hunk != 0 || g.OldStart != 1 || g.NewStart != 1 || g.Count != 4 {
		t.Fatalf("leading gap = %+v", g)
	}
	if g := gaps[1]; g.Hunk != 1 || g.OldStart != 8 || g.NewStart != 8 || g.Count != 12 {
		t.Fatalf("middle gap = %+v", g)
	}
}

func TestGapsForPureInsertion(t *testing.T) {
	p := parsePatch("--- a/f\n+++ b/f\n" +
		"@@ -5,0 +6,2 @@\n" +
		"+one\n" +
		"+two\n" +
		"@@ -10,3 +12,3 @@\n" +
		" ctx\n" +
		"-x\n" +
		"+y\n" +
		" ctx\n")
	gaps := gapsFor(p)
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v", gaps)
	}
	// "-5,0" is an insertion after old line 5, so lines 1..5 are hidden and
	// map one to one.
	if g := gaps[0]; g.OldStart != 1 || g.NewStart != 1 || g.Count != 5 {
		t.Fatalf("leading gap = %+v", g)
	}
	// Lines 6..9 stay unchanged; the insertion happens after line 5, so the
	// gap starts on old line 6 and new line 8.
	if g := gaps[1]; g.OldStart != 6 || g.NewStart != 8 || g.Count != 4 {
		t.Fatalf("middle gap = %+v", g)
	}
}

func TestBuildDiffRowsUnified(t *testing.T) {
	p := parsePatch(twoHunkPatch)
	gaps := map[int]*diffGap{}
	rows := buildDiffRows(p, diffUnified, gaps)
	if len(rows) != 16 {
		t.Fatalf("rows = %d: %+v", len(rows), rows)
	}
	if rows[4].Kind != rowGap || rows[4].Gap.Count != 4 {
		t.Fatalf("row 4 = %+v", rows[4])
	}
	if rows[5].Kind != rowHunk || rows[5].Hunk != 0 {
		t.Fatalf("row 5 = %+v", rows[5])
	}
	if rows[10].Kind != rowGap || rows[10].Gap.Count != 12 {
		t.Fatalf("row 10 = %+v", rows[10])
	}
	// Unified rows carry one line and no pair.
	if rows[7].Left == nil || rows[7].Right != nil || rows[7].Left.Kind != lineDel {
		t.Fatalf("row 7 = %+v", rows[7])
	}
}

func TestBuildDiffRowsSplitPairsChanges(t *testing.T) {
	p := parsePatch(twoHunkPatch)
	rows := buildDiffRows(p, diffSplit, map[int]*diffGap{})
	// Four meta, gap, hunk, context, the del/add pair, context, gap, hunk,
	// context, pair, context.
	if len(rows) != 14 {
		t.Fatalf("rows = %d: %+v", len(rows), rows)
	}
	pair := rows[7]
	if pair.Left == nil || pair.Right == nil ||
		pair.Left.Kind != lineDel || pair.Right.Kind != lineAdd {
		t.Fatalf("pair = %+v", pair)
	}
	ctx := rows[6]
	if ctx.Left == nil || ctx.Right == nil || ctx.Left.Kind != lineContext {
		t.Fatalf("context = %+v", ctx)
	}
	if ctx.Left.OldNo != 5 || ctx.Right.NewNo != 5 {
		t.Fatalf("context numbers = %d/%d", ctx.Left.OldNo, ctx.Right.NewNo)
	}
}

func TestBuildDiffRowsUnpairedAdditions(t *testing.T) {
	p := parsePatch("--- a/f\n+++ b/f\n@@ -1,1 +1,3 @@\n ctx\n+one\n+two\n")
	rows := buildDiffRows(p, diffSplit, map[int]*diffGap{})
	last := rows[len(rows)-1]
	if last.Left != nil || last.Right == nil || last.Right.Kind != lineAdd {
		t.Fatalf("unpaired addition = %+v", last)
	}
}

// diffTestApp returns a shell showing one file's patch, with no service: the
// diff view must render from state alone.
func diffTestApp(layout diffLayout) *App {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.selection = &gitx.Change{Path: "notes.txt"}
	app.diff = gitx.DiffResult{Path: "notes.txt", Patch: twoHunkPatch}
	app.worktreePatch = parsePatch(twoHunkPatch)
	app.hasDiff = true
	app.view = ViewChanges
	app.diffLayout = layout
	return app
}

func TestDiffViewUnifiedShowsGaps(t *testing.T) {
	app := diffTestApp(diffUnified)
	tt := ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("▸ 12 unchanged lines") {
		t.Fatalf("texts = %q", tt.Texts())
	}
}

func TestDiffViewSplitShowsBothSides(t *testing.T) {
	app := diffTestApp(diffSplit)
	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"▸ 12 unchanged lines", "old", "new"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestDiffSourcePicksTheNewSide(t *testing.T) {
	app := New(&service.RepoService{}, nil)

	// A commit's new side is the commit itself.
	app.view = ViewHistory
	app.expanded = &gitx.Commit{OID: "abc123"}
	app.commitFile = &gitx.FileEntry{Path: "notes.txt"}
	id, req, ok := app.diffSource()
	if !ok || !id.history || id.oid != "abc123" || id.path != "notes.txt" ||
		req.Path != "notes.txt" || req.Rev != "abc123" {
		t.Fatalf("history source = %+v, %+v, %v", id, req, ok)
	}

	// An unstaged change reads the worktree; a staged one reads the index.
	app.view = ViewChanges
	app.selection = &gitx.Change{Path: "a.go"}
	if id, req, ok := app.diffSource(); !ok || id.history || id.path != "a.go" ||
		id.rev != "" || req.Rev != "" {
		t.Fatalf("worktree source = %+v, %+v, %v", id, req, ok)
	}
	app.selection = &gitx.Change{Path: "a.go", Staged: true}
	if id, req, ok := app.diffSource(); !ok || id.rev != "index" || req.Rev != "index" {
		t.Fatalf("index source = %+v, %+v, %v", id, req, ok)
	}

	// Nothing selected means no source to read.
	app.selection = nil
	if _, _, ok := app.diffSource(); ok {
		t.Fatal("empty selection reported a source")
	}
}

func TestBuildDiffRowsReusesExpandedGap(t *testing.T) {
	p := parsePatch(twoHunkPatch)
	gaps := map[int]*diffGap{}
	buildDiffRows(p, diffUnified, gaps)
	g := gaps[0]
	g.Lines = []string{"one", "two", "three", "four"}
	g.Expanded = true

	rows := buildDiffRows(p, diffUnified, gaps)
	if gaps[0] != g {
		t.Fatal("rebuild replaced the gap state")
	}
	// The header stays, followed by the four expanded context rows.
	if rows[4].Kind != rowGap || !rows[4].Gap.Expanded {
		t.Fatalf("gap row = %+v", rows[4])
	}
	for k := 0; k < 4; k++ {
		row := rows[5+k]
		if row.Kind != rowLines || row.Left == nil || row.Left.Kind != lineContext {
			t.Fatalf("expanded row %d = %+v", k, row)
		}
		if row.Left.OldNo != 1+k || row.Left.NewNo != 1+k {
			t.Fatalf("expanded row %d numbers = %d/%d", k, row.Left.OldNo, row.Left.NewNo)
		}
		if !strings.HasPrefix(row.Left.Text, " ") {
			t.Fatalf("expanded row %d text = %q", k, row.Left.Text)
		}
	}
}
