package shell

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
)

func TestStep(t *testing.T) {
	for _, tc := range []struct{ cur, delta, n, want int }{
		{-1, 1, 3, 0},  // nothing chosen: Down takes the first
		{-1, -1, 3, 2}, // nothing chosen: Up takes the last
		{0, 1, 3, 1},
		{2, 1, 3, 2},  // the end holds
		{0, -1, 3, 0}, // the start holds
		{0, 1, 0, -1}, // an empty list has nowhere to go
		{-1, 1, 0, -1},
	} {
		if got := step(tc.cur, tc.delta, tc.n); got != tc.want {
			t.Fatalf("step(%d, %d, %d) = %d, want %d", tc.cur, tc.delta, tc.n, got, tc.want)
		}
	}
}

func selectedPath(app *App) string {
	if app.selection == nil {
		return ""
	}
	return app.selection.Path
}

// TestChangesArrowKeysFollowTheScreen walks the file rows in the order they
// are drawn: the tree's order (folders first), skipping a closed folder, with
// the edges holding.
func TestChangesArrowKeysFollowTheScreen(t *testing.T) {
	app := changesTestApp(
		gitx.Change{Path: "README.md", X: ".", Y: "M"},
		gitx.Change{Path: "src/app.go", X: ".", Y: "M"},
		gitx.Change{Path: "src/util/x.go", X: ".", Y: "M"},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	press := func(key ui.Key) {
		tt.Key(0, key)
		tt.Frame()
	}
	// Drawn order: src/util/x.go (deepest folder first), src/app.go, README.md.
	press(ui.KeyDown)
	if got := selectedPath(app); got != "src/util/x.go" {
		t.Fatalf("first Down chose %q", got)
	}
	press(ui.KeyDown)
	press(ui.KeyDown)
	if got := selectedPath(app); got != "README.md" {
		t.Fatalf("third Down chose %q", got)
	}
	press(ui.KeyDown)
	if got := selectedPath(app); got != "README.md" {
		t.Fatalf("Down at the end moved to %q", got)
	}
	press(ui.KeyUp)
	if got := selectedPath(app); got != "src/app.go" {
		t.Fatalf("Up chose %q", got)
	}

	// With src closed, its files are not on screen and not visited.
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	press(ui.KeyUp)
	if got := selectedPath(app); got != "README.md" {
		// The chosen file is no longer drawn: Up starts from the end.
		t.Fatalf("Up with the chosen file hidden chose %q", got)
	}
}

// TestSpaceIgnoresHiddenSelection: a selection the filter hides is not a row
// the user is pointing at, so Space must not write to it.
func TestSpaceIgnoresHiddenSelection(t *testing.T) {
	app := changesTestApp(
		gitx.Change{Path: "a.go", X: ".", Y: "M"},
		gitx.Change{Path: "b.go", X: ".", Y: "M"},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if got := selectedPath(app); got != "a.go" {
		t.Fatalf("Down chose %q", got)
	}
	app.changeFilter = "b.go"
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	// write() marks the app busy before its goroutine starts, so this needs no
	// wait: a stage attempt is visible immediately.
	if app.writing {
		t.Fatal("Space staged a file the filter hides")
	}
}

func TestKeysIgnoredInDialogs(t *testing.T) {
	app := changesTestApp(
		gitx.Change{Path: "a.go", X: ".", Y: "M"},
		gitx.Change{Path: "b.go", X: ".", Y: "M"},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	app.confirmOpen = true
	app.confirm = &confirmation{title: "t", detail: "d", label: "ok", run: func() {}}
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeySpace)
	if app.selection != nil {
		t.Fatalf("Down moved the choice behind a dialog: %q", selectedPath(app))
	}
}

// TestFilterFieldKeepsTypingAndPassesArrows: Cmd+F puts the caret in the
// filter, what is typed there is text, and Up/Down, which a one-line field has
// no use for, still move the choice, so "type, then Down" works.
func TestFilterFieldKeepsTypingAndPassesArrows(t *testing.T) {
	app := changesTestApp(
		gitx.Change{Path: "a.go", X: ".", Y: "M"},
		gitx.Change{Path: "b.go", X: ".", Y: "M"},
	)
	tt := ui.NewTester(app.View, 1100, 720)
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Frame()
	tt.Type("b")
	tt.TypeKey(0, ui.KeySpace, " ") // a space typed in the field, not a stage
	tt.Frame()
	if app.changeFilter != "b " || app.writing {
		t.Fatalf("filter = %q, writing = %v", app.changeFilter, app.writing)
	}
	app.changeFilter = "b.go"
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if got := selectedPath(app); got != "b.go" {
		t.Fatalf("Down from the filter chose %q", got)
	}
}

// TestCommitMessageKeepsArrows: the multi-line message uses Up and Down for
// its caret, and Space for text.
func TestCommitMessageKeepsArrows(t *testing.T) {
	app := changesTestApp(gitx.Change{Path: "a.go", X: ".", Y: "M"})
	tt := ui.NewTester(app.View, 1100, 720)
	amend := mustFind(t, tt, "Amend")
	tt.ClickAt(amend.X+60, amend.Y-40) // inside the message area above the row
	tt.Frame()
	tt.Type("one")
	tt.Key(0, ui.KeyDown)
	tt.TypeKey(0, ui.KeySpace, " ")
	tt.Frame()
	if app.selection != nil {
		t.Fatalf("Down in the commit message moved the choice: %q", selectedPath(app))
	}
}

func TestHistoryArrowKeys(t *testing.T) {
	app, _ := historyApp(t)
	tt := ui.NewTester(app.View, 1100, 720)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if app.selectedCommit == nil || app.selectedCommit.Subject != "commit 3" {
		t.Fatalf("first Down chose %+v", app.selectedCommit)
	}
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if app.selectedCommit.Subject != "commit 2" {
		t.Fatalf("second Down chose %q", app.selectedCommit.Subject)
	}
	// The filter narrows what the keys visit.
	app.commitFilter = "commit 1"
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if app.selectedCommit.Subject != "commit 1" {
		t.Fatalf("Down under a filter chose %q", app.selectedCommit.Subject)
	}
	tt.Key(0, ui.KeyUp)
	tt.Frame()
	if !strings.HasPrefix(app.selectedCommit.Subject, "commit 1") {
		t.Fatalf("Up at the start left %q", app.selectedCommit.Subject)
	}
}

// TestSpaceTogglesStage stages a file with Space and unstages it with Space
// again, the choice following the file between sections.
func TestSpaceTogglesStage(t *testing.T) {
	app, dir := changesRepoApp(t)
	tt := ui.NewTester(app.View, 1100, 720)
	tt.Key(0, ui.KeyDown) // the first drawn file: dir/sub/b.txt
	tt.Frame()
	if got := selectedPath(app); got != "dir/sub/b.txt" {
		t.Fatalf("Down chose %q", got)
	}
	tt.Key(0, ui.KeySpace)
	waitIdle(t, app)
	if got := gitIn(t, dir, "diff", "--cached", "--name-only"); got != "dir/sub/b.txt" {
		t.Fatalf("Space staged %q", got)
	}
	// The refresh moved the file into Staged; the choice follows it.
	app.status = mustStatus(t, app)
	app.adoptStatus(app.status)
	tt.Frame()
	if app.selection == nil || !app.selection.Staged {
		t.Fatalf("the choice did not follow into Staged: %+v", app.selection)
	}
	tt.Key(0, ui.KeySpace)
	waitIdle(t, app)
	if got := gitIn(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("second Space left %q staged", got)
	}
}
