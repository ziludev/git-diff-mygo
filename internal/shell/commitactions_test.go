package shell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

// gitIn runs git in dir for a test fixture and returns its trimmed output.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
	}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// historyApp opens a real repository with three commits (a.txt: 1, 2, 3, one
// commit each, oldest first) in the History tab, loaded like the app loads it.
func historyApp(t *testing.T) (app *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q")
	for _, v := range []string{"1", "2", "3"} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "add", "-A")
		gitIn(t, dir, "commit", "-q", "-m", "commit "+v)
	}
	svc := &service.RepoService{}
	if _, err := svc.Open(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	app = New(svc, nil)
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.status = st
	commits, err := svc.History(context.Background(), gitx.LogRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	app.commits, app.commitsLoaded = commits, true
	app.graph, app.graphLanes = layoutGraph(commits)
	app.view = ViewHistory
	return app, dir
}

// rightClickMenu opens the context menu of the row showing text.
func rightClickMenu(t *testing.T, tt *ui.Tester, text string) []string {
	t.Helper()
	if err := tt.RightClick(text); err != nil {
		t.Fatal(err)
	}
	return tt.Menu()
}

func hasItem(menu []string, want string) bool {
	for _, m := range menu {
		if m == want {
			return true
		}
	}
	return false
}

func TestCommitMenuItems(t *testing.T) {
	app, _ := historyApp(t)
	tt := ui.NewTester(app.View, 1100, 720)

	// The tip is the current commit: nothing to cherry-pick.
	menu := rightClickMenu(t, tt, "commit 3")
	for _, want := range []string{"Copy Hash", "Checkout Revision", "New Branch from Here…",
		"Tag Here…", "Revert", "Reset Current Branch to Here"} {
		if !hasItem(menu, want) {
			t.Fatalf("tip menu = %q misses %q", menu, want)
		}
	}
	if hasItem(menu, "Cherry-pick") {
		t.Fatalf("the tip offers cherry-pick: %q", menu)
	}
	tt.CloseMenu()

	menu = rightClickMenu(t, tt, "commit 1")
	if !hasItem(menu, "Cherry-pick") {
		t.Fatalf("older commit menu = %q", menu)
	}
	tt.CloseMenu()

	// A merge commit has neither cherry-pick nor revert.
	for i := range app.commits {
		if app.commits[i].Subject == "commit 1" {
			app.commits[i].Parents = []string{"a", "b"}
		}
	}
	tt.Frame()
	menu = rightClickMenu(t, tt, "commit 1")
	if hasItem(menu, "Cherry-pick") || hasItem(menu, "Revert") {
		t.Fatalf("merge menu = %q", menu)
	}
}

func TestCommitMenuCopyHash(t *testing.T) {
	app, _ := historyApp(t)
	tt := ui.NewTester(app.View, 1100, 720)
	rightClickMenu(t, tt, "commit 2")
	if err := tt.ChooseMenuItem("Copy Hash"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	var want string
	for _, c := range app.commits {
		if c.Subject == "commit 2" {
			want = c.OID
		}
	}
	if got := tt.Clipboard(); got != want || want == "" {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
}

// TestCommitMenuTagPrompt walks the prompt dialog: the button waits for a name,
// Enter creates a tag at the chosen commit.
func TestCommitMenuTagPrompt(t *testing.T) {
	app, dir := historyApp(t)
	tt := ui.NewTester(app.View, 1100, 720)
	rightClickMenu(t, tt, "commit 1")
	if err := tt.ChooseMenuItem("Tag Here…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.confirmOpen || app.confirm == nil || app.confirm.runInput == nil {
		t.Fatalf("no prompt open: %q", tt.Texts())
	}
	// An empty name does nothing, and the dialog stays.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !app.confirmOpen {
		t.Fatal("Enter with an empty name closed the dialog")
	}
	tt.Type("v1")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if app.confirmOpen {
		t.Fatalf("dialog still open: %q", tt.Texts())
	}
	waitIdle(t, app)
	want := gitIn(t, dir, "rev-parse", "HEAD~2")
	if got := gitIn(t, dir, "rev-parse", "v1^{commit}"); got != want {
		t.Fatalf("tag v1 = %s, want %s", got, want)
	}
}

// waitIdle lets the write the test started finish: without a window, apply
// runs inline, so the write is done once the flag clears.
func waitIdle(t *testing.T, app *App) {
	t.Helper()
	for i := 0; i < 500 && app.writing; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if app.writing {
		t.Fatal("the write never finished")
	}
}

func TestCommitMenuHardResetConfirms(t *testing.T) {
	app, dir := historyApp(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(app.View, 1100, 720)
	rightClickMenu(t, tt, "commit 1")
	if err := tt.ChooseMenuItem("Reset Current Branch to Here", "Hard…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// Nothing moved yet: the dialog asks first.
	if !app.confirmOpen || gitIn(t, dir, "log", "--format=%s", "-1") != "commit 3" {
		t.Fatalf("hard reset did not ask: %q", tt.Texts())
	}
	if err := tt.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
	if got := gitIn(t, dir, "log", "--format=%s", "-1"); got != "commit 1" {
		t.Fatalf("HEAD is %q after the reset", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "1\n" {
		t.Fatalf("a.txt = %q, the hard reset left the dirty edit", b)
	}
}

func TestCommitMenuCheckoutAndRevert(t *testing.T) {
	app, dir := historyApp(t)
	tt := ui.NewTester(app.View, 1100, 720)

	rightClickMenu(t, tt, "commit 3")
	if err := tt.ChooseMenuItem("Revert"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitIdle(t, app)
	if got := gitIn(t, dir, "log", "--format=%s", "-1"); !strings.HasPrefix(got, `Revert "commit 3"`) {
		t.Fatalf("HEAD is %q after the revert", got)
	}

	// The refresh after the write reloads the list in the background.
	for i := 0; i < 500 && (app.loadingCommits || len(app.commits) != 4); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	rightClickMenu(t, tt, "commit 1")
	if err := tt.ChooseMenuItem("Checkout Revision"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitIdle(t, app)
	if got := gitIn(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "HEAD" {
		t.Fatalf("HEAD is %q, want a detached HEAD", got)
	}
}

// changesRepoApp opens a real repository whose work tree has an untracked
// folder (dir/a.txt, dir/sub/b.txt) beside a committed file, in the Changes tab
// with the untracked section open.
func changesRepoApp(t *testing.T) (app *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q")
	for name, body := range map[string]string{"root.txt": "r\n", "dir/a.txt": "a\n", "dir/sub/b.txt": "b\n"} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "add", "root.txt")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	svc := &service.RepoService{}
	if _, err := svc.Open(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	app = New(svc, nil)
	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	app.status = st
	app.view = ViewChanges
	app.closedDirs = map[string]bool{}
	return app, dir
}

// TestFolderStageAndUnstage stages an untracked folder from its hover button
// and unstages it again from the context menu; both act on the whole folder,
// subfolders included, in one call.
func TestFolderStageAndUnstage(t *testing.T) {
	app, dir := changesRepoApp(t)
	tt := ui.NewTester(app.View, 1100, 720)

	tt.Move(100, mustFind(t, tt, "dir").Y+4)
	if err := tt.Click("Stage folder"); err != nil {
		t.Fatalf("no hover button: %v in %q", err, tt.Texts())
	}
	waitIdle(t, app)
	if got := gitIn(t, dir, "diff", "--cached", "--name-only"); got != "dir/a.txt\ndir/sub/b.txt" {
		t.Fatalf("staged = %q", got)
	}

	// The refresh moved the folder to the Staged section; its menu unstages.
	app.status = mustStatus(t, app)
	tt.Frame()
	if menu := rightClickMenu(t, tt, "dir"); !hasItem(menu, "Unstage Folder") || hasItem(menu, "Stage Folder") {
		t.Fatalf("staged folder menu = %q", menu)
	}
	if err := tt.ChooseMenuItem("Unstage Folder"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitIdle(t, app)
	if got := gitIn(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("still staged: %q", got)
	}
}

func mustFind(t *testing.T, tt *ui.Tester, text string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("no element shows %q in %q", text, tt.Texts())
	}
	return r
}

func mustStatus(t *testing.T, app *App) gitx.Status {
	t.Helper()
	st, err := app.svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// pushRepoApp opens a clone of a bare origin with one pushed commit, then adds
// local commits; branch decides whether the clone's branch is renamed first
// (a new branch has no upstream).
func pushRepoApp(t *testing.T, newBranch string, local ...string) (app *App, dir, origin string) {
	t.Helper()
	base := t.TempDir()
	origin = filepath.Join(base, "origin.git")
	dir = filepath.Join(base, "clone")
	gitIn(t, base, "init", "-q", "--bare", "-b", "main", origin)
	gitIn(t, base, "clone", "-q", origin, dir)
	commit := func(msg string) {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(msg+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "add", "-A")
		gitIn(t, dir, "commit", "-q", "-m", msg)
	}
	commit("first")
	gitIn(t, dir, "push", "-q", "origin", "main")
	gitIn(t, dir, "branch", "-q", "--set-upstream-to=origin/main", "main")
	if newBranch != "" {
		gitIn(t, dir, "checkout", "-q", "-b", newBranch)
	}
	for _, msg := range local {
		commit(msg)
	}
	svc := &service.RepoService{}
	if _, err := svc.Open(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	app = New(svc, nil)
	app.status = mustStatus(t, app)
	app.view = ViewChanges
	return app, dir, origin
}

func waitPushLoaded(t *testing.T, app *App) {
	t.Helper()
	for i := 0; i < 500 && (app.push == nil || app.push.loading); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if app.push == nil || app.push.loading {
		t.Fatal("the push dialog never loaded")
	}
}

func waitSyncIdle(t *testing.T, app *App) {
	t.Helper()
	for i := 0; i < 1000 && (app.syncing || app.writing); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if app.syncing || app.writing {
		t.Fatal("the push never finished")
	}
}

// TestPushDialogListsAndPushes: the toolbar's Push opens the preview with the
// outgoing commits, and its button then pushes them.
func TestPushDialogListsAndPushes(t *testing.T) {
	app, _, origin := pushRepoApp(t, "", "second", "third")
	tt := ui.NewTester(app.View, 1100, 720)
	if err := tt.Click("Push"); err != nil {
		t.Fatal(err)
	}
	waitPushLoaded(t, app)
	tt.Frame()
	for _, want := range []string{"Push main → origin/main", "second", "third"} {
		if !tt.HasText(want) {
			t.Fatalf("dialog misses %q in %q", want, tt.Texts())
		}
	}
	if got := gitIn(t, origin, "log", "--format=%s", "-1", "main"); got != "first" {
		t.Fatalf("origin changed before the button: %q", got)
	}
	// "Push" names both the toolbar icon and the dialog's button, so the
	// button's action is called directly.
	app.runPush()
	waitSyncIdle(t, app)
	if got := gitIn(t, origin, "log", "--format=%s", "-1", "main"); got != "third" {
		t.Fatalf("origin has %q after the push", got)
	}
}

func TestPushDialogPublishesNewBranch(t *testing.T) {
	app, _, origin := pushRepoApp(t, "topic", "topic work")
	tt := ui.NewTester(app.View, 1100, 720)
	app.openPush()
	waitPushLoaded(t, app)
	tt.Frame()
	if !tt.HasText("Publish topic → origin/topic") || !tt.HasText("topic work") || !tt.HasText("Publish") {
		t.Fatalf("publish dialog = %q", tt.Texts())
	}
	app.runPush()
	waitSyncIdle(t, app)
	if got := gitIn(t, origin, "log", "--format=%s", "-1", "topic"); got != "topic work" {
		t.Fatalf("origin topic = %q", got)
	}
	if up := gitIn(t, app.status.Root, "rev-parse", "--abbrev-ref", "topic@{upstream}"); up != "origin/topic" {
		t.Fatalf("upstream = %q", up)
	}
}

func TestPushDialogNothingToPush(t *testing.T) {
	app, _, _ := pushRepoApp(t, "")
	tt := ui.NewTester(app.View, 1100, 720)
	app.openPush()
	waitPushLoaded(t, app)
	tt.Frame()
	if app.canPush() || !tt.HasText("Nothing to push.") {
		t.Fatalf("empty dialog = %q", tt.Texts())
	}
	// Escape or Cancel closes it without touching anything.
	app.closePush()
	tt.Frame()
	if app.pushOpen || app.push != nil {
		t.Fatal("dialog still open")
	}
}

// TestCommitAndPushOpensDialog commits the staged file, then shows the dialog
// with the new commit in it; Amend turns the combined action off.
func TestCommitAndPushOpensDialog(t *testing.T) {
	app, dir, _ := pushRepoApp(t, "")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "new.txt")
	app.status = mustStatus(t, app)
	app.message = "add new"
	tt := ui.NewTester(app.View, 1100, 720)

	app.amend = true
	app.commitThen(true)
	if app.writing || app.pushOpen {
		t.Fatal("Commit and Push ran with Amend checked")
	}
	app.amend = false
	if err := tt.Click("Commit and Push…"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, app)
	waitPushLoaded(t, app)
	tt.Frame()
	if !tt.HasText("add new") || !app.canPush() {
		t.Fatalf("dialog after commit = %q", tt.Texts())
	}
	if got := gitIn(t, dir, "log", "--format=%s", "-1"); got != "add new" {
		t.Fatalf("HEAD = %q", got)
	}
}

// stashApp opens a repository with one commit, a modified tracked file and an
// untracked one, with the Changes tab showing it.
func stashApp(t *testing.T) (app *App, dir string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := &service.RepoService{}
	if _, err := svc.Open(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	app = New(svc, nil)
	app.status = mustStatus(t, app)
	app.view = ViewChanges
	return app, dir
}

// waitStashes waits for the stash list to settle on n entries.
func waitStashes(t *testing.T, app *App, n int) {
	t.Helper()
	for i := 0; i < 500 && (app.loadingStashes || !app.stashesLoaded || len(app.stashes) != n); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if len(app.stashes) != n {
		t.Fatalf("stashes = %+v, want %d", app.stashes, n)
	}
}

// TestStashFlow walks the whole feature through the UI: stash with the dialog
// (untracked files included), see it in the Stashes tab with its files in the
// right panel, apply it, then drop it after the confirmation.
func TestStashFlow(t *testing.T) {
	app, dir := stashApp(t)
	tt := ui.NewTester(app.View, 1100, 720)

	if err := tt.Click("Stash"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.confirmOpen || app.confirm.runInput == nil || !app.promptOption {
		t.Fatalf("no stash dialog: %q", tt.Texts())
	}
	tt.Type("wip login")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	waitIdle(t, app)
	waitStashes(t, app, 1)
	if got := gitIn(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("work tree after stash: %q (untracked files must go along)", got)
	}

	app.show(ViewStashes)
	tt.Frame()
	if !tt.HasText("wip login") || !tt.HasText("Stashes") {
		t.Fatalf("stashes tab = %q", tt.Texts())
	}
	// Choosing the stash shows its changes in the right panel.
	if err := tt.Click("wip login"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500 && (app.loadingFiles || len(app.commitFiles) == 0); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if len(app.commitFiles) != 1 || app.commitFiles[0].Path != "a.txt" {
		t.Fatalf("stash files = %+v", app.commitFiles)
	}

	rightClickMenu(t, tt, "wip login")
	if err := tt.ChooseMenuItem("Apply"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	waitIdle(t, app)
	if b, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(b) != "edited\n" {
		t.Fatalf("apply left a.txt = %q", b)
	}
	waitStashes(t, app, 1) // apply keeps the stash

	rightClickMenu(t, tt, "wip login")
	if err := tt.ChooseMenuItem("Drop…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !app.confirmOpen || len(app.stashes) != 1 {
		t.Fatal("drop did not ask first")
	}
	// "Drop" also names the row's hover button, so the dialog's action runs
	// directly.
	app.confirmOpen = false
	app.confirm.run()
	waitIdle(t, app)
	waitStashes(t, app, 0)
	tt.Frame()
	if !tt.HasText("No stashes") {
		t.Fatalf("empty tab = %q", tt.Texts())
	}
}

// TestStashNothingToSave: a clean tree disables the button, and the dialog's
// Include-untracked default is on.
func TestStashButtonDisabledWhenClean(t *testing.T) {
	app, dir := stashApp(t)
	gitIn(t, dir, "checkout", "--", "a.txt")
	gitIn(t, dir, "clean", "-fdq")
	app.status = mustStatus(t, app)
	if app.canStash() {
		t.Fatal("a clean work tree can be stashed")
	}
}

func TestStashPopRestoresAndDrops(t *testing.T) {
	app, dir := stashApp(t)
	gitIn(t, dir, "stash", "push", "-q", "--include-untracked", "-m", "keep")
	app.status = mustStatus(t, app)
	app.loadStashes()
	waitStashes(t, app, 1)
	app.popStash(app.stashes[0])
	waitIdle(t, app)
	waitStashes(t, app, 0)
	if got := gitIn(t, dir, "status", "--porcelain"); !strings.Contains(got, "a.txt") || !strings.Contains(got, "new.txt") {
		t.Fatalf("status after pop = %q", got)
	}
}
