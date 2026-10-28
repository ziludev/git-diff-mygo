// Package shell is Twill's native user interface: it holds the window state,
// runs repository calls away from the main thread, and builds the view MyGo
// draws. It never starts git itself; internal/service owns that.
package shell

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"twill/internal/gitx"
	"twill/internal/service"
)

// historyPage is one page of the commit list; "load older" appends the next.
const historyPage = 100

// View is the left sidebar's tab.
type View int

const (
	ViewChanges View = iota
	ViewHistory
	ViewBranches
)

// App holds the window state. Every field is read and written on the main
// thread; background results arrive through Window.Update.
type App struct {
	svc     *service.RepoService
	actions *service.MenuActions
	win     *mygo.Window

	// sidebar is the file list's width when the panes sit side by side;
	// sidebarHeight is its height when they are stacked.
	sidebar       float32
	sidebarHeight float32
	panes         paneLayout

	status        gitx.Status
	loadingStatus bool
	selection     *gitx.Change
	diff          gitx.DiffResult
	worktreePatch patchFile
	hasDiff       bool
	loadingDiff   bool
	writing       bool
	errorText     string

	view           View
	commits        []gitx.Commit
	graph          []graphRow
	graphLanes     int
	commitsLoaded  bool
	hasMoreCommits bool
	loadingCommits bool
	historyErr     string

	// selectedCommit is the commit the right panel shows: its details, its
	// files and the diff of the file picked among them.
	selectedCommit    *gitx.Commit
	commitBody        string
	loadingBody       bool
	commitFiles       []gitx.FileEntry
	loadingFiles      bool
	commitFile        *gitx.FileEntry
	commitDiff        gitx.DiffResult
	commitPatch       patchFile
	hasCommitDiff     bool
	loadingCommitDiff bool
	commitErr         string

	branches        []gitx.BranchEntry
	branchesLoaded  bool
	loadingBranches bool
	branchErr       string

	message      string
	amend        bool
	changeFilter string
	branchFilter string
	creating     bool
	newName      string
	focusNew     bool

	confirm      *confirmation
	confirmOpen  bool
	toast        string
	toastIsError bool
	toastDur     time.Duration
	toastTimer   *time.Timer

	historyList ui.ListState
	// commitFilesList scrolls the selected commit's file list.
	commitFilesList ui.ListState
	diffList        ui.ListState

	// The diff pane's rows are derived, not read directly: a layout choice,
	// an expanded gap and the shown patch all change them. diffDirty makes
	// the next build recompute; diffKey names the patch they belong to.
	diffLayout diffLayout
	diffRows   []diffRow
	diffKey    diffSourceID
	diffGaps   map[int]*diffGap
	diffDirty  bool
	gapText    gapRead

	epochs epochs
}

// epochs drop responses for state the user already moved past, as the web
// store's request counters did.
type epochs struct {
	status     atomic.Uint64
	diff       atomic.Uint64
	commits    atomic.Uint64
	files      atomic.Uint64
	body       atomic.Uint64
	commitDiff atomic.Uint64
	branches   atomic.Uint64
}

type confirmation struct {
	title  string
	detail string
	label  string
	run    func()
}

// New returns the shell for one window.
func New(svc *service.RepoService, actions *service.MenuActions) *App {
	a := &App{
		svc:           svc,
		actions:       actions,
		sidebar:       380,
		sidebarHeight: 380,
		toastDur:      4 * time.Second,
	}
	// The watcher and menu actions run outside the view; marshal their
	// notifications back to the main thread before touching state.
	svc.OnChange(func(root, reason string) {
		// apply marshals to the main thread once the window exists; before
		// that there is only one thread, so running here is safe.
		a.apply(func() {
			if reason == "open" {
				// A different repository invalidates everything derived.
				a.resetRepoState(root)
			}
			a.refresh(reason)
		})
	})
	return a
}

// Attach records the window the shell updates and draws. It is called after
// NewWindow, before the window runs.
func (a *App) Attach(w *mygo.Window) { a.win = w }

// apply runs fn on the main thread, then redraws. Before the window exists
// there is only one thread, so it runs at once.
func (a *App) apply(fn func()) {
	if a.win == nil {
		fn()
		return
	}
	a.win.Update(fn)
}

// refresh reloads the status and the lists derived from it. Watch events
// skip the commit page (an edit fires one per batch) but keep the branch
// list, which is one for-each-ref.
func (a *App) refresh(reason string) {
	a.reloadStatus()
	if reason != "watch" && a.commitsLoaded {
		a.loadCommits(false)
	}
	if a.branchesLoaded {
		a.loadBranches()
	}
}

// reloadStatus reads the working tree and adopts it.
func (a *App) reloadStatus() {
	epoch := a.epochs.status.Add(1)
	a.loadingStatus = true
	root := a.status.Root
	go func() {
		st, err := a.svc.Status(context.Background())
		a.apply(func() {
			if epoch != a.epochs.status.Load() {
				return // a newer load won
			}
			a.loadingStatus = false
			if err != nil {
				a.showError(err)
				return
			}
			if root != "" && st.Root != root {
				// Another repository: its lists are not ours.
				a.resetRepoState(st.Root)
			}
			a.adoptStatus(st)
		})
	}()
}

// adoptStatus installs a status and keeps the selected row when it still
// exists, following the same path into its new bucket.
func (a *App) adoptStatus(st gitx.Status) {
	a.status = st
	if st.Root == "" {
		a.resetDiff()
		return
	}
	var next *gitx.Change
	if cur := a.selection; cur != nil {
		if same := findChange(st, *cur); same != nil {
			next = same
		} else {
			next = findChangeByPath(st, cur.Path)
		}
	}
	if next == nil {
		next = firstSelection(st)
	}
	if next == nil {
		a.resetDiff()
		return
	}
	a.loadDiff(*next)
}

// loadDiff fetches the patch of one row. The row is kept even when the load
// fails, so the panel can offer a retry through the list.
func (a *App) loadDiff(ch gitx.Change) {
	epoch := a.epochs.diff.Add(1)
	root := a.status.Root
	a.selection = &ch
	a.loadingDiff = true
	a.errorText = ""
	// The patch is about to be replaced; its gap state and reads no longer
	// describe what the pane shows.
	a.resetDiffGaps()
	go func() {
		res, err := a.svc.Diff(context.Background(), gitx.DiffRequest{
			Path:      ch.Path,
			OrigPath:  ch.OrigPath,
			Staged:    ch.Staged,
			Untracked: ch.Untracked,
		})
		// Parse away from the main thread: a large patch would drop frames.
		patch := parsePatch(res.Patch)
		a.apply(func() {
			if epoch != a.epochs.diff.Load() || a.status.Root != root {
				return
			}
			a.loadingDiff = false
			if err != nil {
				a.errorText = err.Error()
				a.hasDiff = false
				return
			}
			a.diff = res
			a.worktreePatch = patch
			a.hasDiff = true
		})
	}()
}

// resetRepoState drops everything derived from the repository the user left:
// loaded pages, the selected commit, branch lists and both diff selections.
// Epochs are bumped so an in-flight response from the old repository cannot
// land after the reset, and the loading flags are cleared so the new
// repository's first load is not blocked by a request that will never finish.
// The status is emptied too: rows from the old repository must not stay
// clickable while the new status is loading.
func (a *App) resetRepoState(root string) {
	a.epochs.commits.Add(1)
	a.epochs.branches.Add(1)
	a.loadingCommits = false
	a.loadingBranches = false
	a.status = gitx.NewStatus(root)
	a.resetDiff()
	a.resetCommitSelection()
	a.commits = nil
	a.graph = nil
	a.commitsLoaded = false
	a.hasMoreCommits = false
	a.historyErr = ""
	a.branches = nil
	a.branchesLoaded = false
	a.branchErr = ""
}

func (a *App) resetDiff() {
	a.epochs.diff.Add(1)
	a.selection = nil
	a.diff = gitx.DiffResult{}
	a.worktreePatch = patchFile{}
	a.hasDiff = false
	a.loadingDiff = false
	a.errorText = ""
	// The patch is gone, so neither its expansions nor a read still in flight
	// may survive.
	a.resetDiffGaps()
}

// write serializes one index or worktree write, then refreshes. A failure is
// a toast: it should not replace the diff with an error page.
func (a *App) write(run func() error) {
	if a.writing {
		return
	}
	a.writing = true
	go func() {
		err := run()
		a.apply(func() {
			a.writing = false
			if err != nil {
				a.showError(err)
				return
			}
			a.refresh("")
		})
	}()
}

func (a *App) stage(ch gitx.Change) {
	a.write(func() error { return a.svc.Stage(context.Background(), ch.Path) })
}

func (a *App) unstage(ch gitx.Change) {
	a.write(func() error {
		return a.svc.Unstage(context.Background(), ch.Path, ch.OrigPath)
	})
}

func (a *App) discard(ch gitx.Change) {
	a.confirmAction("Discard changes to "+ch.Path+"?",
		"This cannot be undone. The file returns to the index version.",
		"Discard", func() {
			a.write(func() error { return a.svc.Discard(context.Background(), ch.Path) })
		})
}

// applyHunk stages, unstages or discards one hunk of the selected file.
func (a *App) applyHunk(op gitx.ApplyOp, hunk int) {
	ch := a.selection
	if ch == nil {
		return
	}
	// A status or diff load in flight means the hunk indices may belong to
	// the previous patch; applying one now could touch the wrong lines.
	if a.writing || a.loadingStatus || a.loadingDiff {
		return
	}
	a.write(func() error {
		return a.svc.ApplyHunks(context.Background(), gitx.ApplyRequest{
			Path:      ch.Path,
			OrigPath:  ch.OrigPath,
			Op:        op,
			Hunks:     []int{hunk},
			Untracked: ch.Untracked,
		})
	})
}

func (a *App) commit() {
	if !a.canCommit() {
		return
	}
	msg, amend := a.message, a.amend
	a.writing = true
	go func() {
		_, err := a.svc.Commit(context.Background(), gitx.CommitRequest{
			Message: msg,
			Amend:   amend,
		})
		a.apply(func() {
			a.writing = false
			if err != nil {
				a.showError(err)
				return
			}
			a.message = ""
			a.amend = false
			a.refresh("")
		})
	}()
}

// loadCommits reads one page; more appends the next page. A page always
// replaces the list from the top so a newer commit cannot shift an older
// page the user already scrolled through.
func (a *App) loadCommits(more bool) {
	root := a.status.Root
	if root == "" {
		return
	}
	if more && a.loadingCommits {
		return
	}
	epoch := a.epochs.commits.Add(1)
	skip := 0
	if more {
		skip = len(a.commits)
	}
	a.loadingCommits = true
	a.historyErr = ""
	go func() {
		page, err := a.svc.History(context.Background(), gitx.LogRequest{
			Limit: historyPage,
			Skip:  skip,
		})
		a.apply(func() {
			if epoch != a.epochs.commits.Load() || a.status.Root != root {
				return
			}
			a.loadingCommits = false
			if err != nil {
				a.commitsLoaded = true
				// A failed append keeps the page the user already has.
				if more {
					a.showError(err)
				} else {
					a.historyErr = err.Error()
				}
				return
			}
			if more {
				a.commits = append(a.commits, page...)
			} else {
				a.commits = page
			}
			a.commitsLoaded = true
			a.hasMoreCommits = len(page) == historyPage
			a.graph, a.graphLanes = layoutGraph(a.commits)
		})
	}()
}

func (a *App) ensureCommits() {
	if a.status.Root != "" && !a.commitsLoaded && !a.loadingCommits {
		a.loadCommits(false)
	}
}

// selectCommit shows a commit in the right panel: its message details, its
// files and the diff of the first one. Selecting the commit already shown
// changes nothing.
func (a *App) selectCommit(c gitx.Commit) {
	if a.selectedCommit != nil && a.selectedCommit.OID == c.OID {
		return
	}
	a.resetCommitSelection()
	oid := c.OID
	a.selectedCommit = &c
	a.loadCommitBody(oid)

	epoch := a.epochs.files.Add(1)
	a.loadingFiles = true
	go func() {
		files, err := a.svc.CommitFiles(context.Background(), oid)
		a.apply(func() {
			if epoch != a.epochs.files.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingFiles = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitFiles = files
			// Show the first file right away: a selected commit with an empty
			// diff area looks like it is still loading.
			if len(files) > 0 {
				a.selectCommitFile(files[0])
			}
		})
	}()
}

// loadCommitBody reads the message below the subject for the detail panel.
// A failure only drops the body: the subject and the files still show.
func (a *App) loadCommitBody(oid string) {
	epoch := a.epochs.body.Add(1)
	a.loadingBody = true
	go func() {
		body, err := a.svc.CommitBody(context.Background(), oid)
		a.apply(func() {
			if epoch != a.epochs.body.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingBody = false
			if err != nil {
				log.Printf("shell: read commit %s body: %v", oid, err)
				return
			}
			a.commitBody = body
		})
	}()
}

// showsCommit reports whether oid is still the selected commit, which is what
// makes a late file or body response worth installing.
func (a *App) showsCommit(oid string) bool {
	return a.selectedCommit != nil && a.selectedCommit.OID == oid
}

func (a *App) selectCommitFile(f gitx.FileEntry) {
	commit := a.selectedCommit
	if commit == nil {
		return
	}
	epoch := a.epochs.commitDiff.Add(1)
	oid := commit.OID
	a.commitFile = &f
	a.loadingCommitDiff = true
	a.resetDiffGaps()
	a.commitDiff = gitx.DiffResult{}
	a.commitPatch = patchFile{}
	a.hasCommitDiff = false
	a.commitErr = ""
	go func() {
		res, err := a.svc.CommitDiff(context.Background(), gitx.CommitDiffRequest{
			OID:      oid,
			Path:     f.Path,
			OrigPath: f.OrigPath,
		})
		patch := parsePatch(res.Patch)
		a.apply(func() {
			if epoch != a.epochs.commitDiff.Load() || !a.showsCommit(oid) {
				return
			}
			a.loadingCommitDiff = false
			if err != nil {
				a.commitErr = err.Error()
				return
			}
			a.commitDiff = res
			a.commitPatch = patch
			a.hasCommitDiff = true
		})
	}()
}

func (a *App) resetCommitSelection() {
	a.epochs.files.Add(1)
	a.epochs.body.Add(1)
	a.epochs.commitDiff.Add(1)
	a.selectedCommit = nil
	a.commitBody = ""
	a.loadingBody = false
	a.commitFiles = nil
	a.loadingFiles = false
	a.commitFile = nil
	a.commitDiff = gitx.DiffResult{}
	a.commitPatch = patchFile{}
	a.hasCommitDiff = false
	a.loadingCommitDiff = false
	a.commitErr = ""
	a.resetDiffGaps()
}

func (a *App) loadBranches() {
	root := a.status.Root
	if root == "" {
		return
	}
	epoch := a.epochs.branches.Add(1)
	a.loadingBranches = true
	a.branchErr = ""
	go func() {
		list, err := a.svc.Branches(context.Background())
		a.apply(func() {
			if epoch != a.epochs.branches.Load() || a.status.Root != root {
				return
			}
			a.loadingBranches = false
			if err != nil {
				// Keep the last successful list: a failed refresh should not
				// replace it with an error page. The view offers a retry.
				a.branchesLoaded = true
				a.branchErr = err.Error()
				log.Printf("shell: load branches for %q failed: %v", root, err)
				return
			}
			a.branches = list
			a.branchesLoaded = true
		})
	}()
}

func (a *App) ensureBranches() {
	if a.status.Root != "" && !a.branchesLoaded && !a.loadingBranches {
		a.loadBranches()
	}
}

func (a *App) checkoutBranch(name string) {
	a.write(func() error {
		if err := a.svc.CheckoutBranch(context.Background(), name); err != nil {
			return err
		}
		// The selected diff and commit belong to the branch we left.
		a.apply(func() {
			a.resetDiff()
			a.resetCommitSelection()
		})
		return nil
	})
}

func (a *App) createBranch(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	a.write(func() error { return a.svc.CreateBranch(context.Background(), name) })
}

func (a *App) deleteBranch(name string) {
	a.confirmAction("Delete branch "+name+"?",
		"Only the branch pointer is removed. Git refuses a branch whose commits are not merged elsewhere.",
		"Delete", func() {
			a.write(func() error { return a.svc.DeleteBranch(context.Background(), name) })
		})
}

// canCommit mirrors the commit panel's disabled rules.
func (a *App) canCommit() bool {
	return !a.writing && strings.TrimSpace(a.message) != "" &&
		(a.amend || len(a.status.Staged) > 0)
}

// openPicker asks for a repository and opens it; the service notifies the
// shell when the open completes.
func (a *App) openPicker() {
	if a.actions != nil {
		a.actions.OpenFromMenu()
	}
}

// showError reports a failed action without hiding the current view.
func (a *App) showError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	// git's stderr often ends with a newline; a toast should not.
	msg = strings.TrimSpace(msg)
	a.toast = msg
	a.toastIsError = true
	if a.win != nil {
		// Clear the toast after a few seconds unless a newer one replaced it.
		// One timer per App: a burst of errors must not leave timers behind.
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
		text := msg
		a.toastTimer = time.AfterFunc(a.toastDur, func() {
			a.apply(func() {
				if a.toast == text {
					a.toast = ""
					a.toastIsError = false
					a.toastTimer = nil
				}
			})
		})
	}
}

func (a *App) confirmAction(title, detail, label string, run func()) {
	a.confirm = &confirmation{title: title, detail: detail, label: label, run: run}
	a.confirmOpen = true
}

// helpers

func sameChange(a, b gitx.Change) bool {
	return a.Path == b.Path && a.Staged == b.Staged &&
		a.Untracked == b.Untracked && a.Conflicted == b.Conflicted
}

func allChanges(st gitx.Status) []gitx.Change {
	out := make([]gitx.Change, 0, len(st.Conflicted)+len(st.Staged)+len(st.Unstaged)+len(st.Untracked))
	out = append(out, st.Conflicted...)
	out = append(out, st.Staged...)
	out = append(out, st.Unstaged...)
	out = append(out, st.Untracked...)
	return out
}

func findChange(st gitx.Status, want gitx.Change) *gitx.Change {
	all := allChanges(st)
	for i := range all {
		if sameChange(all[i], want) {
			return &all[i]
		}
	}
	return nil
}

func findChangeByPath(st gitx.Status, path string) *gitx.Change {
	all := allChanges(st)
	for i := range all {
		if all[i].Path == path {
			return &all[i]
		}
	}
	return nil
}

func firstSelection(st gitx.Status) *gitx.Change {
	switch {
	case len(st.Conflicted) > 0:
		return &st.Conflicted[0]
	case len(st.Unstaged) > 0:
		return &st.Unstaged[0]
	case len(st.Staged) > 0:
		return &st.Staged[0]
	case len(st.Untracked) > 0:
		return &st.Untracked[0]
	}
	return nil
}

func repoName(root string) string {
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

func relativeTime(unix int64) string {
	delta := time.Until(time.Unix(unix, 0))
	abs := delta
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < time.Minute:
		return "just now"
	case abs < time.Hour:
		return fmt.Sprintf("%dm ago", int(abs.Minutes()))
	case abs < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(abs.Hours()))
	case abs < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(abs.Hours()/24))
	case abs < 365*24*time.Hour:
		return fmt.Sprintf("%dmo ago", int(abs.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy ago", int(abs.Hours()/(24*365)))
	}
}
