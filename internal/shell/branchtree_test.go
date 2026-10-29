package shell

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

// branchTestApp returns a shell with the Branches tab open on a loaded list.
func branchTestApp(branches ...gitx.BranchEntry) *App {
	app := New(&service.RepoService{}, nil)
	app.status = gitx.NewStatus("/tmp/demo")
	app.status.Branch = gitx.Branch{Head: "main", OID: "abc"}
	app.view = ViewBranches
	app.branches = branches
	app.branchesLoaded = true
	return app
}

func TestBuildBranchTreeGroupsPrefixes(t *testing.T) {
	tree := buildBranchTree([]gitx.BranchEntry{
		{Name: "main", Current: true},
		{Name: "feature/login"},
		{Name: "feature/logout"},
		// A branch and a folder with the same name both stay reachable.
		{Name: "feature"},
		{Name: "origin/main", Remote: true},
	}, "")

	if len(tree.roots) != 2 || tree.roots[0] != branchLocalSection ||
		tree.roots[1] != branchRemoteSection {
		t.Fatalf("roots = %v", tree.roots)
	}
	if got := tree.counts[branchLocalSection]; got != 4 {
		t.Fatalf("local count = %d, want 4", got)
	}
	// Folders sort before leaves; the checked-out branch sorts above the rest.
	wantLocal := []string{
		branchFolderKey(branchLocalSection, "feature"),
		branchLeafKey(branchLocalSection, "main"),
		branchLeafKey(branchLocalSection, "feature"),
	}
	assertKeys(t, tree.children[branchLocalSection], wantLocal)
	assertKeys(t, tree.children[branchFolderKey(branchLocalSection, "feature")], []string{
		branchLeafKey(branchLocalSection, "feature/login"),
		branchLeafKey(branchLocalSection, "feature/logout"),
	})
	// A remote's first segment is its name, so remotes stay apart.
	remote := tree.children[branchRemoteSection]
	assertKeys(t, remote, []string{branchFolderKey(branchRemoteSection, "origin")})
	assertKeys(t, tree.children[branchFolderKey(branchRemoteSection, "origin")], []string{
		branchLeafKey(branchRemoteSection, "origin/main"),
	})
	if tree.counts[branchFolderKey(branchLocalSection, "feature")] != 2 {
		t.Fatalf("folder count = %d, want 2",
			tree.counts[branchFolderKey(branchLocalSection, "feature")])
	}
}

func assertKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("children[%d] = %q, want %q (all %v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildBranchTreeFilterKeepsAncestors(t *testing.T) {
	branches := []gitx.BranchEntry{
		{Name: "main", Current: true},
		{Name: "feature/login"},
		{Name: "origin/feature/login", Remote: true},
		{Name: "origin/main", Remote: true},
	}
	tree := buildBranchTree(branches, "login")
	// Both the local and the remote match, with their ancestors.
	if len(tree.roots) != 2 {
		t.Fatalf("roots = %v", tree.roots)
	}
	assertKeys(t, tree.children[branchLocalSection],
		[]string{branchFolderKey(branchLocalSection, "feature")})
	origin := branchFolderKey(branchRemoteSection, "origin")
	assertKeys(t, tree.children[branchRemoteSection], []string{origin})
	assertKeys(t, tree.children[origin],
		[]string{branchFolderKey(branchRemoteSection, "origin/feature")})

	if tree := buildBranchTree(branches, "zzz"); len(tree.roots) != 0 {
		t.Fatalf("a non-matching filter kept roots %v", tree.roots)
	}
	if tree := buildBranchTree(branches, "main"); len(tree.roots) != 2 {
		t.Fatalf("main should match locally and remotely: %v", tree.roots)
	}
}

func TestCheckoutTarget(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "topic"},
		gitx.BranchEntry{Name: "feature/x"},
	)
	for _, tc := range []struct {
		entry      gitx.BranchEntry
		wantName   string
		wantRemote bool
	}{
		// A local branch switches directly.
		{gitx.BranchEntry{Name: "topic"}, "topic", false},
		// A remote with a local counterpart switches to it instead of
		// creating a second branch.
		{gitx.BranchEntry{Name: "origin/topic", Remote: true}, "topic", false},
		{gitx.BranchEntry{Name: "origin/feature/x", Remote: true}, "feature/x", false},
		// A remote without one becomes a local tracking branch.
		{gitx.BranchEntry{Name: "origin/other", Remote: true}, "origin/other", true},
	} {
		name, remote := app.checkoutTarget(tc.entry)
		if name != tc.wantName || remote != tc.wantRemote {
			t.Fatalf("checkoutTarget(%+v) = %q, %v; want %q, %v",
				tc.entry, name, remote, tc.wantName, tc.wantRemote)
		}
	}
}

func TestBranchTabTreeShowsSections(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
		gitx.BranchEntry{Name: "origin/main", Remote: true},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	for _, want := range []string{"Local", "Remote", "feature", "login", "origin"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	// The current branch's ancestors are open on the first build, so a
	// checked-out feature branch is visible without a click.
	app = branchTestApp(
		gitx.BranchEntry{Name: "feature/login", Current: true},
		gitx.BranchEntry{Name: "main"},
	)
	tt = ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("login") || !tt.HasText("current") {
		t.Fatalf("the current branch is hidden: %q", tt.Texts())
	}
}

func TestBranchTabFilterNarrowsTree(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "alpha/one"},
		gitx.BranchEntry{Name: "beta/two"},
	)
	app.branchTab.filter = "alpha"
	tt := ui.NewTester(app.View, 1100, 720)
	if !tt.HasText("alpha") || !tt.HasText("one") {
		t.Fatalf("match missing in %q", tt.Texts())
	}
	if tt.HasText("beta") || tt.HasText("two") {
		t.Fatalf("non-match survived in %q", tt.Texts())
	}
}

func TestBranchChipOpensPopover(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
		gitx.BranchEntry{Name: "origin/main", Remote: true},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if tt.HasText("Search branches") {
		t.Fatalf("popover is open before a click: %q", tt.Texts())
	}
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.branchPopupOpen {
		t.Fatal("chip click did not open the popover")
	}
	if !tt.HasText("Manage Branches") || !tt.HasText("Local") || !tt.HasText("Remote") {
		t.Fatalf("popover texts = %q", tt.Texts())
	}

	// The same chip click closes it again.
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.branchPopupOpen {
		t.Fatal("a second chip click did not close the popover")
	}
}

func TestBranchPopoverRowChecksOut(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// A click chooses the row; Enter activates it, as a list row's activation
	// is Submitted rather than a click on its content.
	if err := tt.Click("login"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	// The empty service fails the checkout; the toast or the write slot proves
	// the row's handler ran.
	if app.toast == "" && !app.writing {
		t.Fatalf("row handler did not run: %q", tt.Texts())
	}
}

func TestBranchPopoverSearchesAndCloses(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// The search field takes focus when the popover opens, so typing narrows
	// the tree without another click.
	tt.Type("login")
	tt.Frame()
	if app.branchPopupTree.filter != "login" {
		t.Fatalf("filter = %q, want login", app.branchPopupTree.filter)
	}
	if !tt.HasText("login") || tt.HasText("current") {
		t.Fatalf("filtered texts = %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if app.branchPopupOpen {
		t.Fatal("Escape did not close the popover")
	}
}

func TestBranchTreeStateResetKeepsFilter(t *testing.T) {
	st := newBranchTreeState()
	if st.row != -1 {
		t.Fatalf("row = %d, want -1", st.row)
	}
	st.filter = "feat"
	st.row = 3
	st.seeded = true
	st.outline.Open.Add(branchLocalSection)
	st.outline.Open.Add(branchFolderKey(branchLocalSection, "feature"))

	st.reset()

	if st.row != -1 || st.seeded {
		t.Fatalf("row = %d, seeded = %v", st.row, st.seeded)
	}
	if st.outline.Open.Len() != 0 {
		t.Fatalf("open folders survived: %d", st.outline.Open.Len())
	}
	// The filter is the user's words, not repository data, so it stays.
	if st.filter != "feat" {
		t.Fatalf("filter = %q, want feat", st.filter)
	}
}

// TestBranchTreeFilterCollapseSticks guards the filter's ancestor expansion:
// it must happen when the filter changes, not on every frame, or a folder
// cannot be collapsed while filtering.
func TestBranchTreeFilterCollapseSticks(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
		gitx.BranchEntry{Name: "feature/logout"},
	)
	app.branchTab.filter = "log"
	tt := ui.NewTester(app.View, 1100, 720)
	folder := branchFolderKey(branchLocalSection, "feature")
	if !app.branchTab.outline.Open.Has(folder) {
		t.Fatalf("the filter did not open the match's ancestors: %q", tt.Texts())
	}

	app.branchTab.outline.Open.Remove(folder)
	tt.Frame()
	if app.branchTab.outline.Open.Has(folder) {
		t.Fatal("a collapsed folder was reopened on the next frame")
	}

	// Another filter expands the new matches again.
	app.branchTab.filter = "logi"
	tt.Frame()
	if !app.branchTab.outline.Open.Has(folder) {
		t.Fatal("a changed filter did not open the match's ancestors")
	}
}

func TestRevealCurrentBranch(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
	)
	// Pretend both trees have shown their first list.
	app.branchTab.seeded, app.branchPopupTree.seeded = true, true

	app.status.Branch = gitx.Branch{Head: "feature/login", OID: "abc"}
	app.revealCurrentBranch()
	folder := branchFolderKey(branchLocalSection, "feature")
	for name, st := range map[string]*branchTreeState{
		"tab":     &app.branchTab,
		"popover": &app.branchPopupTree,
	} {
		if !st.outline.Open.Has(folder) {
			t.Fatalf("%s did not reveal the current branch's folder", name)
		}
	}

	// HEAD did not move again, so a folder the user closed stays closed.
	app.branchTab.outline.Open.Remove(folder)
	app.revealCurrentBranch()
	if app.branchTab.outline.Open.Has(folder) {
		t.Fatal("an unchanged HEAD reopened a closed folder")
	}
}

func TestBranchPopoverKeyboardChooses(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("login")
	tt.Frame()
	// The arrows choose a branch without leaving the search field...
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if app.branchPopupTree.row < 0 {
		t.Fatal("Down did not choose a branch")
	}
	// ...and Enter switches to it. The empty service fails the checkout; the
	// write slot or the toast proves the handler ran.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if app.toast == "" && !app.writing {
		t.Fatalf("Enter did not check out: %q", tt.Texts())
	}
}

func TestBranchPopoverCloseCancelsDraft(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature/login"},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("New Branch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.creating != branchCreatePopup {
		t.Fatalf("creating = %v, want the popover", app.creating)
	}
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.branchPopupOpen || app.creating != branchCreateNone {
		t.Fatalf("open = %v, creating = %v", app.branchPopupOpen, app.creating)
	}
}

func TestBranchPopoverCreateNeedsACommit(t *testing.T) {
	app := branchTestApp()
	app.status.Branch = gitx.Branch{Head: "main"} // unborn
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("New Branch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if app.creating != branchCreateNone {
		t.Fatalf("an unborn repository started a branch: %v", app.creating)
	}
}

func TestBranchTreeSelectedRowShowsDelete(t *testing.T) {
	app := branchTestApp(
		gitx.BranchEntry{Name: "main", Current: true},
		gitx.BranchEntry{Name: "feature-x"},
	)
	app.view = ViewChanges
	tt := ui.NewTester(app.View, 1200, 720)
	if err := tt.Click("Current branch: main"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// Choose the branch row directly; a click would check it out.
	want := branchLeafKey(branchLocalSection, "feature-x")
	for i := 0; i < app.branchPopupTree.outline.Rows(); i++ {
		if app.branchPopupTree.outline.Item(i) == want {
			app.branchPopupTree.row = i
		}
	}
	tt.Frame()
	if !tt.HasText("Delete branch") {
		t.Fatalf("the selected row has no delete action: %q", tt.Texts())
	}
}
