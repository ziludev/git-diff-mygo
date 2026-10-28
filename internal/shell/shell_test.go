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

func TestChangesViewShowsSections(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Staged = []gitx.Change{{Path: "staged.go", X: "M", Y: ".", Staged: true}}
	app.status.Unstaged = []gitx.Change{{Path: "work.go", X: ".", Y: "M"}}
	app.status.Untracked = []gitx.Change{{Path: "new.go", X: "?", Y: "?", Untracked: true}}
	app.view = ViewChanges

	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"STAGED", "CHANGES", "UNTRACKED", "staged.go", "work.go", "new.go", "Commit to main"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
}

func TestChangeRowClickSelects(t *testing.T) {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.status.Unstaged = []gitx.Change{{Path: "work.go", X: ".", Y: "M"}}
	app.status.Untracked = []gitx.Change{{Path: "new.go", X: "?", Y: "?", Untracked: true}}
	app.view = ViewChanges

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

func TestBranchRowClickRunsAction(t *testing.T) {
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
	if err := tt.Click("feature-x"); err != nil {
		t.Fatal(err)
	}
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
	if err := tt.Click("New branch from HEAD"); err != nil {
		t.Fatal(err)
	}
	tt.Type("verify-branch")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if app.creating || app.newName != "" {
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
	if !tt.HasText("src/app.go") || tt.HasText("README.md") {
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
