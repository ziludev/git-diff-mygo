package shell

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

func TestEmptyView(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	tt := ui.NewTester(app.View, 900, 640)
	if !tt.HasText("Twill") {
		t.Fatalf("toolbar texts = %q", tt.Texts())
	}
	if !tt.HasText("Open a repository") {
		t.Fatalf("empty state texts = %q", tt.Texts())
	}
}

// TestChangesUntrackedStartsClosed checks the untracked section's default:
// the section is folded, and its header opens it.
func TestChangesUntrackedStartsClosed(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Untracked = []gitx.Change{{Path: "new.go", X: "?", Y: "?", Untracked: true}}
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1100, 720)
	if tt.HasText("new.go") {
		t.Fatalf("untracked is open by default: %q", tt.Texts())
	}
	if err := tt.Click("UNTRACKED"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("new.go") {
		t.Fatalf("the untracked header did not open: %q", tt.Texts())
	}
}

// TestChangesTreeGroupsByDirectory checks the tree: folders hold the files
// under their path, and a folder row folds them away and back.
func TestChangesTreeGroupsByDirectory(t *testing.T) {
	app := changesTestApp(
		gitx.Change{Path: "README.md", X: ".", Y: "M"},
		gitx.Change{Path: "src/app.go", X: ".", Y: "M"},
		gitx.Change{Path: "src/util/x.go", X: ".", Y: "M"},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"README.md", "src", "util", "app.go", "x.go"} {
		if !tt.HasText(want) {
			t.Fatalf("tree misses %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("app.go") || tt.HasText("x.go") {
		t.Fatalf("closing src left its files: %q", tt.Texts())
	}
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("app.go") || !tt.HasText("x.go") {
		t.Fatalf("opening src hid its files: %q", tt.Texts())
	}
}

// TestChangesFlatShowsFullPaths checks the more menu's flat list: no folders,
// one row per file with its path.
func TestChangesFlatShowsFullPaths(t *testing.T) {
	app := changesTestApp(gitx.Change{Path: "src/app.go", X: ".", Y: "M"})
	app.changesTree = false
	tt := ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("src/app.go") {
		t.Fatalf("flat list misses the path: %q", tt.Texts())
	}
}

// TestHistoryFilterNarrowsList checks the history filter: a needle keeps the
// commits whose message, author or ref matches, and an empty result says so.
func TestHistoryFilterNarrowsList(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewHistory
	app.commitsLoaded = true
	app.commits = []gitx.Commit{
		{OID: "a1", Short: "a1", Subject: "Add readme", Author: "Demo"},
		{OID: "b2", Short: "b2", Subject: "Fix parser", Author: "Ana"},
	}
	app.commitFilter = "parser"
	tt := ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("Fix parser") || tt.HasText("Add readme") {
		t.Fatalf("filter texts = %q", tt.Texts())
	}
	app.commitFilter = "Ana"
	tt.Frame()
	if !tt.HasText("Fix parser") || tt.HasText("Add readme") {
		t.Fatalf("author filter texts = %q", tt.Texts())
	}
	app.commitFilter = "nothing matches this"
	tt.Frame()
	if !tt.HasText("No matching commits") {
		t.Fatalf("empty filter texts = %q", tt.Texts())
	}
}

// TestHistoryAuthorMenuFilters picks an author from the button's menu: only
// that author's commits stay, "All authors" brings the rest back, and the
// choice combines with the text filter.
func TestHistoryAuthorMenuFilters(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewHistory
	app.commitsLoaded = true
	app.commits = []gitx.Commit{
		{OID: "a1", Short: "a1", Subject: "Add readme", Author: "Demo"},
		{OID: "b2", Short: "b2", Subject: "Fix parser", Author: "Ana"},
		{OID: "c3", Short: "c3", Subject: "Fix lexer", Author: "Ana"},
	}
	tt := ui.NewTester(app.View, 1100, 720)
	if err := tt.Click("All authors"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); len(menu) != 4 {
		t.Fatalf("author menu = %q, want All authors, separator, Ana, Demo", menu)
	}
	if err := tt.ChooseMenuItem("Ana"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.commitAuthor != "Ana" || tt.HasText("Add readme") || !tt.HasText("Fix parser") {
		t.Fatalf("author %q texts = %q", app.commitAuthor, tt.Texts())
	}
	app.commitFilter = "lexer"
	tt.Frame()
	if tt.HasText("Fix parser") || !tt.HasText("Fix lexer") {
		t.Fatalf("author plus text texts = %q", tt.Texts())
	}
	app.commitFilter = ""
	app.commitAuthor = ""
	tt.Frame()
	if !tt.HasText("Add readme") {
		t.Fatalf("all authors texts = %q", tt.Texts())
	}
}

func TestChangesViewShowsSections(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Staged = []gitx.Change{{Path: "staged.go", X: "M", Y: ".", Staged: true}}
	app.status.Unstaged = []gitx.Change{{Path: "work.go", X: ".", Y: "M"}}
	app.status.Untracked = []gitx.Change{{Path: "new.go", X: "?", Y: "?", Untracked: true}}
	app.view = ViewChanges
	// Untracked starts closed; the test opens it to see the file.
	app.closedDirs = map[string]bool{}

	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"STAGED", "CHANGES", "UNTRACKED", "staged.go", "work.go", "new.go", "Commit to main"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
}

// TestChangesSplitResizes drags the divider between the changes list and the
// commit box: the list takes the room, and the commit box moves down with it.
func TestChangesSplitResizes(t *testing.T) {
	app := changesTestApp(gitx.Change{Path: "work.go", X: ".", Y: "M"})
	app.changesPercent = 50
	tt := ui.NewTester(app.View, 1100, 720)

	// The divider answers with the resize cursor: find where it shows rather
	// than hard-coding the heights above it.
	var dividerY float32
	for y := float32(150); y < 700 && dividerY == 0; y += 2 {
		tt.Move(200, y)
		if tt.Cursor() == ui.CursorResizeNS {
			dividerY = y
		}
	}
	if dividerY == 0 {
		t.Fatal("no divider between the changes list and the commit box")
	}
	before, ok := tt.Find("Commit to main")
	if !ok {
		t.Fatal("no commit button")
	}

	tt.Press(200, dividerY)
	tt.Move(200, dividerY+80)
	tt.Release(200, dividerY+80)
	tt.Frame()
	// 80 DIPs of the 603 the split shared, not the exact DIP of the divider.
	if got := app.changesPercent; got < 62 || got > 65 {
		t.Fatalf("dragged 80 DIPs down, the list takes %v%%, want about 63%%", got)
	}
	after, ok := tt.Find("Commit to main")
	if !ok {
		t.Fatal("no commit button after the drag")
	}
	if after.Y < before.Y+78 || after.Y > before.Y+82 {
		t.Fatalf("the commit button stayed at %v instead of following to %v", before.Y, after.Y)
	}
}

// TestHistorySplitResizes drags the divider between the selected commit's
// details and file list and its diff: the details take the room, and the diff
// moves down with them.
func TestHistorySplitResizes(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewHistory
	app.selectedCommit = &gitx.Commit{OID: "abc", Short: "abc1234", Subject: "Add readme", Author: "Demo"}
	app.commitFiles = []gitx.FileEntry{{Path: "README.md"}, {Path: "app.go"}}
	app.commitFile = &app.commitFiles[0]
	app.commitPanelHeight = 200
	tt := ui.NewTester(app.View, 1100, 720)

	// The divider answers with the resize cursor: find where it shows rather
	// than hard-coding the details' height.
	const x float32 = 800
	var dividerY float32
	for y := float32(150); y < 700 && dividerY == 0; y += 2 {
		tt.Move(x, y)
		if tt.Cursor() == ui.CursorResizeNS {
			dividerY = y
		}
	}
	if dividerY == 0 {
		t.Fatal("no divider between the commit's details and its diff")
	}
	before, ok := tt.Find("Unified")
	if !ok {
		t.Fatal("no diff header")
	}

	tt.Press(x, dividerY)
	tt.Move(x, dividerY+60)
	tt.Release(x, dividerY+60)
	tt.Frame()
	if got := app.commitPanelHeight; got < 259 || got > 261 {
		t.Fatalf("dragged 60 DIPs down, the details take %v DIPs, want 260", got)
	}
	after, ok := tt.Find("Unified")
	if !ok {
		t.Fatal("no diff header after the drag")
	}
	if after.Y < before.Y+58 || after.Y > before.Y+62 {
		t.Fatalf("the diff stayed at %v instead of following to %v", before.Y, after.Y)
	}
}

func TestChangeRowClickSelects(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Unstaged = []gitx.Change{{Path: "work.go", X: ".", Y: "M"}}
	app.status.Untracked = []gitx.Change{{Path: "new.go", X: "?", Y: "?", Untracked: true}}
	app.view = ViewChanges
	app.closedDirs = map[string]bool{}

	tt := ui.NewTester(app.View, 1100, 720)
	if err := tt.Click("new.go"); err != nil {
		t.Fatal(err)
	}
	if app.selection == nil || app.selection.Path != "new.go" {
		t.Fatalf("selection = %+v", app.selection)
	}
}

func TestTabClickSwitchesView(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1100, 720)
	for _, tab := range []struct {
		label string
		view  View
	}{{"History", ViewHistory}, {"Branches", ViewBranches}, {"Changes", ViewChanges}} {
		if err := tt.Click(tab.label); err != nil {
			t.Fatal(err)
		}
		if app.view != tab.view {
			t.Fatalf("click %q -> view %v", tab.label, app.view)
		}
	}
}

// changesTestApp returns a shell showing the given working-tree changes.
func changesTestApp(unstaged ...gitx.Change) *App {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Unstaged = unstaged
	app.view = ViewChanges
	return app
}

func TestChangeRowHoverShowsActions(t *testing.T) {
	app := changesTestApp(gitx.Change{Path: "work.go", X: ".", Y: "M"})

	tt := ui.NewTester(app.View, 1100, 720)
	if tt.HasText("Stage") {
		t.Fatalf("actions show without hover: %q", tt.Texts())
	}
	r, ok := tt.Find("work.go")
	if !ok {
		t.Fatalf("row missing in %q", tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	if !tt.HasText("Stage") {
		t.Fatalf("hover did not show actions: %q", tt.Texts())
	}
	tt.Move(0, 0)
	tt.Frame()
	if tt.HasText("Stage") {
		t.Fatalf("actions stayed after hover: %q", tt.Texts())
	}
}

func TestChangeRowContextMenu(t *testing.T) {
	app := changesTestApp(gitx.Change{Path: "work.go", X: ".", Y: "M"})

	tt := ui.NewTester(app.View, 1100, 720)
	if err := tt.RightClick("work.go"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	menu := tt.Menu()
	for _, want := range []string{"Show Diff", "Stage", "Discard Changes…"} {
		found := false
		for _, item := range menu {
			if item == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("menu %q has no %q", menu, want)
		}
	}
	// The empty service makes the stage fail; a toast or the write slot proves
	// the item's handler ran.
	if err := tt.ChooseMenuItem("Stage"); err != nil {
		t.Fatal(err)
	}
	if app.toast == "" && !app.writing {
		t.Fatalf("stage did not run: %q", tt.Texts())
	}
}

func TestBranchRowActivationRunsAction(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewBranches
	app.branches = []gitx.BranchEntry{
		{Name: "main", Current: true},
		{Name: "feature-x", Subject: "add feature"},
	}
	app.branchesLoaded = true
	tt := ui.NewTester(app.View, 1100, 720)
	// A click chooses the row; Enter activates it.
	if err := tt.Click("feature-x"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	// The empty service makes the checkout fail; the toast proves the row's
	// handler ran.
	if app.toast == "" && !app.writing {
		t.Fatalf("row handler did not run: %q", tt.Texts())
	}
}

func TestNewBranchSubmit(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewBranches
	app.branches = []gitx.BranchEntry{{Name: "main", Current: true}}
	app.branchesLoaded = true
	tt := ui.NewTester(app.View, 900, 600)
	if err := tt.Click("New branch"); err != nil {
		t.Fatal(err)
	}
	tt.Type("verify-branch")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if app.creating != branchCreateNone || app.newName != "" {
		t.Fatalf("submit did not run: creating=%v name=%q", app.creating, app.newName)
	}
}

func TestParsePatch(t *testing.T) {
	patch := "diff --git a/a.go b/a.go\n" +
		"index 111..222 100644\n" +
		"--- a/a.go\n" +
		"+++ b/a.go\n" +
		"@@ -1,3 +1,4 @@\n" +
		" one\n" +
		"-two\n" +
		"+two changed\n" +
		"+three\n" +
		" four\n" +
		"@@ -10 +11,2 @@\n" +
		"-old\n" +
		"+new\n" +
		"+more\n"
	got := parsePatch(patch)
	if len(got.Hunks) != 2 {
		t.Fatalf("hunks = %+v", got.Hunks)
	}
	if got.Additions != 4 || got.Deletions != 2 {
		t.Fatalf("stats +%d -%d", got.Additions, got.Deletions)
	}
	h := got.Hunks[0]
	if h.OldStart != 1 || h.NewStart != 1 || h.Additions != 2 || h.Deletions != 1 {
		t.Fatalf("hunk 0 = %+v", h)
	}
	// +three is the hunk's third new line.
	var added patchLine
	for _, line := range got.Lines {
		if line.Kind == lineAdd && line.Text == "+three" {
			added = line
		}
	}
	if added.NewNo != 3 || added.Hunk != 0 {
		t.Fatalf("added = %+v", added)
	}
	// A pure rename has headers but no hunk: it must not look like a parse
	// failure, just an empty body.
	rename := parsePatch("diff --git a/x b/y\nsimilarity index 100%\nrename from x\nrename to y\n")
	if len(rename.Hunks) != 0 || len(rename.Lines) == 0 {
		t.Fatalf("rename = %+v", rename)
	}
}

func TestLayoutGraph(t *testing.T) {
	// new — merge — old, with a side branch merged into the merge commit.
	commits := []gitx.Commit{
		{OID: "m", Parents: []string{"a", "b"}},
		{OID: "a", Parents: []string{"root"}},
		{OID: "b", Parents: []string{"root"}},
		{OID: "root"},
	}
	rows, lanes := layoutGraph(commits)
	if len(rows) != 4 || lanes < 2 {
		t.Fatalf("rows=%d lanes=%d", len(rows), lanes)
	}
	if rows[0].Lane != 0 || rows[0].Color != 0 || len(rows[0].Edges) != 2 {
		t.Fatalf("merge row = %+v", rows[0])
	}
	// The side branch ends at root; both a and b point at it.
	if rows[3].Lane != 0 {
		t.Fatalf("root lane = %d", rows[3].Lane)
	}
}

// historyTestApp returns a shell showing one tagged commit.
func historyTestApp() *App {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.view = ViewHistory
	app.commitsLoaded = true
	app.commits = []gitx.Commit{{
		OID:     "abc1234567",
		Short:   "abc1234",
		Subject: "Add avatars",
		Author:  "ada",
		Time:    time.Now().Add(-2 * time.Hour).Unix(),
		Refs:    []gitx.Ref{{Name: "v1.0", Kind: "tag"}},
	}}
	app.graph, app.graphLanes = layoutGraph(app.commits)
	return app
}

func TestHistoryRowShowsColumns(t *testing.T) {
	app := historyTestApp()
	tt := ui.NewTester(app.View, 1100, 720)

	// One row carries the avatar initial, the author, the time, the message and
	// the tag chip, in that order from left to right.
	want := []string{"A", "ada", "2h ago", "Add avatars", "v1.0"}
	rects := make([]ui.Rect, len(want))
	for i, text := range want {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
		rects[i] = r
	}
	for i := 1; i < len(rects); i++ {
		if rects[i].X <= rects[i-1].X {
			t.Fatalf("%q is not right of %q: %+v", want[i], want[i-1], rects)
		}
	}
}

func TestToolbarShowsSyncButtons(t *testing.T) {
	app := historyTestApp()
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"Fetch", "Pull", "Push"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}

	// While one runs, the toolbar names it instead of saying "Refreshing…",
	// even when a status reload is in flight too.
	app.syncing, app.syncOp, app.loadingStatus = true, "pull", true
	tt.Frame()
	if !tt.HasText("Pulling…") || tt.HasText("Refreshing…") {
		t.Fatalf("busy label texts = %q", tt.Texts())
	}
}

func TestPullNeedsABranch(t *testing.T) {
	app := historyTestApp()
	app.status.Branch = gitx.Branch{Detached: true, OID: "abc"}
	app.pullRemote()
	if app.syncing {
		t.Fatal("pull started on a detached HEAD")
	}

	app.status.Branch = gitx.Branch{Head: "main"} // unborn
	app.pushRemote()
	if app.syncing {
		t.Fatal("push started on an unborn branch")
	}
}

func TestHistoryDetailsAboveFiles(t *testing.T) {
	app := historyTestApp()
	app.selectedCommit = &app.commits[0]
	app.commitBody = "Why this change\nSecond line"
	app.commitFiles = []gitx.FileEntry{
		{Path: "src/main.go", Status: "M", Additions: 3, Deletions: 1},
		{Path: "src/util.go", Status: "A", Additions: 9},
	}
	app.commitFile = &app.commitFiles[0]
	tt := ui.NewTester(app.View, 1100, 720)

	// The right panel shows the message body, then the file list; the sidebar
	// keeps one row per commit.
	body, okBody := tt.Find("Why this change")
	file, okFile := tt.Find("src/main.go")
	meta, okMeta := tt.Find(commitMeta(&app.commits[0]))
	if !okBody || !okFile || !okMeta {
		t.Fatalf("texts = %q", tt.Texts())
	}
	const sidebar = 380
	if body.X < sidebar || file.X < sidebar {
		t.Fatalf("details and files belong to the right panel: body=%v file=%v", body, file)
	}
	if body.Y >= file.Y {
		t.Fatalf("body at %v is not above the files at %v", body, file)
	}
	if meta.Y <= body.Y {
		t.Fatalf("meta at %v is not below the body at %v", meta, body)
	}
	if !tt.HasText("src/util.go") {
		t.Fatalf("second file missing in %q", tt.Texts())
	}
}

func TestChangesFilterNarrowsList(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Unstaged = []gitx.Change{
		{Path: "src/app.go", X: ".", Y: "M"},
		{Path: "README.md", X: ".", Y: "M"},
	}
	app.view = ViewChanges
	app.changeFilter = "app.go"
	tt := ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("src") || !tt.HasText("app.go") || tt.HasText("README.md") {
		t.Fatalf("filter texts = %q", tt.Texts())
	}
}

func TestDetachedAndUnbornViews(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Detached: true, OID: "abc"}
	app.view = ViewBranches
	app.branches = []gitx.BranchEntry{{Name: "main"}}
	app.branchesLoaded = true
	tt := ui.NewTester(app.View, 900, 600)
	if !tt.HasText("Detached HEAD") {
		t.Fatalf("detached texts = %q", tt.Texts())
	}

	app.status.Branch = gitx.Branch{}
	app.branches = nil
	// The view caches what the loaders adopt, so a direct assignment in a test
	// bumps the versions the caches key on.
	app.statusVersion++
	app.branchesVersion++
	tt.Frame()
	if !tt.HasText("No branches yet") {
		t.Fatalf("unborn texts = %q", tt.Texts())
	}
}

func TestBranchErrorKeepsLoadedList(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewBranches

	app.branchErr = "boom"
	app.branchesLoaded = true
	tt := ui.NewTester(app.View, 900, 600)
	if !tt.HasText("Could not load branches") || !tt.HasText("Retry") {
		t.Fatalf("empty state texts = %q", tt.Texts())
	}

	app.branches = []gitx.BranchEntry{{Name: "main", Current: true}}
	tt.Frame()
	if !tt.HasText("main") || !tt.HasText("Could not refresh branches.") {
		t.Fatalf("stale list texts = %q", tt.Texts())
	}
}

func TestResetRepoStateUnblocksLoads(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/old")
	app.status.Unstaged = []gitx.Change{{Path: "old.go", X: ".", Y: "M"}}
	app.selection = &app.status.Unstaged[0]
	app.loadingCommits, app.loadingBranches = true, true
	app.commitsLoaded, app.branchesLoaded = true, true
	app.commits = []gitx.Commit{{OID: "old"}}
	app.branches = []gitx.BranchEntry{{Name: "old"}}

	app.resetRepoState("/new")

	if app.loadingCommits || app.loadingBranches {
		t.Fatal("loading flags survived a repository reset")
	}
	if app.commitsLoaded || app.branchesLoaded || len(app.commits) != 0 || len(app.branches) != 0 {
		t.Fatal("old lists survived a repository reset")
	}
	if app.selection != nil || len(app.status.Unstaged) != 0 || app.status.Root != "/new" {
		t.Fatalf("old status survived: %+v", app.status)
	}
}
